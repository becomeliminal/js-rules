package esmdev

import (
	"strings"
	"testing"
)

func TestDetectComponents(t *testing.T) {
	tests := []struct {
		name string
		code string
		want []string
	}{
		{
			name: "plain function component",
			code: "function App() {\n  return <div/>;\n}",
			want: []string{"App"},
		},
		{
			name: "export default function",
			code: "export default function Home() {\n  return <div/>;\n}",
			want: []string{"Home"},
		},
		{
			name: "named export function",
			code: "export function Header() {\n  return <div/>;\n}",
			want: []string{"Header"},
		},
		{
			name: "arrow function component",
			code: "const Counter = () => {\n  return <div/>;\n};",
			want: []string{"Counter"},
		},
		{
			name: "exported const with React.memo",
			code: "export const Button = React.memo(\n  () => <button/>\n);",
			want: []string{"Button"},
		},
		{
			name: "lowercase function not detected",
			code: "function helper() {\n  return 42;\n}",
			want: nil,
		},
		{
			name: "lowercase const not detected",
			code: "const value = 42;",
			want: nil,
		},
		{
			name: "multiple components both detected",
			code: "function App() {\n  return <div/>;\n}\nconst Sidebar = () => {\n  return <nav/>;\n};",
			want: []string{"App", "Sidebar"},
		},
		{
			name: "duplicate names counted once",
			code: "function App() {}\nconst App = () => {};",
			want: []string{"App"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectComponents(tt.code)
			if len(got) != len(tt.want) {
				t.Fatalf("detectComponents() returned %d components %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("detectComponents()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestInjectRefreshRegistration(t *testing.T) {
	original := []byte("const App = () => <div>hello</div>;")
	urlPath := "/src/App.tsx"
	components := []string{"App", "Header"}

	result := string(injectRefreshRegistration(original, urlPath, components))

	t.Run("preamble at start", func(t *testing.T) {
		if !strings.HasPrefix(result, "import.meta.hot = window.__ESM_HMR__?.createContext(") {
			t.Errorf("expected import.meta.hot preamble at start, got:\n%s", result[:80])
		}
	})

	t.Run("contains RefreshReg for each component", func(t *testing.T) {
		for _, name := range components {
			needle := `window.$RefreshReg$(` + name + `, "` + name + `")`
			if !strings.Contains(result, needle) {
				t.Errorf("expected %q in output, not found", needle)
			}
		}
	})

	t.Run("contains hot accept at end", func(t *testing.T) {
		if !strings.HasSuffix(strings.TrimRight(result, "\n"), "import.meta.hot?.accept();") {
			t.Errorf("expected import.meta.hot?.accept() at end, got:\n%s", result[len(result)-60:])
		}
	})

	t.Run("original code preserved", func(t *testing.T) {
		if !strings.Contains(result, string(original)) {
			t.Error("original code not found in output")
		}
	})

	t.Run("url path is quoted", func(t *testing.T) {
		if !strings.Contains(result, `"/src/App.tsx"`) {
			t.Error("expected quoted URL path in output")
		}
	})
}
