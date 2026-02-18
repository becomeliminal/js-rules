package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// exportValue represents a node in the package.json exports tree.
// Each node is either a string path (leaf) or a map of condition/subpath
// keys to child nodes (branch). Custom UnmarshalJSON handles the polymorphism.
type exportValue struct {
	Path string
	Map  map[string]*exportValue
}

func (v *exportValue) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		v.Path = s
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
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

// packageJSON holds the fields we need for module resolution.
type packageJSON struct {
	Exports *exportValue `json:"exports"`
	Module  string       `json:"module"`
	Main    string       `json:"main"`
}

// resolvePackageEntry reads a package's package.json and resolves the entry
// point for the given subpath (e.g. "." or "./react"). It tries the exports
// field first, then falls back to module/main fields for the root subpath.
func resolvePackageEntry(pkgDir, subpath, platform string) string {
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

	// Fallback to module/main for root subpath only
	if subpath == "." {
		for _, val := range []string{pkg.Module, pkg.Main} {
			if val != "" {
				resolved := filepath.Join(pkgDir, val)
				if _, err := os.Stat(resolved); err == nil {
					return resolved
				}
			}
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
