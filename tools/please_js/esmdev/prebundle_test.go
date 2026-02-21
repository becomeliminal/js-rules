package esmdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrebundlePackage_NestedNodeModules verifies that version-conflicted
// packages nested in node_modules/ get bundled into the parent, not
// externalized. npm puts packages in <parent>/node_modules/<pkg> only when
// the version conflicts with the hoisted copy. If we externalize them, the
// import map points to the hoisted (wrong) version.
//
// Scenario modelled after the real bug: porto needs zod@4 (has ./mini) but
// the hoisted zod@3 (lacks ./mini) is what the import map resolves to.
func TestPrebundlePackage_NestedNodeModules(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	// --- parent-pkg: imports "nested-dep/sub" ---
	parentDir := filepath.Join(dir, "parent-pkg")
	os.MkdirAll(parentDir, 0755)
	os.WriteFile(filepath.Join(parentDir, "package.json"), []byte(`{
  "name": "parent-pkg",
  "version": "1.0.0",
  "exports": {
    ".": "./index.js"
  }
}`), 0644)
	os.WriteFile(filepath.Join(parentDir, "index.js"), []byte(
		"export { value } from \"nested-dep/sub\";\n",
	), 0644)

	// --- nested-dep@2.0.0 (version-conflicted, in parent's node_modules) ---
	// This version HAS the ./sub export — the one parent-pkg actually needs.
	nestedDir := filepath.Join(parentDir, "node_modules", "nested-dep")
	os.MkdirAll(nestedDir, 0755)
	os.WriteFile(filepath.Join(nestedDir, "package.json"), []byte(`{
  "name": "nested-dep",
  "version": "2.0.0",
  "exports": {
    ".": "./index.js",
    "./sub": "./sub.js"
  }
}`), 0644)
	os.WriteFile(filepath.Join(nestedDir, "index.js"), []byte(
		"export const name = \"nested-dep-v2\";\n",
	), 0644)
	os.WriteFile(filepath.Join(nestedDir, "sub.js"), []byte(
		"export const value = \"from-nested-v2\";\n",
	), 0644)

	// --- Run prebundlePackage ---
	result := prebundlePackage("parent-pkg", parentDir, nil, outdir, nil, "")

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}

	if len(result.depCache) == 0 {
		t.Fatal("expected non-empty depCache")
	}

	// The bundled output should NOT contain an externalized import of
	// "nested-dep/sub" — the nested dep's code must be inlined.
	for path, content := range result.depCache {
		text := string(content)
		if strings.Contains(text, `from "nested-dep/sub"`) || strings.Contains(text, `from "nested-dep"`) {
			t.Errorf("file %s still has externalized nested-dep import; nested dep was not bundled in:\n%s", path, text)
		}
	}

	// The inlined value from the nested dep should appear in the output.
	found := false
	for _, content := range result.depCache {
		if strings.Contains(string(content), "from-nested-v2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'from-nested-v2' from nested dep to be inlined in bundle output")
		for path, content := range result.depCache {
			t.Logf("  %s: %s", path, string(content))
		}
	}
}

// TestPrebundlePackage_NestedScopedPackage verifies that scoped packages
// (e.g., @scope/pkg) nested in node_modules are also bundled in.
func TestPrebundlePackage_NestedScopedPackage(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	parentDir := filepath.Join(dir, "parent-pkg")
	os.MkdirAll(parentDir, 0755)
	os.WriteFile(filepath.Join(parentDir, "package.json"), []byte(`{
  "name": "parent-pkg",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(parentDir, "index.js"), []byte(
		"export { helper } from \"@scope/util\";\n",
	), 0644)

	// Scoped package nested in parent's node_modules
	scopedDir := filepath.Join(parentDir, "node_modules", "@scope", "util")
	os.MkdirAll(scopedDir, 0755)
	os.WriteFile(filepath.Join(scopedDir, "package.json"), []byte(`{
  "name": "@scope/util",
  "version": "2.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(scopedDir, "index.js"), []byte(
		"export const helper = \"scoped-v2\";\n",
	), 0644)

	result := prebundlePackage("parent-pkg", parentDir, nil, outdir, nil, "")

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}

	// Should NOT have externalized @scope/util
	for path, content := range result.depCache {
		text := string(content)
		if strings.Contains(text, `from "@scope/util"`) {
			t.Errorf("file %s still has externalized @scope/util import:\n%s", path, text)
		}
	}

	// The inlined value should be present
	found := false
	for _, content := range result.depCache {
		if strings.Contains(string(content), "scoped-v2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'scoped-v2' from nested scoped dep to be inlined")
	}
}

