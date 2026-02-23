package esmdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

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

// PrebundleAll runs the full pre-bundle pipeline for all npm dependencies
// and writes the output to outDir. This is used by the "prebundle" subcommand
// at build time so Please can cache the result.
func PrebundleAll(moduleConfigPath, outDir string) error {
	moduleMap, err := common.ParseModuleConfig(moduleConfigPath)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	define := make(map[string]string)
	common.MergeEnvDefines(define, "development")
	depCache, importMap, failedPkgs := prebundleAllPackages(context.Background(), moduleMap, nil, define, "")

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
func PrebundlePkg(moduleConfigPath, outDir, nodePath string) error {
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
	define := make(map[string]string)
	common.MergeEnvDefines(define, "development")

	for pkgName, pkgDir := range moduleMap {
		if isLocalLibrary(pkgDir) {
			continue // local js_library targets are not pre-bundled
		}
		result := prebundlePackage(pkgName, pkgDir, nil, outdir, define, nodePath, moduleMap)
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

	addPrefixImportMapEntries(mergedImportMap)

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
//
// When moduleConfigPath and depsDir are non-empty, it also scans the bundled
// .js files in depsDir for bare import specifiers that aren't in the merged
// import map, and bundles any missing packages/subpaths so the dev server
// doesn't need on-demand fallback for transitive deps.
func MergeImportmaps(files []string, outPath, moduleConfigPath, depsDir string) error {
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

	addPrefixImportMapEntries(merged)

	// Scan bundled deps for bare imports missing from the import map.
	if moduleConfigPath != "" && depsDir != "" {
		if err := fillMissingDeps(merged, moduleConfigPath, depsDir); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: fill missing deps: %v\n", err)
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

// fillMissingDeps scans all .js files in depsDir for bare import specifiers,
// finds those missing from the import map, and bundles them. Uses a worklist
// (BFS) to follow transitive dependency chains to completion.
func fillMissingDeps(importMap map[string]string, moduleConfigPath, depsDir string) error {
	moduleMap, err := common.ParseModuleConfig(moduleConfigPath)
	if err != nil {
		return fmt.Errorf("parse moduleconfig: %w", err)
	}
	if len(moduleMap) == 0 {
		return nil
	}
	define := make(map[string]string)
	common.MergeEnvDefines(define, "development")
	outdir, _ := filepath.Abs(".esm-prebundle-tmp")

	// visited tracks packages already processed or in the import map.
	visited := make(map[string]bool)
	for spec := range importMap {
		visited[packageNameFromSpec(spec)] = true
	}

	// Seed worklist by scanning all .js files in depsDir.
	var worklist []string
	filepath.Walk(depsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		worklist = append(worklist, extractMissingPkgs(data, moduleMap, visited)...)
		return nil
	})

	// BFS: bundle missing packages, discovering transitive deps as we go.
	for len(worklist) > 0 {
		pkgName := worklist[0]
		worklist = worklist[1:]
		if visited[pkgName] {
			continue
		}
		visited[pkgName] = true

		pkgDir := moduleMap[pkgName]
		result := prebundlePackage(pkgName, pkgDir, nil, outdir, define, "", moduleMap)
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "  warning: skipping missing dep %s: %v\n", pkgName, result.err)
			continue
		}

		for k, v := range result.importMap {
			importMap[k] = v
		}
		// Write bundled files to depsDir and scan for more missing packages.
		for urlPath, data := range result.depCache {
			rel := strings.TrimPrefix(urlPath, "/@deps/")
			filePath := filepath.Join(depsDir, rel)
			os.MkdirAll(filepath.Dir(filePath), 0755)
			os.WriteFile(filePath, data, 0644)
			if strings.HasSuffix(rel, ".js") {
				worklist = append(worklist, extractMissingPkgs(data, moduleMap, visited)...)
			}
		}
	}

	addPrefixImportMapEntries(importMap)

	// Bundle missing subpaths — single pass is sufficient since subpath
	// bundling doesn't introduce new package-level dependencies.
	var missingSubpaths []string
	seen := make(map[string]bool)
	filepath.Walk(depsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range importSpecRe.FindAllStringSubmatch(string(data), -1) {
			spec := m[1]
			if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || seen[spec] {
				continue
			}
			if _, ok := importMap[spec]; ok {
				continue
			}
			pkgName := resolveModuleName(spec, moduleMap)
			if spec == pkgName {
				continue // whole-package missing would have been caught by BFS
			}
			pkgDir, ok := moduleMap[pkgName]
			if !ok || isLocalLibrary(pkgDir) {
				continue
			}
			seen[spec] = true
			missingSubpaths = append(missingSubpaths, spec)
		}
		return nil
	})

	sort.Strings(missingSubpaths)
	for _, spec := range missingSubpaths {
		pkgName := resolveModuleName(spec, moduleMap)
		pkgDir := moduleMap[pkgName]
		code, err := bundleSubpathViaStdin(spec, pkgName, pkgDir, moduleMap, define)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: skipping missing subpath %s: %v\n", spec, err)
			continue
		}
		urlPath := "/@deps/" + spec + ".js"
		importMap[spec] = urlPath
		filePath := filepath.Join(depsDir, spec+".js")
		os.MkdirAll(filepath.Dir(filePath), 0755)
		os.WriteFile(filePath, code, 0644)
	}

	return nil
}

