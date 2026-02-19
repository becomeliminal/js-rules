package esmdev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrebundlePackage_NestedNodeModules verifies that version-conflicted
// packages nested in node_modules/ get bundled into the parent, not
// externalized. npm puts packages in <parent>/node_modules/<pkg> only when
// the version conflicts with the hoisted copy. If we externalize them, the
// import map points to the hoisted (wrong) version.
//
// Scenario modelled after the real bug: porto needs zod@4 (has ./mini) but
// the hoisted zod@3 (lacks ./mini) is what the import map resolves to.
func TestPrebundlePackage_NestedNodeModules(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	// --- parent-pkg: imports "nested-dep/sub" ---
	parentDir := filepath.Join(dir, "parent-pkg")
	os.MkdirAll(parentDir, 0755)
	os.WriteFile(filepath.Join(parentDir, "package.json"), []byte(`{
  "name": "parent-pkg",
  "version": "1.0.0",
  "exports": {
    ".": "./index.js"
  }
}`), 0644)
	os.WriteFile(filepath.Join(parentDir, "index.js"), []byte(
		"export { value } from \"nested-dep/sub\";\n",
	), 0644)

	// --- nested-dep@2.0.0 (version-conflicted, in parent's node_modules) ---
	// This version HAS the ./sub export — the one parent-pkg actually needs.
	nestedDir := filepath.Join(parentDir, "node_modules", "nested-dep")
	os.MkdirAll(nestedDir, 0755)
	os.WriteFile(filepath.Join(nestedDir, "package.json"), []byte(`{
  "name": "nested-dep",
  "version": "2.0.0",
  "exports": {
    ".": "./index.js",
    "./sub": "./sub.js"
  }
}`), 0644)
	os.WriteFile(filepath.Join(nestedDir, "index.js"), []byte(
		"export const name = \"nested-dep-v2\";\n",
	), 0644)
	os.WriteFile(filepath.Join(nestedDir, "sub.js"), []byte(
		"export const value = \"from-nested-v2\";\n",
	), 0644)

	// --- Run prebundlePackage ---
	result := prebundlePackage("parent-pkg", parentDir, nil, outdir)

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}

	if len(result.depCache) == 0 {
		t.Fatal("expected non-empty depCache")
	}

	// The bundled output should NOT contain an externalized import of
	// "nested-dep/sub" — the nested dep's code must be inlined.
	for path, content := range result.depCache {
		text := string(content)
		if strings.Contains(text, `from "nested-dep/sub"`) || strings.Contains(text, `from "nested-dep"`) {
			t.Errorf("file %s still has externalized nested-dep import; nested dep was not bundled in:\n%s", path, text)
		}
	}

	// The inlined value from the nested dep should appear in the output.
	found := false
	for _, content := range result.depCache {
		if strings.Contains(string(content), "from-nested-v2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'from-nested-v2' from nested dep to be inlined in bundle output")
		for path, content := range result.depCache {
			t.Logf("  %s: %s", path, string(content))
		}
	}
}

// TestPrebundlePackage_NestedScopedPackage verifies that scoped packages
// (e.g., @scope/pkg) nested in node_modules are also bundled in.
func TestPrebundlePackage_NestedScopedPackage(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	parentDir := filepath.Join(dir, "parent-pkg")
	os.MkdirAll(parentDir, 0755)
	os.WriteFile(filepath.Join(parentDir, "package.json"), []byte(`{
  "name": "parent-pkg",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(parentDir, "index.js"), []byte(
		"export { helper } from \"@scope/util\";\n",
	), 0644)

	// Scoped package nested in parent's node_modules
	scopedDir := filepath.Join(parentDir, "node_modules", "@scope", "util")
	os.MkdirAll(scopedDir, 0755)
	os.WriteFile(filepath.Join(scopedDir, "package.json"), []byte(`{
  "name": "@scope/util",
  "version": "2.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(scopedDir, "index.js"), []byte(
		"export const helper = \"scoped-v2\";\n",
	), 0644)

	result := prebundlePackage("parent-pkg", parentDir, nil, outdir)

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}

	// Should NOT have externalized @scope/util
	for path, content := range result.depCache {
		text := string(content)
		if strings.Contains(text, `from "@scope/util"`) {
			t.Errorf("file %s still has externalized @scope/util import:\n%s", path, text)
		}
	}

	// The inlined value should be present
	found := false
	for _, content := range result.depCache {
		if strings.Contains(string(content), "scoped-v2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'scoped-v2' from nested scoped dep to be inlined")
	}
}

// TestPrebundlePackage_NoNestedNodeModules verifies that packages without
// nested node_modules still work correctly (no regression).
func TestPrebundlePackage_NoNestedNodeModules(t *testing.T) {
	dir := t.TempDir()
	outdir := filepath.Join(dir, "outdir")

	pkgDir := filepath.Join(dir, "simple-pkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{
  "name": "simple-pkg",
  "version": "1.0.0",
  "main": "index.js"
}`), 0644)
	os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte(
		"export const x = 42;\n",
	), 0644)

	result := prebundlePackage("simple-pkg", pkgDir, nil, outdir)

	if result.err != nil {
		t.Fatalf("prebundlePackage failed: %v", result.err)
	}
	if len(result.depCache) == 0 {
		t.Fatal("expected non-empty depCache")
	}

	found := false
	for _, content := range result.depCache {
		if strings.Contains(string(content), "42") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected '42' in bundle output")
	}
}