// TestBundleSubpathViaStdin_CJSFixup verifies that bundleSubpathViaStdin
// applies CJS-to-ESM fixups (named exports, dynamic require replacement)
// and resolves process.env.NODE_ENV via Define. Without these fixups,
// CJS packages like use-sync-external-store produce only a default export,
// causing "does not provide an export named X" errors in the browser.
func TestBundleSubpathViaStdin_CJSFixup(t *testing.T) {
	dir := t.TempDir()

	// Create a CJS package with a conditional require (like use-sync-external-store).
	// The index.js delegates to dev/prod via process.env.NODE_ENV, and the
	// development module exports a named function.
	pkgDir := filepath.Join(dir, "cjs-pkg")
	os.MkdirAll(filepath.Join(pkgDir, "cjs"), 0755)
	os.MkdirAll(filepath.Join(pkgDir, "shim"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "cjs-pkg",
  "version": "1.0.0",
  "exports": {
    ".": "./index.js",
    "./shim": "./shim/index.js"
  }
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(`'use strict';
if (process.env.NODE_ENV === 'production') {
  module.exports = require('./cjs/prod.js');
} else {
  module.exports = require('./cjs/dev.js');
}
`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "cjs", "dev.js"), []byte(`'use strict';
exports.myNamedExport = function() { return "dev"; };
`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "cjs", "prod.js"), []byte(`'use strict';
exports.myNamedExport = function() { return "prod"; };
`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "shim", "index.js"), []byte(`'use strict';
if (process.env.NODE_ENV === 'production') {
  module.exports = require('../cjs/prod.js');
} else {
  module.exports = require('../cjs/dev.js');
}
`), 0644)

	moduleMap := map[string]string{"cjs-pkg": pkgDir}
	define := map[string]string{"process.env.NODE_ENV": `"development"`}

	code, err := bundleSubpathViaStdin("cjs-pkg/shim", "cjs-pkg", pkgDir, moduleMap, define)
	if err != nil {
		t.Fatalf("bundleSubpathViaStdin failed: %v", err)
	}

	text := string(code)

	// Named export should be present (CJS fixup worked)
	if !strings.Contains(text, "export const myNamedExport = __cjs_exports.myNamedExport;") {
		t.Errorf("expected named export 'myNamedExport' in output, got:\n%s", text)
	}

	// Should NOT have __require("specifier") calls (fixDynamicRequires should replace them).
	// Note: __require() in the __commonJS helper definition is fine — we only care
	// about __require("some-specifier") calls that represent external dependencies.
	if dynamicRequireRe.MatchString(text) {
		t.Errorf("expected no __require(\"...\") calls in output, got:\n%s", text)
	}

	// Should reference "dev" not "prod" (Define resolved process.env.NODE_ENV)
	if strings.Contains(text, `"prod"`) {
		t.Errorf("expected development path (not production), got:\n%s", text)
	}
}

// TestPrebundlePackage_NodeBuiltinPolyfill verifies that when a package
// imports a Node.js builtin name (like "events") that also exists as an npm
// polyfill in the full moduleMap, the prebundle externalizes it instead of
// stubbing it to an empty module. Without this, packages like WalletConnect's
// EthereumProvider get `EventEmitter is not a constructor` at runtime.
func TestPrebundlePackage_NodeBuiltinPolyfill(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	// Create a package that imports EventEmitter from "events"
	pkgDir := filepath.Join(dir, "my-provider")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "my-provider",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(
		"import { EventEmitter } from \"events\";\nexport class Provider extends EventEmitter {}\n",
	), 0644)

	// Full moduleMap includes "events" as an available npm polyfill.
	// This tells NodeBuiltinEmptyPlugin to skip "events" and let
	// UnknownExternalPlugin externalize it instead.
	fullModuleMap := map[string]string{
		"my-provider": pkgDir,
		"events":      filepath.Join(dir, "fake-events"), // path doesn't matter, only key existence
	}

	result := prebundlePackage("my-provider", pkgDir, nil, outdir, nil, "", fullModuleMap)

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}

	// The output should have "events" as an external import (from "events"),
	// NOT an inlined empty stub. When events is stubbed, the output will
	// contain the __commonJS helper with `module.exports = {}` and no
	// external import of "events".
	hasExternalEventsImport := false
	for _, content := range result.depCache {
		text := string(content)
		if strings.Contains(text, `from "events"`) {
			hasExternalEventsImport = true
		}
	}
	if !hasExternalEventsImport {
		t.Error("expected 'events' to be externalized (from \"events\"), not stubbed empty")
		for path, content := range result.depCache {
			t.Logf("  %s:\n%s", path, string(content))
		}
	}
}

