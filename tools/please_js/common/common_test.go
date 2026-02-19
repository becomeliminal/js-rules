package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

func TestIsNonPackageSpecifier(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// True: subpath imports
		{"#util", true},
		{"#internal/helper", true},

		// True: protocol URLs
		{"data:text/javascript,export default 42", true},
		{"https://cdn.example.com/lib.js", true},
		{"http://example.com/lib.js", true},
		{"file:///path.js", true},
		{"node:fs", true},

		// True: virtual modules
		{"\x00plugin:virtual", true},

		// False: real npm packages
		{"react", false},
		{"@scope/pkg", false},
		{"lodash", false},
		{"data-utils", false},
		{"https-proxy-agent", false},
	}
	for _, tt := range tests {
		got := isNonPackageSpecifier(tt.path)
		if got != tt.want {
			t.Errorf("isNonPackageSpecifier(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestUnknownExternalPlugin_SkipsDataURIs(t *testing.T) {
	// Build a file that imports a data: URI. With the fix, the plugin should
	// pass data: specifiers through to esbuild's native resolver, which
	// handles them natively. Without the fix, UnknownExternalPlugin would
	// mark the data: import as external.
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "entry.js")
	if err := os.WriteFile(entry, []byte(
		`import val from "data:text/javascript,export default 42";`+"\n"+
			`console.log(val);`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{} // empty — no known modules

	result := api.Build(api.BuildOptions{
		EntryPoints: []string{entry},
		Bundle:      true,
		Write:       false,
		Platform:    api.PlatformBrowser,
		Format:      api.FormatESModule,
		Plugins: []api.Plugin{
			UnknownExternalPlugin(moduleMap),
		},
	})

	if len(result.Errors) > 0 {
		var msgs []string
		for _, e := range result.Errors {
			msgs = append(msgs, e.Text)
		}
		t.Fatalf("build errors: %s", strings.Join(msgs, "; "))
	}

	if len(result.OutputFiles) == 0 {
		t.Fatal("expected output files, got none")
	}

	output := string(result.OutputFiles[0].Contents)

	// The data: URI should have been resolved and inlined by esbuild,
	// not left as an external import.
	if strings.Contains(output, `"data:`) {
		t.Errorf("output still contains external data: import:\n%s", output)
	}

	// The inlined value (42) should be present in the output.
	if !strings.Contains(output, "42") {
		t.Errorf("expected inlined value 42 in output:\n%s", output)
	}
}

func TestUnknownExternalPlugin_SkipsHashImports(t *testing.T) {
	// Create a temp package that uses #-prefixed imports (package.json "imports" field).
	tmp := t.TempDir()
	pkg := filepath.Join(tmp, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}

	// package.json with "imports" field mapping #util → ./util.js
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{
		"name": "mypkg",
		"main": "index.js",
		"imports": { "#util": "./util.js" }
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pkg, "index.js"), []byte(
		`export { hello } from "#util";`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pkg, "util.js"), []byte(
		`export const hello = "world";`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{"mypkg": pkg}

	result := api.Build(api.BuildOptions{
		EntryPoints: []string{filepath.Join(pkg, "index.js")},
		Bundle:      true,
		Write:       false,
		Platform:    api.PlatformBrowser,
		Format:      api.FormatESModule,
		Plugins: []api.Plugin{
			ModuleResolvePlugin(moduleMap, "browser"),
			NodeBuiltinEmptyPlugin(),
			UnknownExternalPlugin(moduleMap),
		},
	})

	if len(result.Errors) > 0 {
		var msgs []string
		for _, e := range result.Errors {
			msgs = append(msgs, e.Text)
		}
		t.Fatalf("build errors: %s", strings.Join(msgs, "; "))
	}

	if len(result.OutputFiles) == 0 {
		t.Fatal("expected output files, got none")
	}

	output := string(result.OutputFiles[0].Contents)

	// #util should have been resolved and inlined, not left as an external import.
	if strings.Contains(output, `"#util"`) || strings.Contains(output, `'#util'`) {
		t.Errorf("output still contains unresolved #util import:\n%s", output)
	}

	// The resolved value should be present in the bundled output.
	if !strings.Contains(output, `"world"`) && !strings.Contains(output, `world`) {
		t.Errorf("expected resolved value 'world' in output:\n%s", output)
	}
}
