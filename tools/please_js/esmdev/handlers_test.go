package esmdev

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
