package esmdev

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"golang.org/x/sync/errgroup"

	"tools/please_js/common"
)

// Args holds the arguments for the esm-dev subcommand.
type Args struct {
	Entry        string
	ModuleConfig string
	Servedir     string
	Port         int
	Tsconfig     string
	Define       []string
	Proxy        []string
	EnvFile      string
	EnvPrefix    string
	PrebundleDir string // path to pre-bundled deps dir (skips runtime prebundle)
	Root         string // package root for source file resolution
}

// transformEntry caches a transformed source file.
type transformEntry struct {
	code    []byte
	modTime time.Time
}

// sseEvent is sent to clients when files change.
type sseEvent struct {
	Type  string   `json:"type"`
	Files []string `json:"files,omitempty"`
}

// esmServer serves individual ES modules with on-demand transformation.
type esmServer struct {
	sourceRoot     string // servedir — HTML and static files
	packageRoot    string // package root — source files (JS/TS)
	depCache       map[string][]byte // "/@deps/react.js" → pre-bundled ESM
	transCache     sync.Map          // abs path → *transformEntry
	importMapJSON  []byte
	clients        map[chan sseEvent]struct{}
	sseMu          sync.Mutex
	proxies        map[string]*httputil.ReverseProxy
	proxyPrefixes  []string
	define         map[string]string
	tsconfig       string
	hasRefresh     bool     // true if react-refresh found in pre-bundled deps
	entryURLPath   string   // entry file URL path (e.g., "/main.jsx") — skip HMR for this
	componentFiles sync.Map // abs path → bool (true if last transform found components)
}

// liveReloadScript is injected into HTML pages for automatic reload on file changes.
// Used as fallback when react-refresh is not available.
const liveReloadScript = `<script type="module">
(() => {
  const es = new EventSource("/__esm_dev_sse");
  let t;
  es.addEventListener("change", () => {
    clearTimeout(t);
    t = setTimeout(() => location.reload(), 100);
  });
})();
</script>`

// refreshInitScript initializes react-refresh before React loads.
// Imports from "react-refresh" (main entry) which is guaranteed to be in the import map.
const refreshInitScript = `<script type="module">
import RefreshRuntime from "react-refresh";
RefreshRuntime.injectIntoGlobalHook(window);
window.$RefreshReg$ = () => {};
window.$RefreshSig$ = () => (type) => type;
window.__REACT_REFRESH__ = RefreshRuntime;
</script>`

// hmrClientScript is the HMR client that handles SSE events for hot module replacement.
const hmrClientScript = `<script type="module">
window.__ESM_HMR__ = {
  createContext(moduleUrl) {
    const hot = {
      _acceptCb: null,
      accept(cb) { hot._acceptCb = cb || (() => {}); },
    };
    window.__ESM_HMR__._modules.set(moduleUrl, hot);
    return hot;
  },
  _modules: new Map(),
};

const es = new EventSource("/__esm_dev_sse");

es.addEventListener("hmr-update", async (e) => {
  const { files } = JSON.parse(e.data);
  let didUpdate = false;
  for (const file of files) {
    try {
      await import(file + "?t=" + Date.now());
      didUpdate = true;
    } catch (err) {
      console.error("[hmr] Failed to update " + file, err);
      location.reload();
      return;
    }
  }
  if (didUpdate && window.__REACT_REFRESH__) {
    window.__REACT_REFRESH__.performReactRefresh();
  }
});

es.addEventListener("css-update", async (e) => {
  const { files } = JSON.parse(e.data);
  for (const file of files) {
    try {
      await import(file + "?t=" + Date.now());
    } catch (err) {
      console.warn("[hmr] CSS update failed for " + file, err);
    }
  }
});

es.addEventListener("full-reload", () => {
  location.reload();
});
</script>`

// cssModuleTemplate wraps CSS content in a JS module that injects a <style> tag.
// Uses data-file attribute for identification so HMR can replace existing styles.
const cssModuleTemplate = `const __file = %q;
let s = document.querySelector('style[data-file="' + __file + '"]');
if (!s) { s = document.createElement('style'); s.dataset.file = __file; document.head.appendChild(s); }
s.textContent = %s;
`


// assetModuleTemplate wraps an asset URL in a JS module for ESM imports.
const assetModuleTemplate = `export default %q;
`

// assetExts is the set of file extensions treated as static assets.
var assetExts = func() map[string]bool {
	m := make(map[string]bool)
	for ext, loader := range common.Loaders {
		if loader == api.LoaderFile {
			m[ext] = true
		}
	}
	return m
}()

// isAssetExt reports whether the extension is a known asset type.
func isAssetExt(ext string) bool {
	return assetExts[ext]
}

// parseProxies converts "prefix=target" strings into reverse proxy instances.
func parseProxies(specs []string) (map[string]*httputil.ReverseProxy, []string) {
	proxies := make(map[string]*httputil.ReverseProxy, len(specs))
	var prefixes []string
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			continue
		}
		prefix := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		u, err := url.Parse(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid proxy target %q: %v\n", target, err)
			continue
		}
		proxy := httputil.NewSingleHostReverseProxy(u)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = u.Host
		}
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		proxies[prefix] = proxy
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return proxies, prefixes
}

// depLoaders is a filtered version of common.Loaders that excludes the file
// loader. The file loader requires an output path on disk, but pre-bundling
// writes to memory (Write: false). Assets like images and fonts are not needed
// in pre-bundled dependency ESM output.
var depLoaders = func() map[string]api.Loader {
	m := make(map[string]api.Loader, len(common.Loaders))
	for ext, loader := range common.Loaders {
		if loader != api.LoaderFile {
			m[ext] = loader
		}
	}
	return m
}()

// importSpecRe matches bare import specifiers in JS/TS source code.
// Catches: import X from "pkg", import "pkg", import("pkg"), require("pkg"),
// export { X } from "pkg", export * from "pkg".
var importSpecRe = regexp.MustCompile(`(?:from\s+|import\s*\(\s*|import\s+|require\s*\(\s*)["']([^"']+)["']`)

// packageNameFromSpec extracts the npm package name from an import specifier.
// "react" → "react", "react-dom/client" → "react-dom",
// "@scope/pkg" → "@scope/pkg", "@scope/pkg/sub" → "@scope/pkg".
func packageNameFromSpec(spec string) string {
	if strings.HasPrefix(spec, "@") {
		// Scoped: @scope/name or @scope/name/subpath
		parts := strings.SplitN(spec, "/", 3)
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return spec
	}
	// Unscoped: name or name/subpath
	parts := strings.SplitN(spec, "/", 2)
	return parts[0]
}

