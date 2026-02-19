package esmdev

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSourceFileExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".js", true},
		{".jsx", true},
		{".ts", true},
		{".tsx", true},
		{".mjs", true},
		{".css", false},
		{".html", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isSourceFileExt(tt.ext)
			if got != tt.want {
				t.Errorf("isSourceFileExt(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

func TestResolveSourceFile(t *testing.T) {
	dir := t.TempDir()

	// Create main.tsx
	mainTSX := filepath.Join(dir, "main.tsx")
	if err := os.WriteFile(mainTSX, []byte("export default function App() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create components/index.ts
	compDir := filepath.Join(dir, "components")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatal(err)
	}
	compIndex := filepath.Join(compDir, "index.ts")
	if err := os.WriteFile(compIndex, []byte("export {}"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("direct match", func(t *testing.T) {
		got := resolveSourceFile(dir, "/main.tsx")
		if got != mainTSX {
			t.Errorf("resolveSourceFile(dir, /main.tsx) = %q, want %q", got, mainTSX)
		}
	})

	t.Run("extension replacement js to tsx", func(t *testing.T) {
		got := resolveSourceFile(dir, "/main.js")
		if got != mainTSX {
			t.Errorf("resolveSourceFile(dir, /main.js) = %q, want %q", got, mainTSX)
		}
	})

	t.Run("extensionless finds tsx", func(t *testing.T) {
		got := resolveSourceFile(dir, "/main")
		if got != mainTSX {
			t.Errorf("resolveSourceFile(dir, /main) = %q, want %q", got, mainTSX)
		}
	})

	t.Run("index file resolution", func(t *testing.T) {
		got := resolveSourceFile(dir, "/components")
		if got != compIndex {
			t.Errorf("resolveSourceFile(dir, /components) = %q, want %q", got, compIndex)
		}
	})

	t.Run("not found returns empty", func(t *testing.T) {
		got := resolveSourceFile(dir, "/nonexistent")
		if got != "" {
			t.Errorf("resolveSourceFile(dir, /nonexistent) = %q, want empty string", got)
		}
	})
}
