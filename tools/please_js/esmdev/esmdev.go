package esmdev

import (
	"crypto/tls"
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
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evanw/esbuild/pkg/api"

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
	sourceRoot     string
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

// prebundleDeps pre-bundles all npm dependencies in a single esbuild Build
// with code splitting enabled. This allows CJS cross-dep require() calls
// (e.g. react-dom requiring react) to be resolved at bundle time, while
// sharing code across deps via split chunks to avoid duplication.
func prebundleDeps(moduleMap map[string]string) (map[string][]byte, []byte, error) {
	depCache := make(map[string][]byte)
	importMap := make(map[string]string)

	// Collect entry points for all npm packages
	var entryPoints []api.EntryPoint

	for name, pkgDir := range moduleMap {
		absPkgDir, _ := filepath.Abs(pkgDir)

		// Only pre-bundle npm packages (those with package.json).
		if _, err := os.Stat(filepath.Join(absPkgDir, "package.json")); err != nil {
			continue
		}

		// Main entry
		entryPoint := common.ResolvePackageEntry(absPkgDir, ".", "browser")
		if entryPoint == "" {
			candidate := filepath.Join(absPkgDir, "index.js")
			if _, err := os.Stat(candidate); err == nil {
				entryPoint = candidate
			} else {
				continue
			}
		}

		entryPoints = append(entryPoints, api.EntryPoint{
			InputPath:  entryPoint,
			OutputPath: name,
		})
		importMap[name] = "/@deps/" + name + ".js"

		// Subpath exports (e.g., react-dom/client)
		subpaths := findSubpathExports(absPkgDir)
		for _, sub := range subpaths {
			subEntry := common.ResolvePackageEntry(absPkgDir, sub, "browser")
			if subEntry == "" {
				continue
			}
			subImport := name + "/" + strings.TrimPrefix(sub, "./")
			entryPoints = append(entryPoints, api.EntryPoint{
				InputPath:  subEntry,
				OutputPath: subImport,
			})
			importMap[subImport] = "/@deps/" + subImport + ".js"
		}
	}

	if len(entryPoints) == 0 {
		imJSON, _ := json.Marshal(map[string]interface{}{"imports": importMap})
		return depCache, imJSON, nil
	}

	// Single esbuild Build with all entry points and splitting.
	// No externals — cross-dep require() calls (react-dom → react) are
	// resolved at bundle time. Splitting shares code via chunks.
	outdir, _ := filepath.Abs(".esm-dev-deps")
	result := api.Build(api.BuildOptions{
		EntryPointsAdvanced: entryPoints,
		Bundle:              true,
		Write:               false,
		Format:              api.FormatESModule,
		Platform:            api.PlatformBrowser,
		Target:              api.ESNext,
		Splitting:           true,
		ChunkNames:          "chunk-[hash]",
		Outdir:              outdir,
		LogLevel:            api.LogLevelWarning,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap, "browser"),
			common.NodeBuiltinEmptyPlugin(),
		},
		Loader: depLoaders,
	})

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "warning: pre-bundle error: %s\n", e.Text)
		}
	}

	// Map all output files (entries + shared chunks) to /@deps/ paths
	for _, f := range result.OutputFiles {
		rel, err := filepath.Rel(outdir, f.Path)
		if err != nil {
			rel = filepath.Base(f.Path)
		}
		urlPath := "/@deps/" + filepath.ToSlash(rel)
		depCache[urlPath] = f.Contents
	}

	// Post-process: add named re-exports for CJS modules.
	// With splitting, CJS code may live in chunks while entries just have
	// `export default require_xxx()`. We analyze all files together to
	// trace the __commonJS delegation chains and find the real export names.
	addCJSNamedExportsToCache(depCache)

	// Build import map JSON
	imJSON, err := json.Marshal(map[string]interface{}{
		"imports": importMap,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal import map: %w", err)
	}

	return depCache, imJSON, nil
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
			// Skip wildcard patterns
			if strings.Contains(key, "*") {
				continue
			}
			subpaths = append(subpaths, key)
		}
	}
	return subpaths
}

// resolveSourceFile finds the actual file for a URL path, trying various extensions.
func resolveSourceFile(sourceRoot, urlPath string) string {
	// Direct path
	full := filepath.Join(sourceRoot, filepath.FromSlash(urlPath))
	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		return full
	}

	// Strip extension and try alternatives
	exts := []string{".ts", ".tsx", ".js", ".jsx"}

	// Try adding extensions to the path as-is
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

	// 7. Static files from servedir
	filePath := filepath.Join(s.sourceRoot, filepath.FromSlash(urlPath))
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		fmt.Printf("  \033[2m[static] %s %s → 200 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
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
	resolved := resolveSourceFile(s.sourceRoot, urlPath)
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
	filePath := filepath.Join(s.sourceRoot, filepath.FromSlash(urlPath))
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

				rel, err := filepath.Rel(s.sourceRoot, path)
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
func (s *esmServer) walkSourceTree(mtimes map[string]time.Time) {
	filepath.Walk(s.sourceRoot, func(path string, info os.FileInfo, err error) error {
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

	// Pre-bundle dependencies
	fmt.Printf("  \033[2mPre-bundling dependencies...\033[0m\n")
	prebundleStart := time.Now()
	depCache, importMapJSON, err := prebundleDeps(moduleMap)
	if err != nil {
		return fmt.Errorf("failed to pre-bundle dependencies: %w", err)
	}
	fmt.Printf("  \033[2mPre-bundled %d deps in %dms\033[0m\n",
		len(depCache), time.Since(prebundleStart).Milliseconds())

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

	// Normalize entry point to URL path relative to servedir
	absEntry, _ := filepath.Abs(args.Entry)
	entryRel, _ := filepath.Rel(absServedir, absEntry)
	entryURLPath := "/" + filepath.ToSlash(entryRel)

	server := &esmServer{
		sourceRoot:    absServedir,
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
