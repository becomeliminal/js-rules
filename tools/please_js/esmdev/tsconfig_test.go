package esmdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "line comments removed",
			input: "{\n  // this is a comment\n  \"key\": \"value\"\n}",
			want:  "{\n  \n  \"key\": \"value\"\n}",
		},
		{
			name:  "block comments removed",
			input: "{\n  /* block comment */\n  \"key\": \"value\"\n}",
			want:  "{\n  \n  \"key\": \"value\"\n}",
		},
		{
			name:  "trailing commas removed",
			input: "{\"a\": 1, \"b\": 2,}",
			want:  "{\"a\": 1, \"b\": 2}",
		},
		{
			name:  "trailing comma in array",
			input: "[1, 2, 3,]",
			want:  "[1, 2, 3]",
		},
		{
			name:  "trailing comma with whitespace",
			input: "{\"a\": 1,\n}",
			want:  "{\"a\": 1}",
		},
		{
			name:  "comments inside strings preserved",
			input: "{\"key\": \"value // not a comment\"}",
			want:  "{\"key\": \"value // not a comment\"}",
		},
		{
			name:  "block comment inside string preserved",
			input: "{\"key\": \"value /* not a comment */ more\"}",
			want:  "{\"key\": \"value /* not a comment */ more\"}",
		},
		{
			name:  "escaped quotes in strings handled",
			input: "{\"key\": \"has \\\"escaped\\\" quotes // still string\"}",
			want:  "{\"key\": \"has \\\"escaped\\\" quotes // still string\"}",
		},
		{
			name:  "escaped quote before comment",
			input: "{\"k\": \"val\\\"\" // comment\n}",
			want:  "{\"k\": \"val\\\"\" \n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(stripJSONC([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("stripJSONC() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestStripJSONC_ComplexProducesValidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "multiple comment types in one document",
			input: `{
  // line comment
  "a": 1, /* inline block */
  /* multi
     line
     block */
  "b": 2,
}`,
		},
		{
			name: "real-world tsconfig with comments and trailing commas",
			input: `{
  // TypeScript configuration
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    /* Path aliases for imports */
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"],
      "~utils": ["./src/utils/index.ts"],
    },
    "strict": true, // enable strict mode
  },
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripJSONC([]byte(tt.input))
			var parsed map[string]interface{}
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Errorf("stripJSONC() produced invalid JSON: %v\noutput:\n%s", err, got)
			}
		})
	}
}

func TestParseTsconfigPaths_Wildcard(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  "compilerOptions": {
    "paths": {
      "@/*": ["./src/*"]
    }
  }
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries == nil {
		t.Fatal("expected non-nil entries")
	}
	want := "/src/"
	if got := entries["@/"]; got != want {
		t.Errorf("entries[\"@/\"] = %q, want %q", got, want)
	}
}

func TestParseTsconfigPaths_Exact(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  "compilerOptions": {
    "paths": {
      "~utils": ["./src/utils"]
    }
  }
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries == nil {
		t.Fatal("expected non-nil entries")
	}
	want := "/src/utils"
	if got := entries["~utils"]; got != want {
		t.Errorf("entries[\"~utils\"] = %q, want %q", got, want)
	}
}

func TestParseTsconfigPaths_JSONC(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  // Path aliases
  "compilerOptions": {
    "paths": {
      /* wildcard alias */
      "@/*": ["./src/*"],
    },
  },
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries == nil {
		t.Fatal("expected non-nil entries for JSONC input")
	}
	want := "/src/"
	if got := entries["@/"]; got != want {
		t.Errorf("entries[\"@/\"] = %q, want %q", got, want)
	}
}

func TestParseTsconfigPaths_MissingFile(t *testing.T) {
	entries := parseTsconfigPaths("/nonexistent/tsconfig.json", "/nonexistent")
	if entries != nil {
		t.Errorf("expected nil for missing file, got %v", entries)
	}
}

func TestParseTsconfigPaths_NoPaths(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  "compilerOptions": {
    "target": "ES2020"
  }
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries != nil {
		t.Errorf("expected nil for no paths section, got %v", entries)
	}
}

func TestParseTsconfigPaths_EmptyPaths(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  "compilerOptions": {
    "paths": {}
  }
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries != nil {
		t.Errorf("expected nil for empty paths, got %v", entries)
	}
}

func TestParseTsconfigPaths_BaseUrl(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory to use as baseUrl
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0755); err != nil {
		t.Fatal(err)
	}

	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  "compilerOptions": {
    "baseUrl": "./app",
    "paths": {
      "@/*": ["./lib/*"]
    }
  }
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries == nil {
		t.Fatal("expected non-nil entries")
	}
	// With baseUrl=./app and target=./lib/*, the resolved path is app/lib/
	want := "/app/lib/"
	if got := entries["@/"]; got != want {
		t.Errorf("entries[\"@/\"] = %q, want %q", got, want)
	}
}

func TestParseTsconfigPaths_MultipleAliases(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")
	content := `{
  "compilerOptions": {
    "paths": {
      "@/*": ["./src/*"],
      "~components/*": ["./src/components/*"],
      "~utils": ["./src/utils/index.ts"]
    }
  }
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries == nil {
		t.Fatal("expected non-nil entries")
	}

	expected := map[string]string{
		"@/":           "/src/",
		"~components/": "/src/components/",
		"~utils":       "/src/utils/index.ts",
	}

	for alias, want := range expected {
		got, ok := entries[alias]
		if !ok {
			t.Errorf("missing entry for alias %q", alias)
			continue
		}
		if got != want {
			t.Errorf("entries[%q] = %q, want %q", alias, got, want)
		}
	}

	if len(entries) != len(expected) {
		t.Errorf("got %d entries, want %d", len(entries), len(expected))
	}
}

// TestParseTsconfigPaths_WildcardTrailingSlash is a regression test for
// https://github.com/becomeliminal/js-rules/issues/X
//
// In the browser, import map prefix matching requires a trailing slash on BOTH
// the key and value. Without it:
//
//	import "@/lib/wagmi"  with  {"@/": "/src"}  →  browser resolves "/lib/wagmi" (WRONG)
//	import "@/lib/wagmi"  with  {"@/": "/src/"} →  browser resolves "/src/lib/wagmi" (CORRECT)
//
// The original bug produced values without trailing slashes, causing:
//
//	Uncaught TypeError: Failed to resolve module specifier "@/lib/wagmi".
//	Relative references must start with either "/", "./", or "../".
//
// This test uses a JSONC tsconfig (comments + trailing commas) to exercise both
// bugs simultaneously: (A) JSONC parsing failure, (B) missing trailing slash.
func TestParseTsconfigPaths_WildcardTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.json")

	// Real-world tsconfig with comments and trailing commas — the kind that
	// every TypeScript project has and that caused Bug A (JSONC parse failure).
	content := `{
  "compilerOptions": {
    // Path aliases used by the app
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"],
    },
  },
}`
	if err := os.WriteFile(tsconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries := parseTsconfigPaths(tsconfig, dir)
	if entries == nil {
		t.Fatal("parseTsconfigPaths returned nil — JSONC parsing likely failed (Bug A)")
	}

	got, ok := entries["@/"]
	if !ok {
		t.Fatal("missing import map entry for \"@/\" — path alias not parsed")
	}

	// Bug B: the value MUST end with "/" for import map prefix matching to work.
	// Without it, "@/lib/wagmi" fails to resolve in the browser.
	want := "/src/"
	if got != want {
		t.Errorf("entries[\"@/\"] = %q, want %q (trailing slash required for prefix matching)", got, want)
	}

	// Verify the import map entry would correctly resolve "@/lib/wagmi"
	if !strings.HasSuffix(got, "/") {
		t.Errorf("import map value %q missing trailing slash — \"@/lib/wagmi\" would resolve to %q instead of %q",
			got, "/lib/wagmi", got+"lib/wagmi")
	}
}

// TestParseTsconfigPaths_RelativeTsconfigAbsoluteRoot is a regression test for
// the actual root cause of the "@/lib/wagmi" browser error.
//
// The dev server passes a relative --tsconfig path (e.g. "myapp/tsconfig.json")
// but computes packageRoot via filepath.Abs (e.g. "/home/user/myapp").
// Without filepath.Abs on tsconfigDir, filepath.Rel(absolute, relative) silently
// errors and every path alias is dropped — producing an empty import map.
func TestParseTsconfigPaths_RelativeTsconfigAbsoluteRoot(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "myapp")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}

	tsconfigAbs := filepath.Join(appDir, "tsconfig.json")
	if err := os.WriteFile(tsconfigAbs, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate the dev server: cd into dir, relative tsconfig, absolute packageRoot
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	relativeTsconfig := "myapp/tsconfig.json"    // relative (as passed by --tsconfig)
	absoluteRoot, _ := filepath.Abs("myapp")      // absolute (as computed by Run())

	entries := parseTsconfigPaths(relativeTsconfig, absoluteRoot)
	if entries == nil {
		t.Fatal("parseTsconfigPaths returned nil — relative/absolute path mismatch bug")
	}
	want := "/src/"
	if got := entries["@/"]; got != want {
		t.Errorf(`entries["@/"] = %q, want %q`, got, want)
	}
}