// TestCJSFixup_ModuleExportsIdentifier verifies that addCJSNamedExportsToCache
// detects the pattern where module.exports is assigned to a named identifier
// (e.g., `module.exports = EventEmitter`) and that identifier has properties
// assigned to it (e.g., `EventEmitter.once = once; EventEmitter.EventEmitter = EventEmitter`).
// This is common in packages like "events" where the constructor IS module.exports.
func TestCJSFixup_ModuleExportsIdentifier(t *testing.T) {
	// Simulate esbuild output for a CJS package with the self-referencing pattern:
	//   module.exports = EventEmitter;
	//   EventEmitter.EventEmitter = EventEmitter;
	//   EventEmitter.once = once;
	//   EventEmitter.prototype.on = function() {};
	//   EventEmitter._private = something;
	bundled := `// chunk
var require_events = __commonJS({
  "index.js"(exports, module) {
    function EventEmitter() {}
    EventEmitter.prototype.on = function() {};
    EventEmitter.prototype.emit = function() {};
    function once(emitter, name) { return []; }
    EventEmitter.EventEmitter = EventEmitter;
    EventEmitter.once = once;
    EventEmitter._private = true;
    module.exports = EventEmitter;
  }
});
export default require_events();
`

	depCache := map[string][]byte{
		"deps/events.js": []byte(bundled),
	}
	addCJSNamedExportsToCache(depCache, nil)

	result := string(depCache["deps/events.js"])

	// Should have individual export statements
	if !strings.Contains(result, "export const EventEmitter = __cjs_exports.EventEmitter;") {
		t.Errorf("expected named export for EventEmitter, got:\n%s", result)
	}
	if !strings.Contains(result, "export const once = __cjs_exports.once;") {
		t.Errorf("expected named export for once, got:\n%s", result)
	}

	// prototype and _private should NOT be named exports
	if strings.Contains(result, "export const prototype") {
		t.Errorf("prototype should not be a named export, got:\n%s", result)
	}
	if strings.Contains(result, "export const _private") {
		t.Errorf("_private should not be a named export, got:\n%s", result)
	}
}

// TestPrebundlePackage_NoNestedNodeModules verifies that packages without
// nested node_modules still work correctly (no regression).
func TestPrebundlePackage_NoNestedNodeModules(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	pkgDir := filepath.Join(dir, "simple-pkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "simple-pkg",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(
		"export const x = 42;\n",
	), 0644)

	result := prebundlePackage("simple-pkg", pkgDir, nil, outdir, nil, "")

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}
	if len(result.depCache) == 0 {
		t.Fatal("expected non-empty depCache")
	}

	found := false
	for _, content := range result.depCache {
		if strings.Contains(string(content), "42") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected '42' in bundle output")
	}
}

