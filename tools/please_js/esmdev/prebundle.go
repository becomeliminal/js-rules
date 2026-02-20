package esmdev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
	"golang.org/x/sync/errgroup"

	"tools/please_js/common"
)

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
func prebundlePackage(pkgName, pkgDir string, usedImports map[string]bool, outdir string, define map[string]string, nodePath string, fullModuleMap ...map[string]string) packageBuildResult {
	entryPoints, importMap := entryPointsForPackage(pkgName, pkgDir, usedImports)
	if len(entryPoints) == 0 {
		return packageBuildResult{pkgName: pkgName}
	}

	// Single-package moduleMap: only the current package.
	// ModuleResolvePlugin uses this to resolve self-references.
	// UnknownExternalPlugin uses this to externalize all OTHER packages
	// (CJS require() calls get ESM shims, ESM imports get External:true).
	singlePkgMap := map[string]string{pkgName: pkgDir}

	// Include nested node_modules — npm installs packages here only for
	// version conflicts with the hoisted copy. These must be bundled into
	// the parent, not externalized (which would resolve to the wrong version).
	absPkgDir, _ := filepath.Abs(pkgDir)
	nestedNM := filepath.Join(absPkgDir, "node_modules")
	if entries, err := os.ReadDir(nestedNM); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, "@") {
				scopeEntries, _ := os.ReadDir(filepath.Join(nestedNM, name))
				for _, se := range scopeEntries {
					if se.IsDir() {
						singlePkgMap[name+"/"+se.Name()] = filepath.Join(nestedNM, name, se.Name())
					}
				}
			} else {
				singlePkgMap[name] = filepath.Join(nestedNM, name)
			}
		}
	}

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
		Define:              define,
		IgnoreAnnotations:   true,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(singlePkgMap, "browser"),
			common.NodeBuiltinEmptyPlugin(fullModuleMap...),
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

	// Detect CJS exports via Node.js when available
	var knownExports map[string][]string
	if nodePath != "" {
		entryMap := make(map[string]string)
		for _, ep := range entryPoints {
			entryMap[ep.OutputPath] = ep.InputPath
		}
		nodeExports, _ := detectCJSExports(nodePath, entryMap)
		if nodeExports != nil {
			knownExports = make(map[string][]string)
			for spec, exports := range nodeExports {
				if exports != nil {
					urlPath := importMap[spec]
					knownExports[urlPath] = exports
				}
			}
		}
	}

	addCJSNamedExportsToCache(depCache, knownExports)
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
func prebundleAllPackages(ctx context.Context, moduleMap map[string]string, usedImports map[string]bool, define map[string]string, nodePath string) (map[string][]byte, map[string]string, []string) {
	outdir, _ := filepath.Abs(".esm-prebundle-tmp")

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	var mu sync.Mutex
	mergedDepCache := make(map[string][]byte)
	mergedImportMap := make(map[string]string)
	var failedPkgs []string

	for pkgName, pkgDir := range moduleMap {
		if isLocalLibrary(pkgDir) {
			continue // local js_library targets are served via /@lib/, not pre-bundled
		}
		name, dir := pkgName, pkgDir
		g.Go(func() error {
			result := prebundlePackage(name, dir, usedImports, outdir, define, nodePath, moduleMap)

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
	addPrefixImportMapEntries(mergedImportMap)
	return mergedDepCache, mergedImportMap, failedPkgs
}

// addPrefixImportMapEntries adds trailing-slash prefix entries to the import map
// for each package. This allows the browser to resolve deep subpath imports
// (e.g., "use-sync-external-store/shim/with-selector.js") via prefix matching
// when the exact specifier isn't pre-bundled. Per the import map spec, exact
// entries always win over prefix entries, so already-prebundled subpaths are
// unaffected.
func addPrefixImportMapEntries(importMap map[string]string) {
	pkgs := make(map[string]bool)
	for spec, target := range importMap {
		if strings.HasSuffix(spec, "/") {
			continue // already a prefix entry
		}
		// Skip local library entries — their multi-segment names (e.g. "common/js/ui")
		// would be corrupted by packageNameFromSpec into "common".
		if strings.HasPrefix(target, "/@lib/") {
			continue
		}
		pkgs[packageNameFromSpec(spec)] = true
	}
	for pkg := range pkgs {
		prefixKey := pkg + "/"
		if _, exists := importMap[prefixKey]; !exists {
			importMap[prefixKey] = "/@deps/" + pkg + "/"
		}
	}
}

// prebundleDeps pre-bundles npm dependencies using per-package parallel builds.
// Each package is built independently with cross-package imports externalized.
// The browser import map resolves cross-package references at runtime.
func prebundleDeps(moduleMap map[string]string, usedImports map[string]bool, define map[string]string) (map[string][]byte, []byte, error) {
	depCache, importMap, failedPkgs := prebundleAllPackages(context.Background(), moduleMap, usedImports, define, "")

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