// scanSourceImports walks source files and extracts bare import specifiers.
// Only returns specifiers that match packages in the moduleMap.
func scanSourceImports(sourceRoot string, moduleMap map[string]string) map[string]bool {
	used := make(map[string]bool)

	filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "plz-out" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFileExt(filepath.Ext(path)) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		for _, m := range importSpecRe.FindAllStringSubmatch(string(data), -1) {
			spec := m[1]
			// Skip relative and absolute imports
			if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
				continue
			}
			// Only include if the package exists in moduleMap
			pkgName := packageNameFromSpec(spec)
			if _, ok := moduleMap[pkgName]; ok {
				used[spec] = true
			}
		}
		return nil
	})

	return used
}

// packageBuildResult holds the output of a single per-package esbuild Build.
type packageBuildResult struct {
	pkgName   string
	depCache  map[string][]byte
	importMap map[string]string
	err       error
}

// entryPointsForPackage collects esbuild entry points for a single package.
// When usedImports is nil ("all" mode), enumerates main entry + all subpath exports.
// When usedImports is non-nil ("filtered" mode), only includes specifiers found in source.
func entryPointsForPackage(pkgName, pkgDir string, usedImports map[string]bool) ([]api.EntryPoint, map[string]string) {
	absPkgDir, _ := filepath.Abs(pkgDir)

	// Only pre-bundle npm packages (those with package.json).
	if _, err := os.Stat(filepath.Join(absPkgDir, "package.json")); err != nil {
		return nil, nil
	}

	var entryPoints []api.EntryPoint
	importMap := make(map[string]string)
	seen := make(map[string]bool)

	addSpec := func(spec, subpath string) {
		if seen[spec] || strings.HasSuffix(spec, "/") {
			return
		}
		ep := common.ResolvePackageEntry(absPkgDir, subpath, "browser")
		if ep == "" && subpath == "." {
			candidate := filepath.Join(absPkgDir, "index.js")
			if _, err := os.Stat(candidate); err == nil {
				ep = candidate
			}
		}
		if ep == "" {
			return
		}
		seen[spec] = true
		entryPoints = append(entryPoints, api.EntryPoint{
			InputPath:  ep,
			OutputPath: spec,
		})
		importMap[spec] = "/@deps/" + spec + ".js"
	}

	if usedImports == nil {
		// "all" mode: main entry + all subpath exports
		addSpec(pkgName, ".")
		for _, subpath := range findSubpathExports(absPkgDir) {
			trimmed := strings.TrimPrefix(subpath, "./")
			if trimmed == "" {
				continue
			}
			addSpec(pkgName+"/"+trimmed, subpath)
		}
	} else {
		// "filtered" mode: only specifiers found in source code
		for spec := range usedImports {
			if packageNameFromSpec(spec) != pkgName {
				continue
			}
			subpath := "."
			if spec != pkgName {
				subpath = "./" + strings.TrimPrefix(spec, pkgName+"/")
			}
			addSpec(spec, subpath)
		}
	}

	return entryPoints, importMap
}

// prebundlePackage bundles a single npm package with all other packages externalized.
// Uses splitting within the package for shared internal state between subpath exports.
func prebundlePackage(pkgName, pkgDir string, usedImports map[string]bool, outdir string) packageBuildResult {
	entryPoints, importMap := entryPointsForPackage(pkgName, pkgDir, usedImports)
	if len(entryPoints) == 0 {
		return packageBuildResult{pkgName: pkgName}
	}

	// Single-package moduleMap: only the current package.
	// ModuleResolvePlugin uses this to resolve self-references.
	// UnknownExternalPlugin uses this to externalize all OTHER packages
	// (CJS require() calls get ESM shims, ESM imports get External:true).
	singlePkgMap := map[string]string{pkgName: pkgDir}

	result := api.Build(api.BuildOptions{
		EntryPointsAdvanced: entryPoints,
		Bundle:              true,
		Write:               false,
		Format:              api.FormatESModule,
		Splitting:           true,
		ChunkNames:          pkgName + "/chunk-[hash]",
		Platform:            api.PlatformBrowser,
		Target:              api.ESNext,
		Outdir:              outdir,
		LogLevel:            api.LogLevelSilent,
		IgnoreAnnotations:   true,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(singlePkgMap, "browser"),
			common.NodeBuiltinEmptyPlugin(),
			common.UnknownExternalPlugin(singlePkgMap),
		},
		Loader: depLoaders,
	})

	if len(result.Errors) > 0 {
		var msgs []string
		for _, e := range result.Errors {
			msgs = append(msgs, e.Text)
		}
		return packageBuildResult{
			pkgName: pkgName,
			err:     fmt.Errorf("%s", strings.Join(msgs, "; ")),
		}
	}

	depCache := make(map[string][]byte)
	for _, f := range result.OutputFiles {
		rel, err := filepath.Rel(outdir, f.Path)
		if err != nil {
			rel = filepath.Base(f.Path)
		}
		depCache["/@deps/"+filepath.ToSlash(rel)] = f.Contents
	}

	addCJSNamedExportsToCache(depCache)
	fixDynamicRequires(depCache)

	return packageBuildResult{
		pkgName:   pkgName,
		depCache:  depCache,
		importMap: importMap,
	}
}

// prebundleAllPackages orchestrates parallel per-package prebundling.
// Each package is bundled independently with all other packages externalized.
// Cross-package references are resolved by the browser import map at runtime.
func prebundleAllPackages(ctx context.Context, moduleMap map[string]string, usedImports map[string]bool) (map[string][]byte, map[string]string, []string) {
	outdir, _ := filepath.Abs(".esm-prebundle-tmp")

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	var mu sync.Mutex
	mergedDepCache := make(map[string][]byte)
	mergedImportMap := make(map[string]string)
	var failedPkgs []string

	for pkgName, pkgDir := range moduleMap {
		name, dir := pkgName, pkgDir
		g.Go(func() error {
			result := prebundlePackage(name, dir, usedImports, outdir)

			mu.Lock()
			defer mu.Unlock()

			if result.err != nil {
				failedPkgs = append(failedPkgs, name)
				fmt.Fprintf(os.Stderr, "  warning: skipping %s: %v\n", name, result.err)
				return nil
			}

			for k, v := range result.depCache {
				mergedDepCache[k] = v
			}
			for k, v := range result.importMap {
				mergedImportMap[k] = v
			}
			return nil
		})
	}

	g.Wait()
	return mergedDepCache, mergedImportMap, failedPkgs
}

