package esmdev

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBarrelReExportResolution(t *testing.T) {
	dir := t.TempDir()

	// Create extensions/index.ts — barrel that re-exports from ./Foo
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "index.ts"),
		[]byte(`export { Foo } from "./Foo";`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "Foo.ts"),
		[]byte(`export const Foo = 1;`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	srv := &esmServer{
		sourceRoot:  dir,
		packageRoot: dir,
	}

	// Step 1: Request /extensions — should redirect to /extensions/index.ts
	req := httptest.NewRequest("GET", "/extensions", nil)
	rec := httptest.NewRecorder()
	srv.handleSource(rec, req, "/extensions", time.Now())

	// Follow redirect if present
	effectiveURL := "/extensions"
	if rec.Code >= 300 && rec.Code < 400 {
		loc := rec.Header().Get("Location")
		if loc == "" {
			t.Fatal("redirect with no Location header")
		}
		effectiveURL = loc

		// Follow the redirect
		req2 := httptest.NewRequest("GET", effectiveURL, nil)
		rec = httptest.NewRecorder()
		srv.handleSource(rec, req2, effectiveURL, time.Now())
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Step 2: Find the "./Foo" import specifier in the response
	specifier := ""
	for _, q := range []string{`"./Foo"`, `"./Foo.ts"`} {
		if strings.Contains(body, q) {
			// Strip quotes
			specifier = q[1 : len(q)-1]
			break
		}
	}
	if specifier == "" {
		t.Fatalf("barrel re-export should contain ./Foo import, got:\n%s", body)
	}

	// Step 3: Resolve the specifier relative to the effective URL (as a browser would)
	base, err := url.Parse(effectiveURL)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := url.Parse(specifier)
	if err != nil {
		t.Fatal(err)
	}
	resolvedURL := base.ResolveReference(ref).Path

	// Step 4: Request the resolved URL — should be 200, not 404
	req3 := httptest.NewRequest("GET", resolvedURL, nil)
	rec3 := httptest.NewRecorder()
	srv.handleSource(rec3, req3, resolvedURL, time.Now())

	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 for resolved import %s (from effective URL %s + specifier %s), got %d",
			resolvedURL, effectiveURL, specifier, rec3.Code)
	}
}

func TestHandleTextModule(t *testing.T) {
	dir := t.TempDir()
	mdContent := "# Hello World\n\nThis is a test.\n"
	if err := os.WriteFile(filepath.Join(dir, "post.md"), []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	srv := &esmServer{
		sourceRoot:  dir,
		packageRoot: dir,
	}

	req := httptest.NewRequest("GET", "/post.md", nil)
	rec := httptest.NewRecorder()
	srv.handleTextModule(rec, req, "/post.md", time.Now())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("expected Content-Type application/javascript, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.HasPrefix(body, "export default ") {
		t.Error("expected body to start with 'export default '")
	}
	if !strings.Contains(body, "# Hello World") {
		t.Error("expected body to contain the markdown content")
	}
	if !strings.Contains(body, "This is a test.") {
		t.Error("expected body to contain the markdown content")
	}
}

func TestHandleTextModule_FallbackToSourceRoot(t *testing.T) {
	sourceRoot := t.TempDir()
	packageRoot := t.TempDir()
	mdContent := "source root content"
	if err := os.WriteFile(filepath.Join(sourceRoot, "readme.md"), []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	srv := &esmServer{
		sourceRoot:  sourceRoot,
		packageRoot: packageRoot,
	}

	req := httptest.NewRequest("GET", "/readme.md", nil)
	rec := httptest.NewRecorder()
	srv.handleTextModule(rec, req, "/readme.md", time.Now())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "source root content") {
		t.Error("expected body to contain content from sourceRoot fallback")
	}
}

func TestHandleTextModule_NotFound(t *testing.T) {
	dir := t.TempDir()
	srv := &esmServer{
		sourceRoot:  dir,
		packageRoot: dir,
	}

	req := httptest.NewRequest("GET", "/missing.md", nil)
	rec := httptest.NewRecorder()
	srv.handleTextModule(rec, req, "/missing.md", time.Now())

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestHandleDepOnDemand_CJSFunctionSubpath tests the runtime on-demand handler
// for a CJS package subpath with `module.exports = function(){}`.
func TestHandleDepOnDemand_CJSFunctionSubpath(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, "cjs-pkg")
	os.MkdirAll(filepath.Join(pkgDir, "lib"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "cjs-pkg",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte("module.exports = {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "lib", "sub.js"), []byte(
		"module.exports = function helper() { return 'ok'; };\n",
	), 0644)

	srv := &esmServer{
		moduleMap: map[string]string{"cjs-pkg": pkgDir},
		define:    map[string]string{"process.env.NODE_ENV": `"development"`},
	}

	req := httptest.NewRequest("GET", "/@deps/cjs-pkg/lib/sub.js", nil)
	rec := httptest.NewRecorder()
	srv.handleDepOnDemand(rec, req, "/@deps/cjs-pkg/lib/sub.js", time.Now())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "default") {
		t.Errorf("expected default export for CJS function, got:\n%s", body)
	}
}

// TestHandleDepOnDemand_ExtensionlessURL tests that the handler resolves
// a request for "/@deps/pkg/sub.js" when the file on disk has the extension.
func TestHandleDepOnDemand_ExtensionlessURL(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, "ext-pkg")
	os.MkdirAll(filepath.Join(pkgDir, "lib"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "ext-pkg",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte("module.exports = {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "lib", "utils.js"), []byte(
		"export function greet() { return 'hello'; }\n",
	), 0644)

	srv := &esmServer{
		moduleMap: map[string]string{"ext-pkg": pkgDir},
		define:    map[string]string{"process.env.NODE_ENV": `"development"`},
	}

	// Request with .js extension — the file is lib/utils.js
	req := httptest.NewRequest("GET", "/@deps/ext-pkg/lib/utils.js", nil)
	rec := httptest.NewRecorder()
	srv.handleDepOnDemand(rec, req, "/@deps/ext-pkg/lib/utils.js", time.Now())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "greet") {
		t.Errorf("expected 'greet' in output, got:\n%s", body)
	}
}

