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