// prebundleCacheKey computes a hash key based on the moduleconfig content
// and the set of used imports. The cache is invalidated when either changes.
func prebundleCacheKey(moduleConfigPath string, usedImports map[string]bool) string {
	h := sha256.New()
	// Hash moduleconfig content — changes when any dep is added/removed/updated
	if data, err := os.ReadFile(moduleConfigPath); err == nil {
		h.Write(data)
	}
	// Hash used imports — changes when source code adds/removes an import
	var specs []string
	for spec := range usedImports {
		specs = append(specs, spec)
	}
	sort.Strings(specs)
	for _, spec := range specs {
		h.Write([]byte(spec + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// loadPrebundleCache loads a cached pre-bundle result from disk.
func loadPrebundleCache(cacheDir string) (map[string][]byte, []byte, error) {
	importMapJSON, err := os.ReadFile(filepath.Join(cacheDir, "_importmap.json"))
	if err != nil {
		return nil, nil, err
	}

	depCache := make(map[string][]byte)
	filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() == "_importmap.json" {
			return nil
		}
		rel, _ := filepath.Rel(cacheDir, path)
		urlPath := "/@deps/" + filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err == nil {
			depCache[urlPath] = data
		}
		return nil
	})

	return depCache, importMapJSON, nil
}

// savePrebundleCache writes the pre-bundle result to disk for fast loading.
func savePrebundleCache(cacheDir string, depCache map[string][]byte, importMapJSON []byte) {
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "_importmap.json"), importMapJSON, 0644)
	for urlPath, data := range depCache {
		rel := strings.TrimPrefix(urlPath, "/@deps/")
		filePath := filepath.Join(cacheDir, rel)
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, data, 0644)
	}
}

