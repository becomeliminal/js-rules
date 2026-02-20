package esmdev

import (
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
	result := prebundlePackage("parent-pkg", parentDir, nil, outdir, nil)

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

	result := prebundlePackage("parent-pkg", parentDir, nil, outdir, nil)

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
	if !strings.Contains(text, "myNamedExport") {
		t.Errorf("expected named export 'myNamedExport' in output, got:\n%s", text)
	}

	// Should have export const { ... } destructuring (CJS fixup adds this)
	if !strings.Contains(text, "export const {") {
		t.Errorf("expected 'export const {' destructuring in output, got:\n%s", text)
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

	result := prebundlePackage("my-provider", pkgDir, nil, outdir, nil, fullModuleMap)

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
	addCJSNamedExportsToCache(depCache)

	result := string(depCache["deps/events.js"])

	// Should have export const { ... } destructuring
	if !strings.Contains(result, "export const {") {
		t.Fatalf("expected 'export const {' in output, got:\n%s", result)
	}

	// EventEmitter should be a named export
	if !strings.Contains(result, "EventEmitter") || !strings.Contains(result, "export const {") {
		t.Errorf("expected EventEmitter in export destructuring, got:\n%s", result)
	}

	// once should also be a named export
	if !strings.Contains(result, "once") {
		t.Errorf("expected 'once' in export destructuring, got:\n%s", result)
	}

	// prototype and _private should NOT be named exports
	exportLine := ""
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, "export const {") {
			exportLine = line
			break
		}
	}
	if strings.Contains(exportLine, "prototype") {
		t.Errorf("prototype should not be a named export, got:\n%s", exportLine)
	}
	if strings.Contains(exportLine, "_private") {
		t.Errorf("_private should not be a named export, got:\n%s", exportLine)
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

	result := prebundlePackage("simple-pkg", pkgDir, nil, outdir, nil)

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
