package esmdev

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// hasExportStatement returns true if the code contains an ESM export statement.
// Used to detect entry points that lost their exports due to esbuild's code splitting.
func hasExportStatement(code []byte) bool {
	return bytes.Contains(code, []byte("\nexport ")) || bytes.HasPrefix(code, []byte("export "))
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
	// Matches `__reExport(varName, __toESM(require_xxx()))` in stdin-bundled files.
	// esbuild produces this when `export * from "cjs-pkg"` is bundled via stdin.
	reExportRe = regexp.MustCompile(`__reExport\(\w+,\s*__toESM\((require_\w+)\(\)\)\);?`)
)

// cjsModuleInfo holds parsed info about a single __commonJS wrapper.
type cjsModuleInfo struct {
	exports     []string // direct exports (exports.foo = ...)
	delegatesTo string   // module.exports = require_xxx() delegation
}

// jsReservedWords is the set of JavaScript reserved words that cannot appear
// as bare identifiers in destructuring patterns or export declarations.
var jsReservedWords = map[string]bool{
	"default": true, "break": true, "case": true, "catch": true, "class": true,
	"const": true, "continue": true, "debugger": true, "delete": true, "do": true,
	"else": true, "enum": true, "export": true, "extends": true, "finally": true,
	"for": true, "function": true, "if": true, "import": true, "in": true,
	"instanceof": true, "let": true, "new": true, "return": true, "super": true,
	"switch": true, "this": true, "throw": true, "try": true, "typeof": true,
	"var": true, "void": true, "while": true, "with": true, "yield": true,
	"await": true, "implements": true, "interface": true, "package": true,
	"private": true, "protected": true, "public": true, "static": true,
}

// addCJSNamedExportsToCache processes all files in the dep cache together.
// It scans chunks for __commonJS wrappers, traces delegation chains
// (e.g. require_react → require_react_development), and adds named
// re-exports to entry files that only have `export default require_xxx()`.
//
// When knownExports is non-nil, it maps URL paths to export name lists
// detected by running Node.js require(). These take priority over regex
// detection; regex is used as fallback when knownExports is nil or the
// entry isn't in the map.
func addCJSNamedExportsToCache(depCache map[string][]byte, knownExports map[string][]string) {
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

			// Also detect module.exports = SomeVar; SomeVar.xxx = ...
			// This handles CJS packages like "events" where the constructor is
			// assigned to module.exports and properties are added to it directly.
			moduleExportsIdentRe := regexp.MustCompile(`module\.exports\s*=\s*(\w+)\s*;`)
			if m := moduleExportsIdentRe.FindStringSubmatch(block); m != nil {
				ident := m[1]
				identPropRe := regexp.MustCompile(regexp.QuoteMeta(ident) + `\.(\w+)\s*=`)
				for _, pm := range identPropRe.FindAllStringSubmatch(block, -1) {
					name := pm[1]
					if !seen[name] && !strings.HasPrefix(name, "_") && name != "prototype" {
						info.exports = append(info.exports, name)
						seen[name] = true
					}
				}
			}

			cjsInfo[funcName] = info
		}
	}

	// Pass 2: for each entry with `export default require_xxx()` or
	// `__reExport(varName, __toESM(require_xxx()))`, resolve the delegation
	// chain and add named re-exports.
	for urlPath, code := range depCache {
		codeStr := string(code)

		// Try pattern A: `export default require_xxx()`
		if match := defaultRequireRe.FindStringSubmatch(codeStr); match != nil {
			funcName := match[1]

			// Use Node-detected exports when available, fall back to regex
			var names []string
			if knownExports != nil {
				if exports, ok := knownExports[urlPath]; ok && len(exports) > 0 {
					names = exports
				}
			}
			if len(names) == 0 {
				names = resolveCJSExports(cjsInfo, funcName)
			}
			names = filterExportNames(names)
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
			writeNamedExports(&sb, names)
			sb.WriteString(trailing)

			depCache[urlPath] = []byte(sb.String())
			continue
		}

		// Try pattern B: `__reExport(varName, __toESM(require_xxx()))`
		// Produced by esbuild when `export * from "cjs-pkg"` is bundled via stdin.
		if match := reExportRe.FindStringSubmatch(codeStr); match != nil {
			funcName := match[1]

			// Use Node-detected exports when available, fall back to regex
			var names []string
			if knownExports != nil {
				if exports, ok := knownExports[urlPath]; ok && len(exports) > 0 {
					names = exports
				}
			}
			if len(names) == 0 {
				names = resolveCJSExports(cjsInfo, funcName)
			}
			names = filterExportNames(names)
			if len(names) == 0 {
				continue
			}
			sort.Strings(names)

			// Replace the __reExport line with proper ESM exports
			loc := reExportRe.FindStringIndex(codeStr)
			var sb strings.Builder
			sb.WriteString(codeStr[:loc[0]])
			sb.WriteString("var __cjs_exports = ")
			sb.WriteString(funcName)
			sb.WriteString("();\nexport default __cjs_exports;\n")
			writeNamedExports(&sb, names)
			sb.WriteString(codeStr[loc[1]:])

			depCache[urlPath] = []byte(sb.String())
		}
	}
}

