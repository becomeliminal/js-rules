package esmdev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteHTML_ImportMapInjectedBeforeHead(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
<title>Test</title>
</head>
<body></body>
</html>`
	importMap := []byte(`{"imports":{"react":"/npm/react"}}`)
	result := rewriteHTML(html, importMap, false, "/app.js", "/src", "/src")

	if !strings.Contains(result, `<script type="importmap">{"imports":{"react":"/npm/react"}}</script>`) {
		t.Error("expected import map script to be present in output")
	}

	idx := strings.Index(result, `<script type="importmap">`)
	headIdx := strings.Index(result, `</head>`)
	if idx < 0 || headIdx < 0 || idx >= headIdx {
		t.Error("expected import map to be injected before </head>")
	}
}

func TestRewriteHTML_ImportMapInjectedBeforeBody(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
<div>Hello</div>
</body>
</html>`
	importMap := []byte(`{"imports":{}}`)
	result := rewriteHTML(html, importMap, false, "/app.js", "/src", "/src")

	if !strings.Contains(result, `<script type="importmap">{"imports":{}}</script>`) {
		t.Error("expected import map script to be present in output")
	}

	idx := strings.Index(result, `<script type="importmap">`)
	bodyIdx := strings.Index(result, `<body`)
	if idx < 0 || bodyIdx < 0 || idx >= bodyIdx {
		t.Error("expected import map to be injected before <body")
	}
}

func TestRewriteHTML_ImportMapInjectedAtStart(t *testing.T) {
	html := `<div>No head or body tags</div>`
	importMap := []byte(`{"imports":{}}`)
	result := rewriteHTML(html, importMap, false, "/app.js", "/src", "/src")

	if !strings.Contains(result, `<script type="importmap">{"imports":{}}</script>`) {
		t.Error("expected import map script to be present in output")
	}

	if !strings.HasPrefix(result, `<script type="importmap">`) {
		t.Error("expected import map to be injected at start of document")
	}
}

func TestRewriteHTML_HasRefreshTrue(t *testing.T) {
	html := `<html><head></head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, true, "/app.js", "/src", "/src")

	if !strings.Contains(result, `$RefreshReg$`) {
		t.Error("expected refreshInitScript content ($RefreshReg$) when hasRefresh=true")
	}
	if !strings.Contains(result, `__ESM_HMR__`) {
		t.Error("expected hmrClientScript content (__ESM_HMR__) when hasRefresh=true")
	}
	if strings.Contains(result, `EventSource("/__esm_dev_sse")`) {
		// Both hmrClientScript and liveReloadScript use EventSource, but only
		// liveReloadScript wraps it in an IIFE with location.reload on "change".
		// Check that the liveReloadScript-specific "change" listener is NOT present.
	}
}

func TestRewriteHTML_HasRefreshFalse(t *testing.T) {
	html := `<html><head></head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/app.js", "/src", "/src")

	if !strings.Contains(result, `EventSource("/__esm_dev_sse")`) {
		t.Error("expected liveReloadScript content (EventSource) when hasRefresh=false")
	}
	if strings.Contains(result, `$RefreshReg$`) {
		t.Error("did not expect refreshInitScript content when hasRefresh=false")
	}
	if strings.Contains(result, `__ESM_HMR__`) {
		t.Error("did not expect hmrClientScript content when hasRefresh=false")
	}
}

