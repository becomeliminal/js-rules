package esmdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// importSpecRe matches bare import specifiers in JS/TS source code.
// Catches: import X from "pkg", import "pkg", import("pkg"), require("pkg"),
// export { X } from "pkg", export * from "pkg".
var importSpecRe = regexp.MustCompile(`(?:from\s+|import\s*\(\s*|import\s+|require\s*\(\s*)["']([^"']+)["']`)

// isLocalLibrary returns true if the moduleMap entry is a local js_library
// (no package.json) rather than an npm package.
func isLocalLibrary(pkgDir string) bool {
	absPkgDir, _ := filepath.Abs(pkgDir)
	_, err := os.Stat(filepath.Join(absPkgDir, "package.json"))
	return err != nil
}

// resolveModuleName finds the best matching module name for a bare specifier
// by longest-prefix matching against moduleMap keys. Mirrors the logic in
// ModuleResolvePlugin (common.go:231-241). Falls back to packageNameFromSpec.
func resolveModuleName(spec string, moduleMap map[string]string) string {
	best := ""
	for name := range moduleMap {
		if spec == name || strings.HasPrefix(spec, name+"/") {
			if len(name) > len(best) {
				best = name
			}
		}
	}
	if best != "" {
		return best
	}
	return packageNameFromSpec(spec)
}

// extractMissingPkgs scans JS source code for bare import specifiers and
// returns package names that exist in moduleMap but haven't been visited yet.
func extractMissingPkgs(code []byte, moduleMap map[string]string, visited map[string]bool) []string {
	var pkgs []string
	for _, m := range importSpecRe.FindAllStringSubmatch(string(code), -1) {
		spec := m[1]
		if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
			continue
		}
		pkg := resolveModuleName(spec, moduleMap)
		pkgDir, ok := moduleMap[pkg]
		if !ok || isLocalLibrary(pkgDir) || visited[pkg] {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

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
			// Only include if the module exists in moduleMap
			modName := resolveModuleName(spec, moduleMap)
			if _, ok := moduleMap[modName]; ok {
				used[spec] = true
			}
		}
		return nil
	})

	return used
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
