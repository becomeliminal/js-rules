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

func TestSourcefileFromResolved(t *testing.T) {
	tests := []struct {
		name        string
		packageRoot string
		sourceRoot  string
		resolved    string
		want        string
	}{
		{
			name:        "relative to packageRoot",
			packageRoot: "/home/user/project",
			sourceRoot:  "",
			resolved:    "/home/user/project/src/components/index.ts",
			want:        "/src/components/index.ts",
		},
		{
			name:        "relative to sourceRoot when not under packageRoot",
			packageRoot: "/home/user/project",
			sourceRoot:  "/home/user/other",
			resolved:    "/home/user/other/lib/utils.ts",
			want:        "/lib/utils.ts",
		},
		{
			name:        "packageRoot preferred over sourceRoot",
			packageRoot: "/home/user/project",
			sourceRoot:  "/home/user/project",
			resolved:    "/home/user/project/main.tsx",
			want:        "/main.tsx",
		},
		{
			name:        "falls back to resolved when outside both roots",
			packageRoot: "/home/user/project",
			sourceRoot:  "/home/user/other",
			resolved:    "/somewhere/else/file.ts",
			want:        "/somewhere/else/file.ts",
		},
		{
			name:        "empty sourceRoot skips sourceRoot check",
			packageRoot: "/home/user/project",
			sourceRoot:  "",
			resolved:    "/other/path/file.ts",
			want:        "/other/path/file.ts",
		},
		{
			name:        "barrel index file gets full path",
			packageRoot: "/home/user/project/src",
			sourceRoot:  "",
			resolved:    "/home/user/project/src/components/editor/extensions/index.ts",
			want:        "/components/editor/extensions/index.ts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourcefileFromResolved(tt.packageRoot, tt.sourceRoot, tt.resolved)
			if got != tt.want {
				t.Errorf("sourcefileFromResolved(%q, %q, %q) = %q, want %q",
					tt.packageRoot, tt.sourceRoot, tt.resolved, got, tt.want)
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
