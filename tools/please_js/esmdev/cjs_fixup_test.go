package esmdev

import (
	"strings"
	"testing"
)

func TestResolveCJSExports(t *testing.T) {
	tests := []struct {
		name     string
		info     map[string]*cjsModuleInfo
		funcName string
		want     []string
	}{
		{
			name: "direct exports no delegation",
			info: map[string]*cjsModuleInfo{
				"require_foo": {exports: []string{"bar", "baz"}},
			},
			funcName: "require_foo",
			want:     []string{"bar", "baz"},
		},
		{
			name: "delegation chain",
			info: map[string]*cjsModuleInfo{
				"require_react":             {delegatesTo: "require_react_development"},
				"require_react_development": {exports: []string{"useState", "useEffect", "createElement"}},
			},
			funcName: "require_react",
			want:     []string{"useState", "useEffect", "createElement"},
		},
		{
			name: "multi-hop delegation chain",
			info: map[string]*cjsModuleInfo{
				"require_a": {delegatesTo: "require_b"},
				"require_b": {delegatesTo: "require_c"},
				"require_c": {exports: []string{"x", "y"}},
			},
			funcName: "require_a",
			want:     []string{"x", "y"},
		},
		{
			name: "cycle detection",
			info: map[string]*cjsModuleInfo{
				"require_a": {delegatesTo: "require_b"},
				"require_b": {delegatesTo: "require_a"},
			},
			funcName: "require_a",
			want:     nil,
		},
		{
			name: "missing entry",
			info: map[string]*cjsModuleInfo{
				"require_foo": {exports: []string{"bar"}},
			},
			funcName: "require_unknown",
			want:     nil,
		},
		{
			name: "delegation to missing entry",
			info: map[string]*cjsModuleInfo{
				"require_a": {delegatesTo: "require_missing"},
			},
			funcName: "require_a",
			want:     nil,
		},
		{
			name:     "empty info map",
			info:     map[string]*cjsModuleInfo{},
			funcName: "require_anything",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCJSExports(tt.info, tt.funcName)
			if tt.want == nil {
				if got != nil {
					t.Errorf("resolveCJSExports() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("resolveCJSExports() returned %d exports, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i, name := range tt.want {
				if got[i] != name {
					t.Errorf("resolveCJSExports()[%d] = %q, want %q", i, got[i], name)
				}
			}
		})
	}
}

func TestFixDynamicRequires(t *testing.T) {
	t.Run("single require", func(t *testing.T) {
		depCache := map[string][]byte{
			"/chunk-abc.js": []byte(`var x = __require("react");`),
		}
		fixDynamicRequires(depCache)
		result := string(depCache["/chunk-abc.js"])

		if !strings.Contains(result, `import __ext_0 from "react";`) {
			t.Errorf("expected import declaration for react, got:\n%s", result)
		}
		if strings.Contains(result, `__require("react")`) {
			t.Errorf("expected __require to be replaced, got:\n%s", result)
		}
		if !strings.Contains(result, "__ext_0") {
			t.Errorf("expected __ext_0 variable reference, got:\n%s", result)
		}
	})

	t.Run("multiple different requires", func(t *testing.T) {
		depCache := map[string][]byte{
			"/chunk.js": []byte(`var a = __require("react"); var b = __require("react-dom");`),
		}
		fixDynamicRequires(depCache)
		result := string(depCache["/chunk.js"])

		// Both packages should have import declarations
		if !strings.Contains(result, `from "react"`) {
			t.Errorf("expected import for react, got:\n%s", result)
		}
		if !strings.Contains(result, `from "react-dom"`) {
			t.Errorf("expected import for react-dom, got:\n%s", result)
		}
		// Should not contain any remaining __require calls
		if strings.Contains(result, "__require(") {
			t.Errorf("expected all __require calls to be replaced, got:\n%s", result)
		}
		// Should have two different variable names
		if !strings.Contains(result, "__ext_0") || !strings.Contains(result, "__ext_1") {
			t.Errorf("expected two different __ext_ variables, got:\n%s", result)
		}
	})

	t.Run("multiple same requires reuse variable", func(t *testing.T) {
		depCache := map[string][]byte{
			"/chunk.js": []byte(`var a = __require("react"); var b = __require("react");`),
		}
		fixDynamicRequires(depCache)
		result := string(depCache["/chunk.js"])

		// Should only have one import declaration for react
		count := strings.Count(result, `from "react"`)
		if count != 1 {
			t.Errorf("expected exactly 1 import for react, got %d in:\n%s", count, result)
		}
		// Both occurrences should be replaced with the same variable
		if strings.Contains(result, "__ext_1") {
			t.Errorf("expected only __ext_0 (same package reused), got:\n%s", result)
		}
		// The variable should appear twice in the body (replacing both __require calls)
		bodyStart := strings.Index(result, "\n") + 1
		body := result[bodyStart:]
		varCount := strings.Count(body, "__ext_0")
		if varCount != 2 {
			t.Errorf("expected __ext_0 to appear 2 times in body, got %d in:\n%s", varCount, body)
		}
	})

	t.Run("no requires leaves code unchanged", func(t *testing.T) {
		original := `var x = "hello"; export default x;`
		depCache := map[string][]byte{
			"/entry.js": []byte(original),
		}
		fixDynamicRequires(depCache)
		result := string(depCache["/entry.js"])

		if result != original {
			t.Errorf("expected code unchanged, got:\n%s", result)
		}
	})

	t.Run("multiple files processed independently", func(t *testing.T) {
		depCache := map[string][]byte{
			"/a.js": []byte(`var x = __require("lodash");`),
			"/b.js": []byte(`var y = __require("express");`),
		}
		fixDynamicRequires(depCache)

		a := string(depCache["/a.js"])
		b := string(depCache["/b.js"])

		if !strings.Contains(a, `from "lodash"`) {
			t.Errorf("file a: expected lodash import, got:\n%s", a)
		}
		if strings.Contains(a, `from "express"`) {
			t.Errorf("file a: should not contain express import, got:\n%s", a)
		}
		if !strings.Contains(b, `from "express"`) {
			t.Errorf("file b: expected express import, got:\n%s", b)
		}
	})
}

func TestAddCJSNamedExportsToCache(t *testing.T) {
	t.Run("entry with CJS wrapper in chunk", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/entry.js": []byte("export default require_foo();\n"),
			"/dep/chunk-abc.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.bar = function() {};\n" +
					"    exports.baz = 42;\n" +
					"  }\n" +
					"});\n",
			),
		}
		addCJSNamedExportsToCache(depCache)
		result := string(depCache["/dep/entry.js"])

		if !strings.Contains(result, "__cjs_exports") {
			t.Errorf("expected __cjs_exports variable, got:\n%s", result)
		}
		if !strings.Contains(result, "export default __cjs_exports") {
			t.Errorf("expected default export of __cjs_exports, got:\n%s", result)
		}
		// Named exports should be sorted
		if !strings.Contains(result, "export const { bar, baz } = __cjs_exports") {
			t.Errorf("expected named re-exports for bar and baz, got:\n%s", result)
		}
	})

	t.Run("delegation chain across chunks", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/react.js": []byte("export default require_react();\n"),
			"/dep/chunk.js": []byte(
				"var require_react_development = __commonJS({\n" +
					"  \"node_modules/react/cjs/react.development.js\"(exports) {\n" +
					"    exports.useState = function() {};\n" +
					"    exports.useEffect = function() {};\n" +
					"  }\n" +
					"});\n" +
					"var require_react = __commonJS({\n" +
					"  \"node_modules/react/index.js\"(exports, module) {\n" +
					"    module.exports = require_react_development();\n" +
					"  }\n" +
					"});\n",
			),
		}
		addCJSNamedExportsToCache(depCache)
		result := string(depCache["/dep/react.js"])

		if !strings.Contains(result, "export default __cjs_exports") {
			t.Errorf("expected default export, got:\n%s", result)
		}
		if !strings.Contains(result, "useEffect") || !strings.Contains(result, "useState") {
			t.Errorf("expected named exports from delegated module, got:\n%s", result)
		}
	})

	t.Run("no CJS wrappers leaves code unchanged", func(t *testing.T) {
		original := `export const foo = 42; export default foo;`
		depCache := map[string][]byte{
			"/dep/entry.js": []byte(original),
		}
		addCJSNamedExportsToCache(depCache)
		result := string(depCache["/dep/entry.js"])

		if result != original {
			t.Errorf("expected code unchanged, got:\n%s", result)
		}
	})

	t.Run("dunder-prefixed exports are skipped", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/entry.js": []byte("export default require_foo();\n"),
			"/dep/chunk.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.__esModule = true;\n" +
					"    exports.__internal = \"private\";\n" +
					"    exports.publicApi = function() {};\n" +
					"  }\n" +
					"});\n",
			),
		}
		addCJSNamedExportsToCache(depCache)
		result := string(depCache["/dep/entry.js"])

		if strings.Contains(result, "__esModule") {
			t.Errorf("expected __esModule to be skipped, got:\n%s", result)
		}
		if strings.Contains(result, "__internal") {
			t.Errorf("expected __internal to be skipped, got:\n%s", result)
		}
		if !strings.Contains(result, "publicApi") {
			t.Errorf("expected publicApi to be included, got:\n%s", result)
		}
	})

	t.Run("entry without export default require is unchanged", func(t *testing.T) {
		original := "import { foo } from './chunk.js';\nexport { foo };\n"
		depCache := map[string][]byte{
			"/dep/entry.js": []byte(original),
			"/dep/chunk.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.bar = 1;\n" +
					"  }\n" +
					"});\n",
			),
		}
		addCJSNamedExportsToCache(depCache)
		result := string(depCache["/dep/entry.js"])

		if result != original {
			t.Errorf("expected entry unchanged (no export default require_), got:\n%s", result)
		}
	})

	t.Run("duplicate exports are deduplicated", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/entry.js": []byte("export default require_foo();\n"),
			"/dep/chunk.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.bar = 1;\n" +
					"    exports.bar = 2;\n" +
					"    exports.baz = 3;\n" +
					"  }\n" +
					"});\n",
			),
		}
		addCJSNamedExportsToCache(depCache)
		result := string(depCache["/dep/entry.js"])

		// "bar" should only appear once in the destructuring
		exportLine := ""
		for _, line := range strings.Split(result, "\n") {
			if strings.Contains(line, "export const {") {
				exportLine = line
				break
			}
		}
		if strings.Count(exportLine, "bar") != 1 {
			t.Errorf("expected bar to appear exactly once in export destructuring, got:\n%s", exportLine)
		}
	})
}
