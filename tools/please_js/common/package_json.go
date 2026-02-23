package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// exportValue represents a node in the package.json exports tree.
// Each node is either a string path (leaf), a map of condition/subpath
// keys to child nodes (branch), or an array of fallback values.
// Custom UnmarshalJSON handles the polymorphism.
type exportValue struct {
	Path  string
	Map   map[string]*exportValue
	Array []*exportValue
}

func (v *exportValue) UnmarshalJSON(data []byte) error {
	// Try string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		v.Path = s
		return nil
	}
	// Try map (condition object or subpath map)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err == nil {
		v.Map = make(map[string]*exportValue, len(m))
		for k, raw := range m {
			child := &exportValue{}
			if err := json.Unmarshal(raw, child); err != nil {
				return err
			}
			v.Map[k] = child
		}
		return nil
	}
	// Try array (Node.js "fallback" pattern: [conditionObj, fallbackString])
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	v.Array = make([]*exportValue, 0, len(arr))
	for _, raw := range arr {
		child := &exportValue{}
		if err := json.Unmarshal(raw, child); err != nil {
			return err
		}
		v.Array = append(v.Array, child)
	}
	return nil
}

// browserField represents the package.json "browser" field, which can be:
//   - A string: "browser": "lib/browser.js" (alternative entry point)
//   - An object: "browser": {"./lib/index.js": "./lib/browser.js", "fs": false}
//     (per-file replacements; false means "replace with empty module")
type browserField struct {
	Path string
	Map  map[string]string // source path → replacement path ("" if false)
}

func (b *browserField) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		b.Path = s
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	b.Map = make(map[string]string, len(m))
	for k, raw := range m {
		var val string
		if err := json.Unmarshal(raw, &val); err == nil {
			b.Map[k] = val
		} else {
			// false or other non-string → empty replacement
			b.Map[k] = ""
		}
	}
	return nil
}

// Lookup checks if a file path is remapped by this browser field's object form.
// Returns the replacement path and true, or "" and false if no mapping exists.
func (b *browserField) Lookup(filePath string) (string, bool) {
	if b.Map == nil {
		return "", false
	}
	// Normalize to "./" prefix for comparison
	normalized := "./" + strings.TrimPrefix(filePath, "./")
	for key, val := range b.Map {
		normalizedKey := "./" + strings.TrimPrefix(key, "./")
		if normalizedKey == normalized {
			return val, true
		}
	}
	return "", false
}

// packageJSON holds the fields we need for module resolution.
type packageJSON struct {
	Exports *exportValue  `json:"exports"`
	Browser *browserField `json:"browser"`
	Module  string        `json:"module"`
	Main    string        `json:"main"`
}