// bundleSubpathViaStdin bundles a bare specifier for on-demand serving.
// Prefers resolving to a direct file and using it as an entry point, which
// preserves all exports including default. Falls back to stdin with
// `export * from "spec"` when resolution fails (but note: export * does NOT
// re-export default exports per the ES spec).
func bundleSubpathViaStdin(spec, pkgName, pkgDir string, moduleMap map[string]string, define map[string]string) ([]byte, error) {
	singlePkgMap := map[string]string{pkgName: pkgDir}
	absPkgDir, _ := filepath.Abs(pkgDir)

	// Try to resolve the subpath to an actual file so we can use it as a
	// direct entry point. This preserves all exports including default,
	// unlike `export * from "spec"` which strips default exports.
	subpath := "./" + strings.TrimPrefix(spec, pkgName+"/")
	resolved := common.ResolvePackageEntry(absPkgDir, subpath, "browser")
	if resolved == "" {
		resolved = resolveSubpathFile(absPkgDir, subpath)
	}

	if resolved != "" {
		result := api.Build(api.BuildOptions{
			EntryPoints: []string{resolved},
			Bundle:      true,
			Write:       false,
			Format:      api.FormatESModule,
			Platform:    api.PlatformBrowser,
			Target:      api.ESNext,
			LogLevel:    api.LogLevelSilent,
			Define:      define,
			Plugins: []api.Plugin{
				common.ModuleResolvePlugin(singlePkgMap, "browser"),
				common.NodeBuiltinEmptyPlugin(moduleMap),
				common.UnknownExternalPlugin(singlePkgMap),
			},
			Loader: depLoaders,
		})
		if len(result.Errors) == 0 && len(result.OutputFiles) > 0 {
			return fixupOnDemandDep(result.OutputFiles[0].Contents), nil
		}
		// Fall through to stdin approach if direct entry fails.
	}

	// Fallback: stdin. Try with both export * and export { default } first.
	// Per ES spec, export * does NOT include the default export. If the module
	// has no default, esbuild will error, and we retry without it.
	contents := fmt.Sprintf("export * from %q;\nexport { default } from %q;\n", spec, spec)
	buildOpts := api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents:   contents,
			ResolveDir: pkgDir,
			Loader:     api.LoaderJS,
		},
		Bundle:   true,
		Write:    false,
		Format:   api.FormatESModule,
		Platform: api.PlatformBrowser,
		Target:   api.ESNext,
		LogLevel: api.LogLevelSilent,
		Define:   define,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(singlePkgMap, "browser"),
			common.NodeBuiltinEmptyPlugin(moduleMap),
			common.UnknownExternalPlugin(singlePkgMap),
		},
	}
	result := api.Build(buildOpts)
	if len(result.Errors) > 0 {
		// Retry without default export — module may not have one
		buildOpts.Stdin.Contents = fmt.Sprintf("export * from %q;\n", spec)
		result = api.Build(buildOpts)
	}
	if len(result.Errors) > 0 || len(result.OutputFiles) == 0 {
		errMsg := "no output"
		if len(result.Errors) > 0 {
			errMsg = result.Errors[0].Text
		}
		return nil, fmt.Errorf("esbuild stdin failed for %s: %s", spec, errMsg)
	}
	return fixupOnDemandDep(result.OutputFiles[0].Contents), nil
}