// TestFillMissingDeps_ESMDefaultExport is an end-to-end test for the real
// lodash-es bug. It simulates the exact production scenario:
//
//  1. Package "esm-lib" (like lodash-es) — ESM, no exports field, files with default exports.
//  2. Package "consumer" (like mermaid) — imports "esm-lib/memoize.js" which gets externalized.
//  3. Per-package prebundle: consumer is bundled, esm-lib is bundled (main entry only).
//  4. MergeImportmaps runs fillMissingDeps, which finds "esm-lib/memoize.js" in consumer's
//     bundled output and calls bundleSubpathViaStdin to create the missing subpath file.
//
// The bug: bundleSubpathViaStdin used `export * from "esm-lib/memoize.js"` which per the ES
// spec does NOT re-export default exports, producing an empty file.
//
// The fix: resolve to the actual file and use it as a direct entry point.
func TestFillMissingDeps_ESMDefaultExport(t *testing.T) {
	dir := t.TempDir()

	// esm-lib: like lodash-es — ESM, no "exports" field, individual files with default exports
	esmLibDir := filepath.Join(dir, "esm-lib")
	os.MkdirAll(esmLibDir, 0755)
	os.WriteFile(filepath.Join(esmLibDir, "package.json"), []byte(`{
  "name": "esm-lib",
  "version": "1.0.0",
  "type": "module",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(esmLibDir, "index.js"), []byte(
		"export { default as memoize } from './memoize.js';\n",
	), 0644)
	os.WriteFile(filepath.Join(esmLibDir, "memoize.js"), []byte(
		"function memoize(fn) { return fn; }\nexport default memoize;\n",
	), 0644)

	// consumer: like mermaid — imports esm-lib/memoize.js (externalized in its bundle)
	consumerDir := filepath.Join(dir, "consumer")
	os.MkdirAll(consumerDir, 0755)
	os.WriteFile(filepath.Join(consumerDir, "package.json"), []byte(`{
  "name": "consumer",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(consumerDir, "index.js"), []byte(
		"import memoize from 'esm-lib/memoize.js';\nexport default memoize;\n",
	), 0644)

	// Write moduleconfig
	moduleConfigPath := filepath.Join(dir, "moduleconfig")
	os.WriteFile(moduleConfigPath, []byte(
		"esm-lib="+esmLibDir+"\n"+
			"consumer="+consumerDir+"\n",
	), 0644)

	// Simulate prebundle output: esm-lib main entry exists, consumer has
	// an externalized import of "esm-lib/memoize.js".
	depsDir := filepath.Join(dir, "deps")
	os.MkdirAll(filepath.Join(depsDir, "consumer"), 0755)
	os.WriteFile(filepath.Join(depsDir, "consumer.js"), []byte(
		"import memoize from \"esm-lib/memoize.js\";\nvar consumer_default = memoize;\nexport {\n  consumer_default as default\n};\n",
	), 0644)
	os.WriteFile(filepath.Join(depsDir, "esm-lib.js"), []byte(
		"function memoize(fn) { return fn; }\nexport { memoize };\n",
	), 0644)

	// Seed the import map with what per-package prebundling would produce
	importMap := map[string]string{
		"consumer":  "/@deps/consumer.js",
		"consumer/": "/@deps/consumer/",
		"esm-lib":   "/@deps/esm-lib.js",
		"esm-lib/":  "/@deps/esm-lib/",
	}

	err := fillMissingDeps(importMap, moduleConfigPath, depsDir)
	if err != nil {
		t.Fatalf("fillMissingDeps failed: %v", err)
	}

	// The import map should now have esm-lib/memoize.js
	urlPath, ok := importMap["esm-lib/memoize.js"]
	if !ok {
		t.Fatalf("import map missing esm-lib/memoize.js; map: %v", importMap)
	}
	t.Logf("esm-lib/memoize.js → %s", urlPath)

	// Read the generated file from depsDir
	rel := strings.TrimPrefix(urlPath, "/@deps/")
	filePath := filepath.Join(depsDir, rel)
	code, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read generated file %s: %v", filePath, err)
	}

	text := string(code)
	t.Logf("generated esm-lib/memoize.js.js:\n%s", text)

	// MUST have a default export — this is the actual browser error
	if !strings.Contains(text, "default") {
		t.Errorf("esm-lib/memoize.js has NO default export — would cause:\n"+
			"  'The requested module does not provide an export named default'\n"+
			"Content:\n%s", text)
	}
	if !hasExportStatement(code) {
		t.Errorf("esm-lib/memoize.js has no export statements.\nContent:\n%s", text)
	}
}

// TestBundleSubpathViaStdin_DefaultExport verifies that bundleSubpathViaStdin
// preserves default exports from ESM subpath files.
func TestBundleSubpathViaStdin_DefaultExport(t *testing.T) {
	dir := t.TempDir()

	// Create an ESM package with no "exports" field and individual files
	// that use default exports — exactly like lodash-es.
	pkgDir := filepath.Join(dir, "esm-pkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "esm-pkg",
  "version": "1.0.0",
  "type": "module",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(
		"export { default as memoize } from './memoize.js';\n",
	), 0644)
	os.WriteFile(filepath.Join(pkgDir, "memoize.js"), []byte(
		"function memoize(fn) { return fn; }\nexport default memoize;\n",
	), 0644)

	moduleMap := map[string]string{"esm-pkg": pkgDir}
	define := map[string]string{"process.env.NODE_ENV": `"development"`}

	code, err := bundleSubpathViaStdin("esm-pkg/memoize.js", "esm-pkg", pkgDir, moduleMap, define)
	if err != nil {
		t.Fatalf("bundleSubpathViaStdin failed: %v", err)
	}

	text := string(code)
	t.Logf("output:\n%s", text)

	// MUST have a default export — `import memoize from "esm-pkg/memoize.js"` needs it.
	if !strings.Contains(text, "default") {
		t.Errorf("expected default export in output, got:\n%s", text)
	}
	if !hasExportStatement([]byte(text)) {
		t.Errorf("expected export statement in output, got:\n%s", text)
	}
}

// TestBundleSubpathViaStdin_NamedExportsOnly verifies that bundleSubpathViaStdin
// also works for ESM subpath files that only have named exports (no default).
func TestBundleSubpathViaStdin_NamedExportsOnly(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, "esm-pkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "esm-pkg",
  "version": "1.0.0",
  "type": "module",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(
		"export { foo, bar } from './utils.js';\n",
	), 0644)
	os.WriteFile(filepath.Join(pkgDir, "utils.js"), []byte(
		"export function foo() { return 1; }\nexport function bar() { return 2; }\n",
	), 0644)

	moduleMap := map[string]string{"esm-pkg": pkgDir}
	define := map[string]string{"process.env.NODE_ENV": `"development"`}

	code, err := bundleSubpathViaStdin("esm-pkg/utils.js", "esm-pkg", pkgDir, moduleMap, define)
	if err != nil {
		t.Fatalf("bundleSubpathViaStdin failed: %v", err)
	}

	text := string(code)
	t.Logf("output:\n%s", text)

	if !strings.Contains(text, "foo") {
		t.Errorf("expected 'foo' in output, got:\n%s", text)
	}
	if !strings.Contains(text, "bar") {
		t.Errorf("expected 'bar' in output, got:\n%s", text)
	}
	if !hasExportStatement([]byte(text)) {
		t.Errorf("expected export statement in output, got:\n%s", text)
	}
}