// prebundleDeps pre-bundles npm dependencies using per-package parallel builds.
// Each package is built independently with cross-package imports externalized.
// The browser import map resolves cross-package references at runtime.
func prebundleDeps(moduleMap map[string]string, usedImports map[string]bool) (map[string][]byte, []byte, error) {
	depCache, importMap, failedPkgs := prebundleAllPackages(context.Background(), moduleMap, usedImports)

	if len(failedPkgs) > 0 {
		sort.Strings(failedPkgs)
		fmt.Fprintf(os.Stderr, "  \033[33m!\033[0m skipped %d broken deps: %s\n",
			len(failedPkgs), strings.Join(failedPkgs, ", "))
	}

	imJSON, err := json.Marshal(map[string]interface{}{
		"imports": importMap,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal import map: %w", err)
	}

	return depCache, imJSON, nil
}

// dynamicRequireRe matches __require("specifier") calls in esbuild output.
// These are generated when CJS code require()s an external package in ESM format.
// Browsers can't execute __require, so we replace them with static imports.
var dynamicRequireRe = regexp.MustCompile(`__require\("([^"]+)"\)`)

// fixDynamicRequires replaces __require("pkg") calls in esbuild output with
// static ESM imports. For each unique specifier, adds `import __ext_N from "pkg"`
// at the top and replaces all __require("pkg") with __ext_N.
//
// This fixes the "Dynamic require of X is not supported" error in browsers.
// The static import is resolved by the browser's import map at runtime.
// Using the default import gives CJS code the raw module.exports object
// (not a namespace wrapper), preserving correct CJS interop.
func fixDynamicRequires(depCache map[string][]byte) {
	for urlPath, code := range depCache {
		codeStr := string(code)
		matches := dynamicRequireRe.FindAllStringSubmatch(codeStr, -1)
		if len(matches) == 0 {
			continue
		}

		// Collect unique specifiers
		specifiers := make(map[string]string)
		counter := 0
		for _, m := range matches {
			spec := m[1]
			if _, ok := specifiers[spec]; !ok {
				specifiers[spec] = fmt.Sprintf("__ext_%d", counter)
				counter++
			}
		}

		// Build import declarations
		var imports strings.Builder
		for spec, varName := range specifiers {
			fmt.Fprintf(&imports, "import %s from %q;\n", varName, spec)
		}

		// Replace __require("X") with the variable
		result := dynamicRequireRe.ReplaceAllStringFunc(codeStr, func(match string) string {
			m := dynamicRequireRe.FindStringSubmatch(match)
			return specifiers[m[1]]
		})

		depCache[urlPath] = []byte(imports.String() + result)
	}
}

// Regexes for CJS analysis across split chunks.
var (
	// Matches `var require_xxx = __commonJS({` to find CJS wrapper declarations.
	cjsDeclRe = regexp.MustCompile(`var\s+(require_\w+)\s*=\s*__commonJS\(`)
	// Matches `exports.xxx = ` to find named CJS exports.
	cjsExportRe = regexp.MustCompile(`exports\.(\w+)\s*=`)
	// Matches `module.exports = require_xxx()` to find delegation to another wrapper.
	cjsDelegateRe = regexp.MustCompile(`module\.exports\s*=\s*(require_\w+)\(\)`)
	// Matches `export default require_xxx()` in entry files.
	defaultRequireRe = regexp.MustCompile(`export default (require_\w+)\(\)`)
)

// HTML rewriting regexes for entry point resolution.
var (
	// Matches <script type="module" src="..."> to find module script tags.
	scriptSrcRe = regexp.MustCompile(`(<script\s[^>]*type=["']module["'][^>]*\ssrc=["'])([^"']+)(["'][^>]*>)`)
	// Matches <link rel="stylesheet" href="..."> to find CSS link tags.
	cssLinkRe = regexp.MustCompile(`<link\s[^>]*rel=["']stylesheet["'][^>]*href=["'][^"']+["'][^>]*/?>`)
	// Extracts href value from a link tag.
	hrefRe = regexp.MustCompile(`href=["']([^"']+)["']`)
)

// Component detection regexes for React Fast Refresh.
var (
	// function App(   or   export default function App(   or   export function App(
	funcComponentRe = regexp.MustCompile(`(?m)^(?:export\s+(?:default\s+)?)?function\s+([A-Z][a-zA-Z0-9_]*)\s*\(`)
	// const App =   or   export const App =   followed by arrow/function
	constComponentRe = regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+([A-Z][a-zA-Z0-9_]*)\s*=`)
)

// detectComponents returns the names of likely React components in transformed JS.
func detectComponents(code string) []string {
	seen := map[string]bool{}
	var names []string
	for _, m := range funcComponentRe.FindAllStringSubmatch(code, -1) {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	for _, m := range constComponentRe.FindAllStringSubmatch(code, -1) {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	return names
}

// injectRefreshRegistration wraps transformed JS code with React Fast Refresh
// registration calls for the given component names.
func injectRefreshRegistration(code []byte, urlPath string, components []string) []byte {
	var buf strings.Builder

	// Preamble: save/override global refresh hooks
	buf.WriteString("import.meta.hot = window.__ESM_HMR__?.createContext(")
	buf.WriteString(fmt.Sprintf("%q", urlPath))
	buf.WriteString(");\n")
	buf.WriteString("var __prevReg = window.$RefreshReg$;\n")
	buf.WriteString("var __prevSig = window.$RefreshSig$;\n")
	buf.WriteString("window.$RefreshReg$ = (type, id) => window.__REACT_REFRESH__?.register(type, ")
	buf.WriteString(fmt.Sprintf("%q", urlPath+" "))
	buf.WriteString(" + id);\n")
	buf.WriteString("window.$RefreshSig$ = window.__REACT_REFRESH__?.createSignatureFunctionForTransform || (() => (t) => t);\n")

	// Original code
	buf.Write(code)
	buf.WriteString("\n")

	// Footer: register components, then restore hooks
	for _, name := range components {
		buf.WriteString(fmt.Sprintf("window.$RefreshReg$(%s, %q);\n", name, name))
	}
	buf.WriteString("window.$RefreshReg$ = __prevReg;\n")
	buf.WriteString("window.$RefreshSig$ = __prevSig;\n")
	buf.WriteString("import.meta.hot?.accept();\n")

	return []byte(buf.String())
}

// isSourceFileExt returns true if the extension is a JS/TS source file.
func isSourceFileExt(ext string) bool {
	switch ext {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
		return true
	}
	return false
}

// cjsModuleInfo holds parsed info about a single __commonJS wrapper.
type cjsModuleInfo struct {
	exports     []string // direct exports (exports.foo = ...)
	delegatesTo string   // module.exports = require_xxx() delegation
}

// addCJSNamedExportsToCache processes all files in the dep cache together.
// It scans chunks for __commonJS wrappers, traces delegation chains
// (e.g. require_react → require_react_development), and adds named
// re-exports to entry files that only have `export default require_xxx()`.
func addCJSNamedExportsToCache(depCache map[string][]byte) {
	// Pass 1: scan all files for __commonJS declarations
	cjsInfo := make(map[string]*cjsModuleInfo)

	for _, code := range depCache {
		codeStr := string(code)
		if !strings.Contains(codeStr, "__commonJS") {
			continue
		}

		// Find all __commonJS declarations and their positions
		declMatches := cjsDeclRe.FindAllStringSubmatchIndex(codeStr, -1)
		for i, match := range declMatches {
			funcName := codeStr[match[2]:match[3]]

			// Extract the block between this declaration and the next
			startIdx := match[0]
			endIdx := len(codeStr)
			if i+1 < len(declMatches) {
				endIdx = declMatches[i+1][0]
			}
			block := codeStr[startIdx:endIdx]

			info := &cjsModuleInfo{}

			// Check for delegation: module.exports = require_xxx()
			if dm := cjsDelegateRe.FindStringSubmatch(block); dm != nil {
				info.delegatesTo = dm[1]
			}

			// Collect direct exports: exports.xxx =
			exportMatches := cjsExportRe.FindAllStringSubmatch(block, -1)
			seen := make(map[string]bool)
			for _, em := range exportMatches {
				name := em[1]
				if !seen[name] && !strings.HasPrefix(name, "__") {
					info.exports = append(info.exports, name)
					seen[name] = true
				}
			}

			cjsInfo[funcName] = info
		}
	}

	// Pass 2: for each entry with `export default require_xxx()`, resolve
	// the delegation chain and add named re-exports.
	for urlPath, code := range depCache {
		codeStr := string(code)
		match := defaultRequireRe.FindStringSubmatch(codeStr)
		if match == nil {
			continue
		}

		funcName := match[1]
		names := resolveCJSExports(cjsInfo, funcName)
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)

		// Replace `export default require_xxx();` with named exports
		idx := strings.LastIndex(codeStr, "export default ")
		if idx < 0 {
			continue
		}
		rest := codeStr[idx+len("export default "):]
		semiIdx := strings.Index(rest, ";")
		if semiIdx < 0 {
			continue
		}
		expr := rest[:semiIdx]
		trailing := rest[semiIdx+1:]

		var sb strings.Builder
		sb.WriteString(codeStr[:idx])
		sb.WriteString("var __cjs_exports = ")
		sb.WriteString(expr)
		sb.WriteString(";\nexport default __cjs_exports;\n")
		sb.WriteString("export const { ")
		sb.WriteString(strings.Join(names, ", "))
		sb.WriteString(" } = __cjs_exports;\n")
		sb.WriteString(trailing)

		depCache[urlPath] = []byte(sb.String())
	}
}

// resolveCJSExports follows the delegation chain to find the actual
// CJS export names. e.g. require_react → require_react_development
// where the development module has the real exports.
func resolveCJSExports(info map[string]*cjsModuleInfo, funcName string) []string {
	visited := make(map[string]bool)
	for {
		if visited[funcName] {
			return nil // cycle
		}
		visited[funcName] = true

		ci, ok := info[funcName]
		if !ok {
			return nil
		}

		if ci.delegatesTo != "" {
			funcName = ci.delegatesTo
			continue
		}
		return ci.exports
	}
}

// findSubpathExports scans a package's package.json exports field for subpath entries.
func findSubpathExports(pkgDir string) []string {
	data, err := os.ReadFile(filepath.Join(pkgDir, "package.json"))
	if err != nil {
		return nil
	}

	var raw struct {
		Exports json.RawMessage `json:"exports"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Exports == nil {
		return nil
	}

	// Try to parse as a map with subpath keys
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw.Exports, &m); err != nil {
		return nil
	}

	var subpaths []string
	for key := range m {
		// Only subpath exports start with "./" and are not the root "."
		if strings.HasPrefix(key, "./") {
			// Skip wildcard patterns and directory mappings (trailing slash)
			if strings.Contains(key, "*") || strings.HasSuffix(key, "/") {
				continue
			}
			subpaths = append(subpaths, key)
		}
	}
	return subpaths
}