// filterExportNames removes reserved words and __-prefixed names from export lists.
func filterExportNames(names []string) []string {
	var filtered []string
	for _, name := range names {
		if jsReservedWords[name] || strings.HasPrefix(name, "__") {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

// writeNamedExports writes individual export statements for each name.
// Uses property access (export const foo = __cjs_exports.foo) instead of
// destructuring (export const { foo } = __cjs_exports) to avoid issues
// with reserved words as property names.
func writeNamedExports(sb *strings.Builder, names []string) {
	for _, name := range names {
		fmt.Fprintf(sb, "export const %s = __cjs_exports.%s;\n", name, name)
	}
}

// esmExportBlockRe matches `export { ... };` blocks in esbuild ESM output.
var esmExportBlockRe = regexp.MustCompile(`export\s*\{([^}]+)\}\s*;`)

// addESMDefaultExport adds a synthetic default export to ESM files that only
// have named exports. This ensures CJS consumers that do `require("pkg")` —
// which fixDynamicRequires converts to `import X from "pkg"` (a default import)
// — can resolve the default binding.
//
// addCJSNamedExportsToCache handles the CJS side (adding `export default` to
// packages with __commonJS wrappers). This function is the ESM counterpart.
func addESMDefaultExport(depCache map[string][]byte) {
	for urlPath, code := range depCache {
		codeStr := string(code)

		// Skip if already has a default export
		if strings.Contains(codeStr, "export default ") ||
			strings.Contains(codeStr, " as default") {
			continue
		}

		// Find the export { ... } block
		match := esmExportBlockRe.FindStringSubmatch(codeStr)
		if match == nil {
			continue
		}

		// Parse each entry: "localName as exportedName" or "localName"
		type exportEntry struct {
			local, exported string
		}
		var entries []exportEntry
		for _, item := range strings.Split(match[1], ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			parts := strings.Fields(item)
			switch {
			case len(parts) == 3 && parts[1] == "as":
				entries = append(entries, exportEntry{local: parts[0], exported: parts[2]})
			case len(parts) == 1:
				entries = append(entries, exportEntry{local: parts[0], exported: parts[0]})
			}
		}
		if len(entries) == 0 {
			continue
		}

		// Build: var __esm_default = { exported: local, ... };
		var sb strings.Builder
		sb.WriteString("\nvar __esm_default = {")
		for i, e := range entries {
			if i > 0 {
				sb.WriteString(",")
			}
			if e.local == e.exported {
				fmt.Fprintf(&sb, " %s", e.local)
			} else {
				fmt.Fprintf(&sb, " %s: %s", e.exported, e.local)
			}
		}
		sb.WriteString(" };\nexport { __esm_default as default };\n")

		depCache[urlPath] = []byte(codeStr + sb.String())
	}
}

// fixupOnDemandDep applies CJS-to-ESM fixups to a single bundled output.
// Reuses addCJSNamedExportsToCache, fixDynamicRequires, and addESMDefaultExport
// via a throwaway single-entry depCache — the same logic used for prebundled packages.
func fixupOnDemandDep(code []byte) []byte {
	depCache := map[string][]byte{"entry": code}
	addCJSNamedExportsToCache(depCache, nil)
	fixDynamicRequires(depCache)
	addESMDefaultExport(depCache)
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