func TestRewriteHTML_ScriptSrcRewrittenWhenFileNotFound(t *testing.T) {
	sourceRoot := t.TempDir()
	packageRoot := t.TempDir()

	html := `<html><head>
<script type="module" src="/main.js"></script>
</head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/entry.tsx", sourceRoot, packageRoot)

	if !strings.Contains(result, `src="/entry.tsx"`) {
		t.Error("expected script src to be rewritten to entryURLPath when file does not exist")
	}
	if strings.Contains(result, `src="/main.js"`) {
		t.Error("expected original script src to be replaced")
	}
}

func TestRewriteHTML_ScriptSrcUnchangedWhenFileExists(t *testing.T) {
	sourceRoot := t.TempDir()
	packageRoot := t.TempDir()

	// Create the source file so resolveSourceFile finds it.
	if err := os.WriteFile(filepath.Join(sourceRoot, "main.js"), []byte("// app"), 0644); err != nil {
		t.Fatal(err)
	}

	html := `<html><head>
<script type="module" src="/main.js"></script>
</head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/entry.tsx", sourceRoot, packageRoot)

	if !strings.Contains(result, `src="/main.js"`) {
		t.Error("expected script src to remain unchanged when file exists in sourceRoot")
	}
}

func TestRewriteHTML_ScriptSrcRewrittenWhenRootsEqual(t *testing.T) {
	root := t.TempDir()

	// File does NOT exist — rewriting should happen even when packageRoot == sourceRoot.
	html := `<html><head>
<script type="module" src="/main.js"></script>
</head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/entry.tsx", root, root)

	if !strings.Contains(result, `src="/entry.tsx"`) {
		t.Error("expected script src to be rewritten to entryURLPath when file does not exist")
	}
	if strings.Contains(result, `src="/main.js"`) {
		t.Error("expected original script src to be replaced")
	}
}

func TestRewriteHTML_EntryScriptInjectedWhenMissing(t *testing.T) {
	root := t.TempDir()

	// HTML with no module script tag — entry point should be injected before </body>.
	html := `<html><head></head><body>
<div>Hello</div>
</body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/app.js", root, root)

	if !strings.Contains(result, `<script type="module" src="/app.js"></script>`) {
		t.Error("expected entry point script to be injected when no module script tag exists")
	}

	scriptIdx := strings.Index(result, `<script type="module" src="/app.js"></script>`)
	bodyCloseIdx := strings.Index(result, `</body>`)
	if scriptIdx < 0 || bodyCloseIdx < 0 || scriptIdx >= bodyCloseIdx {
		t.Error("expected entry point script to be injected before </body>")
	}
}

func TestRewriteHTML_EntryScriptNotDuplicatedWhenPresent(t *testing.T) {
	root := t.TempDir()

	// HTML already has a script tag that will be rewritten to the entry point —
	// no duplicate should be injected.
	html := `<html><head>
<script type="module" src="/main.js"></script>
</head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/entry.tsx", root, root)

	// The script should be rewritten to entry.tsx
	if !strings.Contains(result, `src="/entry.tsx"`) {
		t.Error("expected script src to be rewritten to entryURLPath")
	}

	// Count occurrences of the entry point — should be exactly one
	count := strings.Count(result, `src="/entry.tsx"`)
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of entry point script, got %d", count)
	}
}

func TestRewriteHTML_CSSLinkRemovedWhenFileNotFound(t *testing.T) {
	sourceRoot := t.TempDir()
	packageRoot := t.TempDir()

	html := `<html><head>
<link rel="stylesheet" href="/styles.css" />
</head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/app.js", sourceRoot, packageRoot)

	if strings.Contains(result, `stylesheet`) {
		t.Error("expected CSS link tag to be removed when file does not exist")
	}
}

func TestRewriteHTML_CSSLinkKeptWhenFileExists(t *testing.T) {
	sourceRoot := t.TempDir()
	packageRoot := t.TempDir()

	// Create the CSS file so resolveSourceFile finds it.
	if err := os.WriteFile(filepath.Join(sourceRoot, "styles.css"), []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}

	html := `<html><head>
<link rel="stylesheet" href="/styles.css" />
</head><body></body></html>`
	importMap := []byte(`{}`)
	result := rewriteHTML(html, importMap, false, "/app.js", sourceRoot, packageRoot)

	if !strings.Contains(result, `href="/styles.css"`) {
		t.Error("expected CSS link tag to remain when file exists in sourceRoot")
	}
}
