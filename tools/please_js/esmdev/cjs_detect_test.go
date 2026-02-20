package esmdev

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestDetectCJSExports(t *testing.T) {
	// Skip if node is not available on PATH
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH, skipping detectCJSExports test")
	}

	t.Run("basic exports.xxx pattern", func(t *testing.T) {
		dir := t.TempDir()
		indexJS := filepath.Join(dir, "index.js")
		os.WriteFile(indexJS, []byte(`
			exports.foo = function() {};
			exports.bar = 42;
		`), 0644)

		result, err := detectCJSExports(nodePath, map[string]string{"test-pkg": indexJS})
		if err != nil {
			t.Fatalf("detectCJSExports failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		exports := result["test-pkg"]
		if exports == nil {
			t.Fatal("expected non-nil exports for test-pkg")
		}
		sort.Strings(exports)
		want := "bar,foo"
		got := strings.Join(exports, ",")
		if got != want {
			t.Errorf("exports = %q, want %q", got, want)
		}
	})

	t.Run("module.exports = Ident with props", func(t *testing.T) {
		dir := t.TempDir()
		indexJS := filepath.Join(dir, "index.js")
		os.WriteFile(indexJS, []byte(`
			function EventEmitter() {}
			EventEmitter.prototype.on = function() {};
			EventEmitter.EventEmitter = EventEmitter;
			EventEmitter.once = function() {};
			module.exports = EventEmitter;
		`), 0644)

		result, err := detectCJSExports(nodePath, map[string]string{"events": indexJS})
		if err != nil {
			t.Fatalf("detectCJSExports failed: %v", err)
		}

		exports := result["events"]
		if exports == nil {
			t.Fatal("expected non-nil exports for events")
		}
		sort.Strings(exports)
		// Should include EventEmitter and once, but NOT prototype (it's on the prototype chain)
		hasEventEmitter := false
		hasOnce := false
		for _, name := range exports {
			if name == "EventEmitter" {
				hasEventEmitter = true
			}
			if name == "once" {
				hasOnce = true
			}
		}
		if !hasEventEmitter {
			t.Errorf("expected EventEmitter in exports, got: %v", exports)
		}
		if !hasOnce {
			t.Errorf("expected once in exports, got: %v", exports)
		}
	})

	t.Run("filters out __esModule and default", func(t *testing.T) {
		dir := t.TempDir()
		indexJS := filepath.Join(dir, "index.js")
		os.WriteFile(indexJS, []byte(`
			Object.defineProperty(exports, "__esModule", { value: true });
			exports.default = function() {};
			exports.helper = 1;
		`), 0644)

		result, err := detectCJSExports(nodePath, map[string]string{"pkg": indexJS})
		if err != nil {
			t.Fatalf("detectCJSExports failed: %v", err)
		}

		exports := result["pkg"]
		if exports == nil {
			t.Fatal("expected non-nil exports for pkg")
		}
		for _, name := range exports {
			if name == "__esModule" {
				t.Error("should not include __esModule")
			}
			if name == "default" {
				t.Error("should not include default")
			}
		}
		found := false
		for _, name := range exports {
			if name == "helper" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected helper in exports, got: %v", exports)
		}
	})

	t.Run("ESM-only package returns null", func(t *testing.T) {
		dir := t.TempDir()
		pkgJSON := filepath.Join(dir, "package.json")
		os.WriteFile(pkgJSON, []byte(`{"name":"esm-pkg","type":"module","main":"index.js"}`), 0644)
		indexJS := filepath.Join(dir, "index.js")
		os.WriteFile(indexJS, []byte(`export const x = 1;`), 0644)

		result, err := detectCJSExports(nodePath, map[string]string{"esm-pkg": indexJS})
		if err != nil {
			t.Fatalf("detectCJSExports failed: %v", err)
		}

		// ESM require() should fail; entry should be null
		if result["esm-pkg"] != nil {
			t.Errorf("expected nil for ESM package, got: %v", result["esm-pkg"])
		}
	})

	t.Run("multiple entry points", func(t *testing.T) {
		dir := t.TempDir()
		aJS := filepath.Join(dir, "a.js")
		bJS := filepath.Join(dir, "b.js")
		os.WriteFile(aJS, []byte(`exports.x = 1;`), 0644)
		os.WriteFile(bJS, []byte(`exports.y = 2; exports.z = 3;`), 0644)

		result, err := detectCJSExports(nodePath, map[string]string{
			"pkg-a": aJS,
			"pkg-b": bJS,
		})
		if err != nil {
			t.Fatalf("detectCJSExports failed: %v", err)
		}

		if len(result["pkg-a"]) != 1 || result["pkg-a"][0] != "x" {
			t.Errorf("pkg-a exports = %v, want [x]", result["pkg-a"])
		}
		sort.Strings(result["pkg-b"])
		if len(result["pkg-b"]) != 2 || result["pkg-b"][0] != "y" || result["pkg-b"][1] != "z" {
			t.Errorf("pkg-b exports = %v, want [y, z]", result["pkg-b"])
		}
	})

	t.Run("invalid node path returns nil gracefully", func(t *testing.T) {
		result, err := detectCJSExports("/nonexistent/node", map[string]string{"pkg": "/tmp/x.js"})
		if err != nil {
			t.Fatalf("expected nil error for graceful fallback, got: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for invalid node path, got: %v", result)
		}
	})

	t.Run("empty entry points", func(t *testing.T) {
		result, err := detectCJSExports(nodePath, map[string]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for empty entries, got: %v", result)
		}
	})
}

func TestAddCJSNamedExportsToCache_WithKnownExports(t *testing.T) {
	t.Run("known exports override regex detection", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/entry.js": []byte("export default require_foo();\n"),
			"/dep/chunk.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.regexDetected = 1;\n" +
					"  }\n" +
					"});\n",
			),
		}
		knownExports := map[string][]string{
			"/dep/entry.js": {"nodeDetected", "anotherExport"},
		}
		addCJSNamedExportsToCache(depCache, knownExports)
		result := string(depCache["/dep/entry.js"])

		// Should use Node-detected exports, not regex
		if !strings.Contains(result, "export const anotherExport = __cjs_exports.anotherExport;") {
			t.Errorf("expected Node-detected export 'anotherExport', got:\n%s", result)
		}
		if !strings.Contains(result, "export const nodeDetected = __cjs_exports.nodeDetected;") {
			t.Errorf("expected Node-detected export 'nodeDetected', got:\n%s", result)
		}
		// Regex-detected export should NOT appear
		if strings.Contains(result, "regexDetected") {
			t.Errorf("should not contain regex-detected export 'regexDetected', got:\n%s", result)
		}
	})

	t.Run("falls back to regex when knownExports entry is nil", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/entry.js": []byte("export default require_foo();\n"),
			"/dep/chunk.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.fromRegex = 1;\n" +
					"  }\n" +
					"});\n",
			),
		}
		// knownExports has the key but nil value — should fall back to regex
		knownExports := map[string][]string{
			"/dep/entry.js": nil,
		}
		addCJSNamedExportsToCache(depCache, knownExports)
		result := string(depCache["/dep/entry.js"])

		if !strings.Contains(result, "export const fromRegex = __cjs_exports.fromRegex;") {
			t.Errorf("expected regex fallback export 'fromRegex', got:\n%s", result)
		}
	})

	t.Run("reserved words in known exports are filtered", func(t *testing.T) {
		depCache := map[string][]byte{
			"/dep/entry.js": []byte("export default require_foo();\n"),
			"/dep/chunk.js": []byte(
				"var require_foo = __commonJS({\n" +
					"  \"node_modules/foo/index.js\"(exports) {\n" +
					"    exports.default = 1;\n" +
					"  }\n" +
					"});\n",
			),
		}
		knownExports := map[string][]string{
			"/dep/entry.js": {"default", "class", "validName"},
		}
		addCJSNamedExportsToCache(depCache, knownExports)
		result := string(depCache["/dep/entry.js"])

		if strings.Contains(result, "export const default") {
			t.Errorf("should not export reserved word 'default', got:\n%s", result)
		}
		if strings.Contains(result, "export const class") {
			t.Errorf("should not export reserved word 'class', got:\n%s", result)
		}
		if !strings.Contains(result, "export const validName = __cjs_exports.validName;") {
			t.Errorf("expected valid export 'validName', got:\n%s", result)
		}
	})
}
