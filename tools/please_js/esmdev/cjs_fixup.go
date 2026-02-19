package esmdev

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

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

// fixupOnDemandDep applies CJS-to-ESM fixups to a single bundled output.
// Reuses addCJSNamedExportsToCache and fixDynamicRequires via a throwaway
// single-entry depCache — the same logic used for prebundled packages.
func fixupOnDemandDep(code []byte) []byte {
	depCache := map[string][]byte{"entry": code}
	addCJSNamedExportsToCache(depCache)
	fixDynamicRequires(depCache)
	return depCache["entry"]
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