// PrebundleAll runs the full pre-bundle pipeline for all npm dependencies
// and writes the output to outDir. This is used by the "prebundle" subcommand
// at build time so Please can cache the result.
func PrebundleAll(moduleConfigPath, outDir string) error {
	moduleMap, err := common.ParseModuleConfig(moduleConfigPath)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	depCache, importMap, failedPkgs := prebundleAllPackages(context.Background(), moduleMap, nil)

	if len(failedPkgs) > 0 {
		sort.Strings(failedPkgs)
		fmt.Fprintf(os.Stderr, "  excluding broken deps: %s\n", strings.Join(failedPkgs, ", "))
	}

	imJSON, err := json.Marshal(map[string]interface{}{
		"imports": importMap,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal import map: %w", err)
	}

	return SavePrebundleDir(outDir, depCache, imJSON)
}

// PrebundlePkg pre-bundles a single npm package and writes the output to outDir.
// The moduleconfig should contain exactly one entry mapping the package name to
// its lib directory. Used by the "prebundle-pkg" subcommand for per-package
// Please rules where each dep is cached independently.
func PrebundlePkg(moduleConfigPath, outDir string) error {
	moduleMap, err := common.ParseModuleConfig(moduleConfigPath)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	if len(moduleMap) == 0 {
		return fmt.Errorf("moduleconfig is empty")
	}

	outdir, _ := filepath.Abs(".esm-prebundle-tmp")
	mergedDepCache := make(map[string][]byte)
	mergedImportMap := make(map[string]string)

	for pkgName, pkgDir := range moduleMap {
		result := prebundlePackage(pkgName, pkgDir, nil, outdir)
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "  warning: skipping %s: %v\n", pkgName, result.err)
			continue
		}
		for k, v := range result.depCache {
			mergedDepCache[k] = v
		}
		for k, v := range result.importMap {
			mergedImportMap[k] = v
		}
	}

	imJSON, err := json.Marshal(map[string]interface{}{
		"imports": mergedImportMap,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal import map: %w", err)
	}

	return SavePrebundleDir(outDir, mergedDepCache, imJSON)
}

// MergeImportmaps reads multiple importmap.json files, merges their "imports"
// objects, and writes the combined result. Used by the aggregation rule to
// merge per-package prebundle outputs.
func MergeImportmaps(files []string, outPath string) error {
	merged := make(map[string]string)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		var im struct {
			Imports map[string]string `json:"imports"`
		}
		if err := json.Unmarshal(data, &im); err != nil {
			return fmt.Errorf("parsing %s: %w", f, err)
		}
		for k, v := range im.Imports {
			merged[k] = v
		}
	}

	result, err := json.Marshal(map[string]interface{}{
		"imports": merged,
	})
	if err != nil {
		return fmt.Errorf("marshaling merged import map: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outPath, result, 0644)
}

// SavePrebundleDir writes pre-bundled deps and import map to a directory.
func SavePrebundleDir(dir string, depCache map[string][]byte, importMapJSON []byte) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "importmap.json"), importMapJSON, 0644); err != nil {
		return err
	}
	for urlPath, data := range depCache {
		rel := strings.TrimPrefix(urlPath, "/@deps/")
		filePath := filepath.Join(dir, "deps", rel)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// LoadPrebundleDir reads pre-bundled deps and import map from a directory.
func LoadPrebundleDir(dir string) (map[string][]byte, []byte, error) {
	importMapJSON, err := os.ReadFile(filepath.Join(dir, "importmap.json"))
	if err != nil {
		return nil, nil, err
	}

	depCache := make(map[string][]byte)
	depsDir := filepath.Join(dir, "deps")
	filepath.Walk(depsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(depsDir, path)
		urlPath := "/@deps/" + filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err == nil {
			depCache[urlPath] = data
		}
		return nil
	})

	return depCache, importMapJSON, nil
}

