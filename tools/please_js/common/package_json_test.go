package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchExports_Wildcard(t *testing.T) {
	tests := []struct {
		name     string
		exports  *exportValue
		subpath  string
		platform string
		want     string
	}{
		{
			name: "simple wildcard",
			exports: &exportValue{Map: map[string]*exportValue{
				".":       {Path: "./index.js"},
				"./lib/*": {Path: "./lib/*.js"},
			}},
			subpath:  "./lib/foo",
			platform: "browser",
			want:     "./lib/foo.js",
		},
		{
			name: "wildcard with conditions",
			exports: &exportValue{Map: map[string]*exportValue{
				".": {Path: "./index.js"},
				"./lib/languages/*": {Map: map[string]*exportValue{
					"import":  {Path: "./es/languages/*.js"},
					"require": {Path: "./lib/languages/*.js"},
				}},
			}},
			subpath:  "./lib/languages/1c",
			platform: "browser",
			want:     "./es/languages/1c.js",
		},
		{
			name: "exact match takes priority over wildcard",
			exports: &exportValue{Map: map[string]*exportValue{
				".":        {Path: "./index.js"},
				"./react":  {Path: "./react-entry.js"},
				"./*":      {Path: "./src/*.js"},
			}},
			subpath:  "./react",
			platform: "browser",
			want:     "./react-entry.js",
		},
		{
			name: "no match — different prefix",
			exports: &exportValue{Map: map[string]*exportValue{
				".":       {Path: "./index.js"},
				"./lib/*": {Path: "./lib/*.js"},
			}},
			subpath:  "./src/foo",
			platform: "browser",
			want:     "",
		},
		{
			name: "wildcard root subpath",
			exports: &exportValue{Map: map[string]*exportValue{
				".":   {Path: "./index.js"},
				"./*": {Path: "./src/*.js"},
			}},
			subpath:  "./utils",
			platform: "browser",
			want:     "./src/utils.js",
		},
		{
			name: "array fallback — condition object first",
			exports: &exportValue{Map: map[string]*exportValue{
				".": {Path: "./index.js"},
				"./helpers/extends": {Array: []*exportValue{
					{Map: map[string]*exportValue{
						"node":    {Path: "./helpers/extends.js"},
						"import":  {Path: "./helpers/esm/extends.js"},
						"default": {Path: "./helpers/extends.js"},
					}},
					{Path: "./helpers/extends.js"},
				}},
			}},
			subpath:  "./helpers/extends",
			platform: "browser",
			want:     "./helpers/esm/extends.js",
		},
		{
			name: "array fallback — node platform",
			exports: &exportValue{Map: map[string]*exportValue{
				"./helpers/extends": {Array: []*exportValue{
					{Map: map[string]*exportValue{
						"node":    {Path: "./helpers/extends.js"},
						"import":  {Path: "./helpers/esm/extends.js"},
						"default": {Path: "./helpers/extends.js"},
					}},
					{Path: "./helpers/extends.js"},
				}},
			}},
			subpath:  "./helpers/extends",
			platform: "node",
			want:     "./helpers/extends.js",
		},
		{
			name: "array fallback — string only",
			exports: &exportValue{Map: map[string]*exportValue{
				"./foo": {Array: []*exportValue{
					{Path: "./foo.js"},
				}},
			}},
			subpath:  "./foo",
			platform: "browser",
			want:     "./foo.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchExports(tt.exports, tt.subpath, tt.platform)
			if got != tt.want {
				t.Errorf("matchExports(%q, %q) = %q, want %q", tt.subpath, tt.platform, got, tt.want)
			}
		})
	}
}

