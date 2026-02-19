package esmdev

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestPackageNameFromSpec(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want string
	}{
		{
			name: "bare package",
			spec: "react",
			want: "react",
		},
		{
			name: "unscoped with subpath",
			spec: "react-dom/client",
			want: "react-dom",
		},
		{
			name: "scoped package",
			spec: "@scope/pkg",
			want: "@scope/pkg",
		},
		{
			name: "scoped package with subpath",
			spec: "@scope/pkg/sub/path",
			want: "@scope/pkg",
		},
		{
			name: "simple unscoped",
			spec: "lodash",
			want: "lodash",
		},
		{
			name: "bare scope without package name",
			spec: "@scope",
			want: "@scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := packageNameFromSpec(tt.spec)
			if got != tt.want {
				t.Errorf("packageNameFromSpec(%q) = %q, want %q", tt.spec, got, tt.want)
			}
		})
	}
}

func TestScanSourceImports_BasicTSX(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.tsx")
	content := `import React from "react";
import { useState } from "react";
import "./local";
import "unknown-pkg";
`
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{
		"react": "/some/path",
	}

	used := scanSourceImports(dir, moduleMap)

	if !used["react"] {
		t.Error("expected 'react' to be in used imports")
	}
	if used["./local"] {
		t.Error("relative import './local' should not be in used imports")
	}
	if used["unknown-pkg"] {
		t.Error("'unknown-pkg' not in moduleMap should not be in used imports")
	}
	if len(used) != 1 {
		t.Errorf("expected 1 used import, got %d: %v", len(used), used)
	}
}

func TestScanSourceImports_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Create a node_modules directory with a source file that imports lodash
	nmDir := filepath.Join(dir, "node_modules", "some-pkg")
	if err := os.MkdirAll(nmDir, 0755); err != nil {
		t.Fatal(err)
	}
	nmFile := filepath.Join(nmDir, "index.js")
	if err := os.WriteFile(nmFile, []byte(`import "lodash";`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a real source file that imports react
	srcFile := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(srcFile, []byte(`import "react";`), 0644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{
		"react":  "/some/path",
		"lodash": "/some/other/path",
	}

	used := scanSourceImports(dir, moduleMap)

	if !used["react"] {
		t.Error("expected 'react' from main.ts to be in used imports")
	}
	if used["lodash"] {
		t.Error("'lodash' from node_modules should be skipped")
	}
}

func TestScanSourceImports_MultipleFileTypes(t *testing.T) {
	dir := t.TempDir()

	// .ts file
	tsFile := filepath.Join(dir, "utils.ts")
	if err := os.WriteFile(tsFile, []byte(`import "lodash";`), 0644); err != nil {
		t.Fatal(err)
	}

	// .jsx file
	jsxFile := filepath.Join(dir, "component.jsx")
	if err := os.WriteFile(jsxFile, []byte(`import "react";`), 0644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{
		"lodash": "/some/path",
		"react":  "/some/path",
	}

	used := scanSourceImports(dir, moduleMap)

	if !used["lodash"] {
		t.Error("expected 'lodash' from .ts file to be in used imports")
	}
	if !used["react"] {
		t.Error("expected 'react' from .jsx file to be in used imports")
	}
}

func TestScanSourceImports_IgnoresNonSourceFiles(t *testing.T) {
	dir := t.TempDir()

	// .css file should be ignored
	cssFile := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(cssFile, []byte(`/* import "react"; */`), 0644); err != nil {
		t.Fatal(err)
	}

	// .json file should be ignored
	jsonFile := filepath.Join(dir, "data.json")
	if err := os.WriteFile(jsonFile, []byte(`{"from": "react"}`), 0644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{
		"react": "/some/path",
	}

	used := scanSourceImports(dir, moduleMap)

	if len(used) != 0 {
		t.Errorf("expected no imports from non-source files, got %v", used)
	}
}

func TestScanSourceImports_ScopedPackageSubpath(t *testing.T) {
	dir := t.TempDir()

	srcFile := filepath.Join(dir, "app.tsx")
	content := `import { createRoot } from "@my-org/ui/client";
`
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	moduleMap := map[string]string{
		"@my-org/ui": "/some/path",
	}

	used := scanSourceImports(dir, moduleMap)

	// The full specifier including subpath should be recorded
	if !used["@my-org/ui/client"] {
		t.Errorf("expected '@my-org/ui/client' to be in used imports, got %v", used)
	}
}

func TestFindSubpathExports_BasicExports(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{
  "name": "my-pkg",
  "exports": {
    ".": "./index.js",
    "./client": "./client.js"
  }
}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	subpaths := findSubpathExports(dir)

	if len(subpaths) != 1 {
		t.Fatalf("expected 1 subpath, got %d: %v", len(subpaths), subpaths)
	}
	if subpaths[0] != "./client" {
		t.Errorf("expected './client', got %q", subpaths[0])
	}
}

func TestFindSubpathExports_SkipsWildcards(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{
  "exports": {
    ".": "./index.js",
    "./client": "./client.js",
    "./locale/*": "./locale/*.js"
  }
}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	subpaths := findSubpathExports(dir)

	for _, sp := range subpaths {
		if sp == "./locale/*" {
			t.Error("wildcard export './locale/*' should be skipped")
		}
	}
	if len(subpaths) != 1 {
		t.Fatalf("expected 1 subpath (only ./client), got %d: %v", len(subpaths), subpaths)
	}
	if subpaths[0] != "./client" {
		t.Errorf("expected './client', got %q", subpaths[0])
	}
}

func TestFindSubpathExports_SkipsTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{
  "exports": {
    ".": "./index.js",
    "./utils/": "./utils/"
  }
}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	subpaths := findSubpathExports(dir)

	if len(subpaths) != 0 {
		t.Errorf("expected 0 subpaths (trailing slash should be skipped), got %d: %v", len(subpaths), subpaths)
	}
}

func TestFindSubpathExports_NoExportsField(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{
  "name": "my-pkg",
  "main": "./index.js"
}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	subpaths := findSubpathExports(dir)

	if subpaths != nil {
		t.Errorf("expected nil for no exports field, got %v", subpaths)
	}
}

func TestFindSubpathExports_StringExports(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{
  "name": "my-pkg",
  "exports": "./index.js"
}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	subpaths := findSubpathExports(dir)

	if subpaths != nil {
		t.Errorf("expected nil for string exports, got %v", subpaths)
	}
}

func TestFindSubpathExports_MissingPackageJSON(t *testing.T) {
	dir := t.TempDir()

	subpaths := findSubpathExports(dir)

	if subpaths != nil {
		t.Errorf("expected nil for missing package.json, got %v", subpaths)
	}
}

func TestFindSubpathExports_MultipleSubpaths(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := filepath.Join(dir, "package.json")
	content := `{
  "exports": {
    ".": "./index.js",
    "./client": "./client.js",
    "./server": "./server.js",
    "./utils": "./utils.js"
  }
}`
	if err := os.WriteFile(pkgJSON, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	subpaths := findSubpathExports(dir)
	sort.Strings(subpaths)

	expected := []string{"./client", "./server", "./utils"}
	if len(subpaths) != len(expected) {
		t.Fatalf("expected %d subpaths, got %d: %v", len(expected), len(subpaths), subpaths)
	}
	for i, want := range expected {
		if subpaths[i] != want {
			t.Errorf("subpaths[%d] = %q, want %q", i, subpaths[i], want)
		}
	}
}