// ResolvePackageEntry reads a package's package.json and resolves the entry
// point for the given subpath (e.g. "." or "./react"). It tries the exports
// field first, then falls back to module/main fields for the root subpath.
func ResolvePackageEntry(pkgDir, subpath, platform string) string {
	data, err := os.ReadFile(filepath.Join(pkgDir, "package.json"))
	if err != nil {
		return ""
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	// Try exports field first
	if pkg.Exports != nil {
		if result := matchExports(pkg.Exports, subpath, platform); result != "" {
			resolved := filepath.Join(pkgDir, result)
			if _, err := os.Stat(resolved); err == nil {
				return resolved
			}
		}
	}

	// Fallback to browser/module/main for root subpath only.
	if subpath == "." {
		// Browser string form: "browser": "lib/browser.js"
		if platform == "browser" && pkg.Browser != nil && pkg.Browser.Path != "" {
			resolved := filepath.Join(pkgDir, pkg.Browser.Path)
			if _, err := os.Stat(resolved); err == nil {
				return resolved
			}
		}

		// Resolve via module → main (try adding .js extension like Node.js)
		var entryPath string
		for _, val := range []string{pkg.Module, pkg.Main} {
			if val != "" {
				resolved := filepath.Join(pkgDir, val)
				if _, err := os.Stat(resolved); err == nil {
					entryPath = val
					break
				}
				// Try with .js extension (e.g. "main": "lib/index" → "lib/index.js")
				if _, err := os.Stat(resolved + ".js"); err == nil {
					entryPath = val + ".js"
					break
				}
			}
		}

		if entryPath != "" {
			// Browser object form: "browser": {"./lib/index.js": "./lib/browser.js"}
			// If the resolved entry is mapped, use the replacement instead.
			if platform == "browser" && pkg.Browser != nil {
				if replacement, ok := pkg.Browser.Lookup(entryPath); ok && replacement != "" {
					resolved := filepath.Join(pkgDir, replacement)
					if _, err := os.Stat(resolved); err == nil {
						return resolved
					}
				}
			}
			return filepath.Join(pkgDir, entryPath)
		}
	}

	return ""
}

// matchExports resolves a subpath against a package.json exports field.
// The exports field can be:
//   - A string: "exports": "./index.js"
//   - A conditions object (no "." keys): "exports": {"import": "...", "default": "..."}
//   - A subpath map ("." keys): "exports": {".": {...}, "./react": {...}}
func matchExports(exports *exportValue, subpath, platform string) string {
	// Direct string export — only valid for root
	if exports.Path != "" {
		if subpath == "." {
			return exports.Path
		}
		return ""
	}

	if exports.Map == nil {
		return ""
	}

	// Determine if this is a subpath map (has keys starting with ".")
	// or a conditions object
	isSubpathMap := false
	for key := range exports.Map {
		if strings.HasPrefix(key, ".") {
			isSubpathMap = true
			break
		}
	}

	if isSubpathMap {
		if entry, ok := exports.Map[subpath]; ok {
			return resolveCondition(entry, platform)
		}
		// Try wildcard patterns: "./lib/*" matches "./lib/foo"
		bestPrefix := ""
		var bestEntry *exportValue
		for pattern, entry := range exports.Map {
			if !strings.Contains(pattern, "*") {
				continue
			}
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(subpath, prefix) && len(prefix) > len(bestPrefix) {
				bestPrefix = prefix
				bestEntry = entry
			}
		}
		if bestEntry != nil {
			stem := strings.TrimPrefix(subpath, bestPrefix)
			result := resolveCondition(bestEntry, platform)
			if result != "" {
				return strings.Replace(result, "*", stem, 1)
			}
		}
		return ""
	}

	// Conditions object — only valid for root
	if subpath == "." {
		return resolveCondition(exports, platform)
	}
	return ""
}

// resolveCondition recursively resolves a condition value from an exports entry.
// It handles strings (direct paths) and condition objects with platform-specific
// priority ordering.
func resolveCondition(value *exportValue, platform string) string {
	if value.Path != "" {
		return value.Path
	}
	if len(value.Array) > 0 {
		for _, elem := range value.Array {
			if result := resolveCondition(elem, platform); result != "" {
				return result
			}
		}
		return ""
	}
	if value.Map == nil {
		return ""
	}

	var keys []string
	if platform == "node" {
		keys = []string{"node", "module", "import", "require", "default"}
	} else {
		keys = []string{"browser", "module", "import", "default"}
	}

	for _, key := range keys {
		if entry, ok := value.Map[key]; ok {
			if result := resolveCondition(entry, platform); result != "" {
				return result
			}
		}
	}
	return ""
}

// ExtractPackageName extracts the npm package name from a lockfile path.
// "node_modules/react" → "react"
// "node_modules/@types/react" → "@types/react"
// "node_modules/react-dom/node_modules/scheduler" → "scheduler"
func ExtractPackageName(path string) string {
	const prefix = "node_modules/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	// Find the last occurrence of "node_modules/" to handle nested packages
	idx := strings.LastIndex(path, prefix)
	return path[idx+len(prefix):]
}

// IsNestedPackage checks if a package is inside another package's node_modules.
// "node_modules/react" → false
// "node_modules/@types/react" → false
// "node_modules/react-dom/node_modules/scheduler" → true
func IsNestedPackage(path string) bool {
	return strings.Count(path, "node_modules/") > 1
}

// ExtractParentPackagePath returns the lockfile path of the parent package
// for a nested dependency.
// "node_modules/porto/node_modules/zod" → "node_modules/porto"
// "node_modules/@metamask/utils/node_modules/semver" → "node_modules/@metamask/utils"
func ExtractParentPackagePath(path string) string {
	idx := strings.LastIndex(path, "/node_modules/")
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

// ExtractRealPackageName extracts the real npm package name from a registry URL.
// "https://registry.npmjs.org/@coinbase/wallet-sdk/-/wallet-sdk-3.9.3.tgz" → "@coinbase/wallet-sdk"
// "https://registry.npmjs.org/ms/-/ms-2.1.3.tgz" → "ms"
func ExtractRealPackageName(resolved string) string {
	const prefix = "https://registry.npmjs.org/"
	if !strings.HasPrefix(resolved, prefix) {
		return ""
	}
	rest := resolved[len(prefix):]
	sepIdx := strings.Index(rest, "/-/")
	if sepIdx < 0 {
		return ""
	}
	return rest[:sepIdx]
}
