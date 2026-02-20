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
	depCache, importMap, failedPkgs := prebundleAllPackages(context.Background(), moduleMap, nil, define)

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
	define := make(map[string]string)
	common.MergeEnvDefines(define, "development")

	for pkgName, pkgDir := range moduleMap {
		if isLocalLibrary(pkgDir) {
			continue // local js_library targets are not pre-bundled
		}
		result := prebundlePackage(pkgName, pkgDir, nil, outdir, define, moduleMap)
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
// finds those missing from the import map, and bundles them.
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

	// Scan all .js files in depsDir for bare import specifiers
	bareSpecs := make(map[string]bool)
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
			if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
				continue
			}
			bareSpecs[spec] = true
		}
		return nil
	})

	// Find missing: specifiers where neither exact match nor base package exists
	var missingPkgs []string   // whole packages not in import map
	var missingSubpaths []string // subpath specifiers where base pkg exists but subpath doesn't
	seen := make(map[string]bool)

	for spec := range bareSpecs {
		pkgName := resolveModuleName(spec, moduleMap)
		// Check if this package is available in moduleMap
		pkgDir, ok := moduleMap[pkgName]
		if !ok {
			continue // not a known package, skip
		}
		// Skip local libraries — they're served on-demand, not pre-bundled
		if isLocalLibrary(pkgDir) {
			continue
		}

		// Check if exact specifier is already in import map
		if _, ok := importMap[spec]; ok {
			continue
		}

		if spec == pkgName {
			// Whole package missing
			if !seen[pkgName] {
				seen[pkgName] = true
				missingPkgs = append(missingPkgs, pkgName)
			}
		} else {
			// Check if base package has ANY entry
			hasBase := false
			if _, ok := importMap[pkgName]; ok {
				hasBase = true
			}
			if !hasBase {
				// Whole package missing
				if !seen[pkgName] {
					seen[pkgName] = true
					missingPkgs = append(missingPkgs, pkgName)
				}
			} else {
				// Base package exists but this subpath doesn't
				missingSubpaths = append(missingSubpaths, spec)
			}
		}
	}

	sort.Strings(missingPkgs)
	sort.Strings(missingSubpaths)

	// Bundle missing whole packages
	if len(missingPkgs) > 0 {
		outdir, _ := filepath.Abs(".esm-prebundle-tmp")
		for _, pkgName := range missingPkgs {
			pkgDir := moduleMap[pkgName]
			result := prebundlePackage(pkgName, pkgDir, nil, outdir, define, moduleMap)
			if result.err != nil {
				fmt.Fprintf(os.Stderr, "  warning: skipping missing dep %s: %v\n", pkgName, result.err)
				continue
			}
			for k, v := range result.importMap {
				importMap[k] = v
			}
			// Write bundled files to depsDir
			for urlPath, data := range result.depCache {
				rel := strings.TrimPrefix(urlPath, "/@deps/")
				filePath := filepath.Join(depsDir, rel)
				os.MkdirAll(filepath.Dir(filePath), 0755)
				os.WriteFile(filePath, data, 0644)
			}
		}
		addPrefixImportMapEntries(importMap)
	}

	// Bundle missing subpaths via esbuild stdin
	if len(missingSubpaths) > 0 {
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
			// Write to depsDir
			filePath := filepath.Join(depsDir, spec+".js")
			os.MkdirAll(filepath.Dir(filePath), 0755)
			os.WriteFile(filePath, code, 0644)
		}
	}

	return nil
}

// bundleSubpathViaStdin bundles a bare specifier using esbuild's stdin API.
// Uses the full moduleMap for cross-package resolution within the bundle.
func bundleSubpathViaStdin(spec, pkgName, pkgDir string, moduleMap map[string]string, define map[string]string) ([]byte, error) {
	contents := fmt.Sprintf("export * from %q;\n", spec)
	// Use single-package map for externalization: only the current package
	// is resolved, all others are externalized.
	singlePkgMap := map[string]string{pkgName: pkgDir}
	result := api.Build(api.BuildOptions{
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
	})
	if len(result.Errors) > 0 || len(result.OutputFiles) == 0 {
		errMsg := "no output"
		if len(result.Errors) > 0 {
			errMsg = result.Errors[0].Text
		}
		return nil, fmt.Errorf("esbuild stdin failed for %s: %s", spec, errMsg)
	}
	return fixupOnDemandDep(result.OutputFiles[0].Contents), nil
}
