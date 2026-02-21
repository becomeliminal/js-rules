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