// resolveSourceFile finds the actual file for a URL path, trying various extensions.
func resolveSourceFile(sourceRoot, urlPath string) string {
	// Direct path
	full := filepath.Join(sourceRoot, filepath.FromSlash(urlPath))
	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		return full
	}

	exts := []string{".ts", ".tsx", ".js", ".jsx"}

	// If the path has an extension like .js, try replacing it with .ts/.tsx/.jsx
	// This handles <script src="/main.js"> when the actual file is main.tsx
	if curExt := filepath.Ext(full); curExt != "" {
		base := strings.TrimSuffix(full, curExt)
		for _, ext := range exts {
			if ext == curExt {
				continue
			}
			candidate := base + ext
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	// Try adding extensions to the path as-is (for extensionless paths)
	for _, ext := range exts {
		candidate := full + ext
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	// Try index files
	for _, ext := range exts {
		candidate := filepath.Join(full, "index"+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

// loaderForFile returns the esbuild loader for a given file path.
func loaderForFile(path string) api.Loader {
	ext := filepath.Ext(path)
	if loader, ok := common.Loaders[ext]; ok {
		return loader
	}
	return api.LoaderJS
}

func (s *esmServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	urlPath := r.URL.Path

	// 1. SSE endpoint
	if urlPath == "/__esm_dev_sse" {
		s.handleSSE(w, r)
		return
	}

	// 2. Proxy matching
	for _, prefix := range s.proxyPrefixes {
		if strings.HasPrefix(urlPath, prefix) {
			fmt.Printf("  \033[2m[proxy] %s %s\033[0m\n", r.Method, urlPath)
			s.proxies[prefix].ServeHTTP(w, r)
			return
		}
	}

	// 3. Pre-bundled deps
	if strings.HasPrefix(urlPath, "/@deps/") {
		if data, ok := s.depCache[urlPath]; ok {
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(data)
			fmt.Printf("  \033[2m[dep] %s %s → 200 (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
			return
		}
		http.NotFound(w, r)
		return
	}

	// 4. HTML files — inject import map + live reload
	if strings.HasSuffix(urlPath, ".html") || urlPath == "/" {
		s.handleHTML(w, r, start)
		return
	}

	// 5. JS/TS/JSX/TSX source files — on-demand transform
	ext := filepath.Ext(urlPath)
	isSourceExt := ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs"
	if isSourceExt || ext == "" {
		s.handleSource(w, r, urlPath, start)
		return
	}

	// 6. CSS files — serve as JS style injector when imported as ES module.
	// Browsers send Sec-Fetch-Dest: script for `import "./style.css"` and
	// Sec-Fetch-Dest: style for `<link rel="stylesheet">`. The ?module query
	// param serves as a fallback for non-browser clients.
	if ext == ".css" {
		fetchDest := r.Header.Get("Sec-Fetch-Dest")
		if fetchDest == "script" || r.URL.Query().Get("module") != "" {
			s.handleCSSModule(w, r, urlPath, start)
			return
		}
	}

	// 6b. Asset files — serve as JS module when imported as ES module.
	if isAssetExt(ext) {
		fetchDest := r.Header.Get("Sec-Fetch-Dest")
		if fetchDest == "script" || r.URL.Query().Get("module") != "" {
			s.handleAssetModule(w, r, urlPath, start)
			return
		}
	}

	// 7. Static files from servedir or packageRoot
	filePath := filepath.Join(s.sourceRoot, filepath.FromSlash(urlPath))
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		fmt.Printf("  \033[2m[static] %s %s → 200 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}
	if s.packageRoot != s.sourceRoot {
		filePath = filepath.Join(s.packageRoot, filepath.FromSlash(urlPath))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, filePath)
			fmt.Printf("  \033[2m[static] %s %s → 200 (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
			return
		}
	}

	// 8. SPA fallback → index.html with import map injection
	s.handleHTML(w, r, start)
}

func (s *esmServer) handleHTML(w http.ResponseWriter, r *http.Request, start time.Time) {
	htmlPath := r.URL.Path
	if htmlPath == "/" || !strings.HasSuffix(htmlPath, ".html") {
		htmlPath = "/index.html"
	}

	filePath := filepath.Join(s.sourceRoot, filepath.FromSlash(htmlPath))
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[req] %s %s → 404 (%dms)\033[0m\n",
			r.Method, r.URL.Path, time.Since(start).Milliseconds())
		return
	}

	html := string(data)

	// Rewrite HTML when packageRoot differs from sourceRoot.
	// - Script src that doesn't resolve in either root → replace with entry URL path
	// - CSS link href that doesn't resolve → remove (CSS is injected via JS modules)
	if s.packageRoot != s.sourceRoot {
		html = scriptSrcRe.ReplaceAllStringFunc(html, func(match string) string {
			parts := scriptSrcRe.FindStringSubmatch(match)
			if parts == nil {
				return match
			}
			src := parts[2]
			// Check if file exists in sourceRoot or packageRoot
			if resolveSourceFile(s.sourceRoot, src) != "" || resolveSourceFile(s.packageRoot, src) != "" {
				return match
			}
			// Replace with actual entry point path
			return parts[1] + s.entryURLPath + parts[3]
		})

		html = cssLinkRe.ReplaceAllStringFunc(html, func(match string) string {
			hrefMatch := hrefRe.FindStringSubmatch(match)
			if hrefMatch == nil {
				return match
			}
			href := hrefMatch[1]
			// Check if CSS file exists in sourceRoot or packageRoot
			cssPath := filepath.Join(s.sourceRoot, filepath.FromSlash(href))
			if _, err := os.Stat(cssPath); err == nil {
				return match
			}
			cssPath = filepath.Join(s.packageRoot, filepath.FromSlash(href))
			if _, err := os.Stat(cssPath); err == nil {
				return match
			}
			// Remove the tag — CSS is injected via JS modules in ESM dev mode
			return ""
		})
	}

	// Inject import map and live reload / HMR script before </head>
	var clientScript string
	if s.hasRefresh {
		clientScript = refreshInitScript + "\n" + hmrClientScript
	} else {
		clientScript = liveReloadScript
	}
	injection := fmt.Sprintf(`<script type="importmap">%s</script>
%s`, string(s.importMapJSON), clientScript)

	if idx := strings.Index(html, "</head>"); idx >= 0 {
		html = html[:idx] + injection + "\n" + html[idx:]
	} else if idx := strings.Index(html, "<body"); idx >= 0 {
		html = html[:idx] + injection + "\n" + html[idx:]
	} else {
		html = injection + "\n" + html
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(html))
	fmt.Printf("  \033[2m[html] %s %s → 200 (%dms)\033[0m\n",
		r.Method, r.URL.Path, time.Since(start).Milliseconds())
}

func (s *esmServer) handleSource(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	resolved := resolveSourceFile(s.packageRoot, urlPath)
	if resolved == "" && s.packageRoot != s.sourceRoot {
		resolved = resolveSourceFile(s.sourceRoot, urlPath)
	}
	if resolved == "" {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[req] %s %s → 404 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Check cache
	info, err := os.Stat(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if cached, ok := s.transCache.Load(resolved); ok {
		entry := cached.(*transformEntry)
		if entry.modTime.Equal(info.ModTime()) {
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(entry.code)
			fmt.Printf("  \033[2m[cached] %s %s → 200 (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
			return
		}
	}

	// Read and transform
	src, err := os.ReadFile(resolved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	loader := loaderForFile(resolved)

	transformOpts := api.TransformOptions{
		Loader:            loader,
		Format:            api.FormatESModule,
		Target:            api.ESNext,
		JSX:               api.JSXAutomatic,
		Sourcemap:         api.SourceMapInline,
		SourcesContent:    api.SourcesContentInclude,
		Sourcefile:        urlPath,
		Define:            s.define,
		LogLevel:          api.LogLevelWarning,
	}
	if s.tsconfig != "" {
		// Read tsconfig for JSX settings
		tsconfigData, err := os.ReadFile(s.tsconfig)
		if err == nil {
			transformOpts.TsconfigRaw = string(tsconfigData)
		}
	}

	result := api.Transform(string(src), transformOpts)
	if len(result.Errors) > 0 {
		// Return error as a JS module that logs the error
		errMsg := result.Errors[0].Text
		errJS := fmt.Sprintf(`console.error("[esm-dev] Transform error in %s:\\n%s");`, urlPath, strings.ReplaceAll(errMsg, `"`, `\"`))
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK) // Browser needs 200 to execute the error reporter
		w.Write([]byte(errJS))
		fmt.Printf("  \033[31m[error] %s %s: %s\033[0m\n", r.Method, urlPath, errMsg)
		return
	}

	code := result.Code

	// Inject React Fast Refresh registration if enabled and not the entry file.
	if s.hasRefresh && urlPath != s.entryURLPath {
		components := detectComponents(string(code))
		s.componentFiles.Store(resolved, len(components) > 0)
		if len(components) > 0 {
			code = injectRefreshRegistration(code, urlPath, components)
		}
	}

	// Cache the result
	s.transCache.Store(resolved, &transformEntry{
		code:    code,
		modTime: info.ModTime(),
	})

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(code)
	fmt.Printf("  \033[2m[transform] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

func (s *esmServer) handleCSSModule(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	filePath := filepath.Join(s.packageRoot, filepath.FromSlash(urlPath))
	if _, err := os.Stat(filePath); err != nil && s.packageRoot != s.sourceRoot {
		filePath = filepath.Join(s.sourceRoot, filepath.FromSlash(urlPath))
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// JSON-encode the CSS content for safe embedding in JS
	cssJSON, err := json.Marshal(string(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	js := fmt.Sprintf(cssModuleTemplate, urlPath, string(cssJSON))
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(js))
	fmt.Printf("  \033[2m[css-module] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

func (s *esmServer) handleAssetModule(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	js := fmt.Sprintf(assetModuleTemplate, urlPath)
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(js))
	fmt.Printf("  \033[2m[asset-module] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

func (s *esmServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ch := make(chan sseEvent, 1)
	s.sseMu.Lock()
	s.clients[ch] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.clients, ch)
		s.sseMu.Unlock()
	}()

	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// broadcast sends an event to all connected SSE clients.
func (s *esmServer) broadcast(evt sseEvent) {
	s.sseMu.Lock()
	for ch := range s.clients {
		select {
		case ch <- evt:
		default:
		}
	}
	s.sseMu.Unlock()
}

// watchFiles polls the source tree for changes and broadcasts SSE events.
func (s *esmServer) watchFiles() {
	mtimes := make(map[string]time.Time)

	// Initial scan
	s.walkSourceTree(mtimes)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		newMtimes := make(map[string]time.Time)
		s.walkSourceTree(newMtimes)

		if !s.hasRefresh {
			// No HMR support — simple change detection with full reload
			changed := false
			for path, newMt := range newMtimes {
				if oldMt, ok := mtimes[path]; !ok || !oldMt.Equal(newMt) {
					changed = true
					s.transCache.Delete(path)
				}
			}
			for path := range mtimes {
				if _, ok := newMtimes[path]; !ok {
					changed = true
					s.transCache.Delete(path)
				}
			}
			if changed {
				mtimes = newMtimes
				s.broadcast(sseEvent{Type: "change"})
			}
			continue
		}

		// HMR-aware change classification
		var hmrFiles []string
		var cssFiles []string
		needFullReload := false

		for path, newMt := range newMtimes {
			if oldMt, ok := mtimes[path]; !ok || !oldMt.Equal(newMt) {
				s.transCache.Delete(path)

				rel, err := filepath.Rel(s.packageRoot, path)
				if err != nil {
					needFullReload = true
					continue
				}
				relPath := "/" + filepath.ToSlash(rel)
				ext := filepath.Ext(path)

				switch {
				case ext == ".css":
					cssFiles = append(cssFiles, relPath)
				case relPath == s.entryURLPath:
					needFullReload = true
				case isSourceFileExt(ext):
					if isComp, ok := s.componentFiles.Load(path); ok && isComp.(bool) {
						hmrFiles = append(hmrFiles, relPath)
					} else {
						needFullReload = true
					}
				default:
					needFullReload = true
				}
			}
		}
		// Check for deleted files
		for path := range mtimes {
			if _, ok := newMtimes[path]; !ok {
				s.transCache.Delete(path)
				s.componentFiles.Delete(path)
				needFullReload = true
			}
		}

		if needFullReload {
			mtimes = newMtimes
			s.broadcast(sseEvent{Type: "full-reload"})
		} else if len(hmrFiles) > 0 || len(cssFiles) > 0 {
			mtimes = newMtimes
			if len(hmrFiles) > 0 {
				s.broadcast(sseEvent{Type: "hmr-update", Files: hmrFiles})
			}
			if len(cssFiles) > 0 {
				s.broadcast(sseEvent{Type: "css-update", Files: cssFiles})
			}
		}
	}
}

// walkSourceTree collects file mtimes, skipping hidden dirs, node_modules, plz-out.
// Walks packageRoot (which may be a parent of sourceRoot) to cover both source and static files.
func (s *esmServer) walkSourceTree(mtimes map[string]time.Time) {
	filepath.Walk(s.packageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "plz-out" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".css", ".html", ".json":
			mtimes[path] = info.ModTime()
		}
		return nil
	})
}

// getLocalIPs returns non-loopback IPv4 addresses.
func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}

// parseTsconfigPaths reads a tsconfig.json and returns import map entries for
// path aliases. Wildcard entries like "@/*": ["./src/*"] produce prefix mappings
// "@/" → "/src/". Exact entries like "~utils": ["./src/utils"] produce exact
// mappings. All paths are resolved relative to baseUrl and made URL-absolute
// relative to packageRoot.
func parseTsconfigPaths(tsconfigPath, packageRoot string) map[string]string {
	data, err := os.ReadFile(tsconfigPath)
	if err != nil {
		return nil
	}

	var tsconfig struct {
		CompilerOptions struct {
			BaseUrl string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &tsconfig); err != nil {
		return nil
	}

	if len(tsconfig.CompilerOptions.Paths) == 0 {
		return nil
	}

	baseUrl := tsconfig.CompilerOptions.BaseUrl
	if baseUrl == "" {
		baseUrl = "."
	}
	// Resolve baseUrl relative to tsconfig directory
	tsconfigDir := filepath.Dir(tsconfigPath)
	absBaseUrl := filepath.Join(tsconfigDir, baseUrl)

	entries := make(map[string]string)
	for alias, targets := range tsconfig.CompilerOptions.Paths {
		if len(targets) == 0 {
			continue
		}
		target := targets[0] // Use the first mapping

		if strings.HasSuffix(alias, "/*") && strings.HasSuffix(target, "/*") {
			// Wildcard: "@/*" → "./src/*" becomes "@/" → "/src/"
			prefix := strings.TrimSuffix(alias, "*")
			targetDir := strings.TrimSuffix(target, "*")
			absTarget := filepath.Join(absBaseUrl, targetDir)
			rel, err := filepath.Rel(packageRoot, absTarget)
			if err != nil {
				continue
			}
			entries[prefix] = "/" + filepath.ToSlash(rel)
		} else {
			// Exact: "~utils" → "./src/utils/index.ts"
			absTarget := filepath.Join(absBaseUrl, target)
			rel, err := filepath.Rel(packageRoot, absTarget)
			if err != nil {
				continue
			}
			entries[alias] = "/" + filepath.ToSlash(rel)
		}
	}

	return entries
}

// Run starts the ESM dev server.
func Run(args Args) error {
	moduleMap, err := common.ParseModuleConfig(args.ModuleConfig)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	port := args.Port
	if port == 0 {
		port = 3000
	}

	servedir := args.Servedir
	if servedir == "" {
		servedir = "."
	}
	absServedir, _ := filepath.Abs(servedir)

	// Compute package root for source file resolution (defaults to servedir)
	absPackageRoot := absServedir
	if args.Root != "" {
		absPackageRoot, _ = filepath.Abs(args.Root)
	}

	// Parse defines and env
	define := common.ParseDefines(args.Define)
	if args.EnvFile != "" {
		envDefines, err := common.LoadEnvFiles(args.EnvFile, "development", args.EnvPrefix)
		if err != nil {
			return fmt.Errorf("failed to load env files: %w", err)
		}
		for k, v := range envDefines {
			if _, ok := define[k]; !ok {
				define[k] = v
			}
		}
	}
	common.MergeEnvDefines(define, "development")

	prebundleStart := time.Now()
	var depCache map[string][]byte
	var importMapJSON []byte

	if args.PrebundleDir != "" {
		// Build-time pre-bundled: load from directory (instant).
		depCache, importMapJSON, err = LoadPrebundleDir(args.PrebundleDir)
		if err != nil {
			return fmt.Errorf("failed to load prebundle dir %s: %w", args.PrebundleDir, err)
		}
		var imData struct{ Imports map[string]string }
		json.Unmarshal(importMapJSON, &imData)
		fmt.Printf("  \033[2mLoaded %d deps from prebundle dir in %dms\033[0m\n",
			len(imData.Imports), time.Since(prebundleStart).Milliseconds())
	} else {
		// Runtime fallback: scan sources and pre-bundle on the fly.
		usedImports := scanSourceImports(absPackageRoot, moduleMap)
		// Always include react-refresh if available (injected by HMR scripts).
		if _, ok := moduleMap["react-refresh"]; ok {
			usedImports["react-refresh"] = true
		}

		cacheKey := prebundleCacheKey(args.ModuleConfig, usedImports)
		cacheDir := filepath.Join(".esm-dev-cache", cacheKey)

		if dc, im, loadErr := loadPrebundleCache(cacheDir); loadErr == nil {
			depCache = dc
			importMapJSON = im
			var imData struct{ Imports map[string]string }
			json.Unmarshal(importMapJSON, &imData)
			fmt.Printf("  \033[2mLoaded %d deps from cache in %dms\033[0m\n",
				len(imData.Imports), time.Since(prebundleStart).Milliseconds())
		} else {
			fmt.Printf("  \033[2mPre-bundling dependencies...\033[0m\n")
			depCache, importMapJSON, err = prebundleDeps(moduleMap, usedImports)
			if err != nil {
				return fmt.Errorf("failed to pre-bundle dependencies: %w", err)
			}
			// Save cache (clean old entries first)
			os.RemoveAll(".esm-dev-cache")
			savePrebundleCache(cacheDir, depCache, importMapJSON)
			var imData struct{ Imports map[string]string }
			json.Unmarshal(importMapJSON, &imData)
			fmt.Printf("  \033[2mPre-bundled %d deps in %dms\033[0m\n",
				len(imData.Imports), time.Since(prebundleStart).Milliseconds())
		}
	}

	// Merge tsconfig path aliases into the import map (lower priority than npm deps)
	if args.Tsconfig != "" {
		if pathAliases := parseTsconfigPaths(args.Tsconfig, absPackageRoot); len(pathAliases) > 0 {
			var imData struct {
				Imports map[string]string `json:"imports"`
			}
			json.Unmarshal(importMapJSON, &imData)
			if imData.Imports == nil {
				imData.Imports = make(map[string]string)
			}
			for alias, target := range pathAliases {
				if _, exists := imData.Imports[alias]; !exists {
					imData.Imports[alias] = target
				}
			}
			importMapJSON, _ = json.Marshal(imData)
		}
	}

	// Parse proxies
	proxies, proxyPrefixes := parseProxies(args.Proxy)

	// Detect react-refresh in pre-bundled deps
	hasRefresh := false
	for urlPath := range depCache {
		if strings.Contains(urlPath, "react-refresh") {
			hasRefresh = true
			break
		}
	}

	// Normalize entry point to URL path relative to packageRoot
	absEntry, _ := filepath.Abs(args.Entry)
	entryRel, _ := filepath.Rel(absPackageRoot, absEntry)
	entryURLPath := "/" + filepath.ToSlash(entryRel)

	server := &esmServer{
		sourceRoot:    absServedir,
		packageRoot:   absPackageRoot,
		depCache:      depCache,
		importMapJSON: importMapJSON,
		clients:       make(map[chan sseEvent]struct{}),
		proxies:       proxies,
		proxyPrefixes: proxyPrefixes,
		define:        define,
		tsconfig:      args.Tsconfig,
		hasRefresh:    hasRefresh,
		entryURLPath:  entryURLPath,
	}

	// Start file watcher
	go server.watchFiles()

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: server,
	}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Print banner
	fmt.Printf("\n  \033[1;36mPLEASE_JS ESM\033[0m  dev server ready\n")
	if hasRefresh {
		fmt.Printf("  \033[1;35mHMR\033[0m  React Fast Refresh enabled\n")
	}
	fmt.Printf("\n  \033[36m➜\033[0m  \033[1mLocal:\033[0m   http://localhost:\033[1m%d\033[0m/\n", port)
	for _, ip := range getLocalIPs() {
		fmt.Printf("  \033[36m➜\033[0m  \033[2mNetwork: http://%s:%d/\033[0m\n", ip, port)
	}
	fmt.Println()

	// Block until Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	httpServer.Close()
	return nil
}