// TestPrebundleDeps_TransitiveDeps verifies that transitive dependencies
// (packages imported by prebundled deps but not by user source code) get
// included in the import map in filtered mode.
//
// Scenario modelled after Tiptap: @tiptap/react → @tiptap/core → prosemirror-state.
// User code imports @tiptap/react. In filtered mode, only @tiptap/react is in
// usedImports. The prebundled @tiptap/react externalizes @tiptap/core, and
// @tiptap/core externalizes prosemirror-state. Without the fix, neither
// @tiptap/core nor prosemirror-state would be in the import map, causing the
// browser to fail with "Failed to resolve module specifier".
func TestPrebundleDeps_TransitiveDeps(t *testing.T) {
	dir := t.TempDir()

	// "wrapper" imports "transitive-dep" (like @tiptap/react → @tiptap/core)
	wrapperDir := filepath.Join(dir, "wrapper")
	os.MkdirAll(wrapperDir, 0755)
	os.WriteFile(filepath.Join(wrapperDir, "package.json"), []byte(`{
  "name": "wrapper",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(wrapperDir, "index.js"), []byte(
		"import { x } from \"transitive-dep\";\nexport const wrapped = x + 1;\n",
	), 0644)

	// "transitive-dep" is a standalone package (like @tiptap/core or prosemirror-state)
	transDir := filepath.Join(dir, "transitive-dep")
	os.MkdirAll(transDir, 0755)
	os.WriteFile(filepath.Join(transDir, "package.json"), []byte(`{
  "name": "transitive-dep",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(transDir, "index.js"), []byte(
		"export const x = 42;\n",
	), 0644)

	moduleMap := map[string]string{
		"wrapper":        wrapperDir,
		"transitive-dep": transDir,
	}
	// Filtered mode: only "wrapper" in usedImports (user code doesn't import transitive-dep)
	usedImports := map[string]bool{"wrapper": true}
	define := map[string]string{"process.env.NODE_ENV": `"development"`}

	_, importMapJSON, err := prebundleDeps(moduleMap, usedImports, define)
	if err != nil {
		t.Fatalf("prebundleDeps failed: %v", err)
	}

	var imData struct {
		Imports map[string]string `json:"imports"`
	}
	if err := json.Unmarshal(importMapJSON, &imData); err != nil {
		t.Fatalf("failed to parse import map: %v", err)
	}

	// wrapper should be in import map
	if _, ok := imData.Imports["wrapper"]; !ok {
		t.Fatalf("wrapper not in import map: %v", imData.Imports)
	}

	// transitive-dep MUST be in import map so the browser can resolve the
	// externalized `from "transitive-dep"` in wrapper.js.
	// Without it, the browser throws: "Failed to resolve module specifier 'transitive-dep'"
	if _, ok := imData.Imports["transitive-dep"]; !ok {
		t.Errorf("transitive-dep missing from import map; browser can't resolve bare specifiers.\nimportMap: %v", imData.Imports)
	}
}

// TestPrebundleDeps_TransitiveDepsChain verifies that chains of transitive
// dependencies (A → B → C) are all filled in, not just one level deep.
// Models the Tiptap chain: @tiptap/react → @tiptap/core → prosemirror-state.
func TestPrebundleDeps_TransitiveDepsChain(t *testing.T) {
	dir := t.TempDir()

	// "app-lib" imports "middle" (like @tiptap/react → @tiptap/core)
	appDir := filepath.Join(dir, "app-lib")
	os.MkdirAll(appDir, 0755)
	os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{
  "name": "app-lib",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(appDir, "index.js"), []byte(
		"import { core } from \"middle\";\nexport const app = core;\n",
	), 0644)

	// "middle" imports "deep-dep" (like @tiptap/core → prosemirror-state)
	middleDir := filepath.Join(dir, "middle")
	os.MkdirAll(middleDir, 0755)
	os.WriteFile(filepath.Join(middleDir, "package.json"), []byte(`{
  "name": "middle",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(middleDir, "index.js"), []byte(
		"import { val } from \"deep-dep\";\nexport const core = val;\n",
	), 0644)

	// "deep-dep" — leaf dependency (like prosemirror-state)
	deepDir := filepath.Join(dir, "deep-dep")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(filepath.Join(deepDir, "package.json"), []byte(`{
  "name": "deep-dep",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(deepDir, "index.js"), []byte(
		"export const val = 99;\n",
	), 0644)

	moduleMap := map[string]string{
		"app-lib":  appDir,
		"middle":   middleDir,
		"deep-dep": deepDir,
	}
	usedImports := map[string]bool{"app-lib": true}
	define := map[string]string{"process.env.NODE_ENV": `"development"`}

	_, importMapJSON, err := prebundleDeps(moduleMap, usedImports, define)
	if err != nil {
		t.Fatalf("prebundleDeps failed: %v", err)
	}

	var imData struct {
		Imports map[string]string `json:"imports"`
	}
	json.Unmarshal(importMapJSON, &imData)

	for _, pkg := range []string{"app-lib", "middle", "deep-dep"} {
		if _, ok := imData.Imports[pkg]; !ok {
			t.Errorf("%s missing from import map; full chain must be resolved.\nimportMap: %v", pkg, imData.Imports)
		}
	}
}