// TestHandleDepOnDemand_ESMNamedExports tests that ESM subpaths with only
// named exports (no default) are served correctly.
func TestHandleDepOnDemand_ESMNamedExports(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, "esm-pkg")
	os.MkdirAll(filepath.Join(pkgDir, "lib"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "esm-pkg",
  "version": "1.0.0",
  "type": "module",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte("export default {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "lib", "helpers.js"), []byte(
		"export function foo() { return 1; }\nexport function bar() { return 2; }\n",
	), 0644)

	srv := &esmServer{
		moduleMap: map[string]string{"esm-pkg": pkgDir},
		define:    map[string]string{"process.env.NODE_ENV": `"development"`},
	}

	req := httptest.NewRequest("GET", "/@deps/esm-pkg/lib/helpers.js", nil)
	rec := httptest.NewRecorder()
	srv.handleDepOnDemand(rec, req, "/@deps/esm-pkg/lib/helpers.js", time.Now())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "foo") {
		t.Errorf("expected 'foo' in output, got:\n%s", body)
	}
	if !strings.Contains(body, "bar") {
		t.Errorf("expected 'bar' in output, got:\n%s", body)
	}
}

// TestHandleDepOnDemand_CSSSubpath tests that CSS files from npm packages
// are served as style-injecting JS modules.
func TestHandleDepOnDemand_CSSSubpath(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, "css-pkg")
	os.MkdirAll(filepath.Join(pkgDir, "dist"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "css-pkg",
  "version": "1.0.0",
  "main": "index.js",
  "exports": {
    ".": "./index.js",
    "./styles.css": "./dist/styles.css"
  }
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte("module.exports = {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "dist", "styles.css"), []byte(
		".btn { color: red; }\n",
	), 0644)

	srv := &esmServer{
		moduleMap: map[string]string{"css-pkg": pkgDir},
		define:    map[string]string{"process.env.NODE_ENV": `"development"`},
	}

	req := httptest.NewRequest("GET", "/@deps/css-pkg/styles.css", nil)
	rec := httptest.NewRecorder()
	srv.handleDepOnDemand(rec, req, "/@deps/css-pkg/styles.css", time.Now())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/javascript" {
		t.Errorf("expected Content-Type application/javascript, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "style") {
		t.Errorf("expected style injection in output, got:\n%s", body)
	}
	if !strings.Contains(body, ".btn") {
		t.Errorf("expected CSS content in output, got:\n%s", body)
	}
}