func TestResolvePackageEntry_Wildcard(t *testing.T) {
	dir := t.TempDir()

	// Create a package with wildcard exports like highlight.js
	pkgDir := filepath.Join(dir, "hljs")
	os.MkdirAll(filepath.Join(pkgDir, "es", "languages"), 0755)
	os.MkdirAll(filepath.Join(pkgDir, "lib", "languages"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "hljs",
  "version": "1.0.0",
  "exports": {
    ".": "./index.js",
    "./lib/languages/*": {
      "import": "./es/languages/*.js",
      "require": "./lib/languages/*.js"
    }
  }
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte("export default {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "es", "languages", "javascript.js"), []byte("export default function(hljs) { return {}; };\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "lib", "languages", "javascript.js"), []byte("module.exports = function(hljs) { return {}; };\n"), 0644)

	// Browser platform → should resolve via "import" condition → es/languages/javascript.js
	got := ResolvePackageEntry(pkgDir, "./lib/languages/javascript", "browser")
	want := filepath.Join(pkgDir, "es", "languages", "javascript.js")
	if got != want {
		t.Errorf("ResolvePackageEntry(./lib/languages/javascript, browser) = %q, want %q", got, want)
	}

	// Root subpath still works
	gotRoot := ResolvePackageEntry(pkgDir, ".", "browser")
	wantRoot := filepath.Join(pkgDir, "index.js")
	if gotRoot != wantRoot {
		t.Errorf("ResolvePackageEntry(., browser) = %q, want %q", gotRoot, wantRoot)
	}
}

func TestResolvePackageEntry_ArrayExports(t *testing.T) {
	dir := t.TempDir()

	// Simulate @babel/runtime: array fallback exports with no root "." entry
	pkgDir := filepath.Join(dir, "babel-runtime")
	os.MkdirAll(filepath.Join(pkgDir, "helpers", "esm"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "@babel/runtime",
  "version": "7.26.0",
  "exports": {
    "./helpers/extends": [
      {"node": "./helpers/extends.js", "import": "./helpers/esm/extends.js", "default": "./helpers/extends.js"},
      "./helpers/extends.js"
    ],
    "./helpers/objectSpread2": [
      {"node": "./helpers/objectSpread2.js", "import": "./helpers/esm/objectSpread2.js", "default": "./helpers/objectSpread2.js"},
      "./helpers/objectSpread2.js"
    ]
  }
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "helpers", "extends.js"), []byte("module.exports = function() {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "helpers", "esm", "extends.js"), []byte("export default function() {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "helpers", "objectSpread2.js"), []byte("module.exports = function() {};\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "helpers", "esm", "objectSpread2.js"), []byte("export default function() {};\n"), 0644)

	// Browser platform → should resolve via "import" condition → esm/extends.js
	got := ResolvePackageEntry(pkgDir, "./helpers/extends", "browser")
	want := filepath.Join(pkgDir, "helpers", "esm", "extends.js")
	if got != want {
		t.Errorf("ResolvePackageEntry(./helpers/extends, browser) = %q, want %q", got, want)
	}

	// Node platform → should resolve via "node" condition → helpers/extends.js
	gotNode := ResolvePackageEntry(pkgDir, "./helpers/extends", "node")
	wantNode := filepath.Join(pkgDir, "helpers", "extends.js")
	if gotNode != wantNode {
		t.Errorf("ResolvePackageEntry(./helpers/extends, node) = %q, want %q", gotNode, wantNode)
	}

	// No root export → should return empty
	gotRoot := ResolvePackageEntry(pkgDir, ".", "browser")
	if gotRoot != "" {
		t.Errorf("ResolvePackageEntry(., browser) = %q, want empty (no root export)", gotRoot)
	}
}

func TestResolvePackageEntry_MainWithoutExtension(t *testing.T) {
	dir := t.TempDir()

	// Simulate text-encoding-utf-8: "main": "lib/encoding.lib" with actual file "lib/encoding.lib.js"
	pkgDir := filepath.Join(dir, "text-encoding-utf-8")
	os.MkdirAll(filepath.Join(pkgDir, "lib"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "text-encoding-utf-8",
  "main": "lib/encoding.lib"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "lib", "encoding.lib.js"), []byte("module.exports = {};\n"), 0644)

	got := ResolvePackageEntry(pkgDir, ".", "browser")
	want := filepath.Join(pkgDir, "lib", "encoding.lib.js")
	if got != want {
		t.Errorf("ResolvePackageEntry(., browser) = %q, want %q", got, want)
	}

	// Also works for node platform
	gotNode := ResolvePackageEntry(pkgDir, ".", "node")
	if gotNode != want {
		t.Errorf("ResolvePackageEntry(., node) = %q, want %q", gotNode, want)
	}
}

func TestResolvePackageEntry_ModuleWithoutExtension(t *testing.T) {
	dir := t.TempDir()

	// "module" field without .js extension
	pkgDir := filepath.Join(dir, "test-pkg")
	os.MkdirAll(filepath.Join(pkgDir, "es"), 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "test-pkg",
  "module": "es/index"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "es", "index.js"), []byte("export default {};\n"), 0644)

	got := ResolvePackageEntry(pkgDir, ".", "browser")
	want := filepath.Join(pkgDir, "es", "index.js")
	if got != want {
		t.Errorf("ResolvePackageEntry(., browser) = %q, want %q", got, want)
	}
}
