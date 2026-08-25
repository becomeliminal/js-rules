package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/store"
)

func pkg(t *testing.T, root, entry, name, version string, deps ...store.Ref) store.Source {
	t.Helper()
	dir := filepath.Join(root, "src", entry)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"` + name + `","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return store.Source{
		Dir:  dir,
		Meta: store.Meta{Package: name, Version: version, Name: entry},
		Deps: deps,
	}
}

// ref names an entry as it appears in a node_modules directory.
func ref(as, entry string) store.Ref { return store.Ref{As: as, Entry: entry} }

// resolves follows every symlink. Asserting on link text would pass for a link
// that points somewhere plausible but wrong, which is the failure this package
// exists to prevent.
func resolves(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func TestScopedAndUnscopedLinksResolve(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "react_18_3_1", "react", "18.3.1"),
		// A scoped package sits inside a scope directory, so the store root is
		// one level further up than for an unscoped one.
		pkg(t, root, "scope_thing_1_0_0", "@scope/thing", "1.0.0"),
	}
	if err := store.Build(tree, sources, []store.Ref{
		ref("react", "react_18_3_1"),
		ref("@scope/thing", "scope_thing_1_0_0"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"react", "@scope/thing"} {
		if !resolves(t, filepath.Join(tree, name, "package.json")) {
			target, _ := os.Readlink(filepath.Join(tree, name))
			t.Errorf("%s does not resolve; link points at %q", name, target)
		}
	}
}

func TestDepsResolveFromInsideTheirPackage(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "react_dom_18_3_1", "react-dom", "18.3.1",
			ref("react", "react_18_3_1"), ref("@scope/thing", "scope_thing_1_0_0")),
		pkg(t, root, "react_18_3_1", "react", "18.3.1"),
		pkg(t, root, "scope_thing_1_0_0", "@scope/thing", "1.0.0"),
	}
	if err := store.Build(tree, sources, []store.Ref{ref("react-dom", "react_dom_18_3_1")}); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(tree, store.StoreRoot, store.StoreDir("react_dom_18_3_1"), "node_modules")
	for _, dep := range []string{"react", "@scope/thing"} {
		if !resolves(t, filepath.Join(base, dep, "package.json")) {
			target, _ := os.Readlink(filepath.Join(base, dep))
			t.Errorf("react-dom cannot resolve %s; link points at %q", dep, target)
		}
	}
}

// npm's cycles are real and in core packages: @babel/core and
// @babel/helper-module-transforms require each other. Please rejects a cyclic
// build graph, so dependencies are names resolved here rather than build edges.
func TestCyclicDependenciesAreFine(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "babel_core", "@babel/core", "7.29.0",
			ref("@babel/helper-module-transforms", "babel_helper")),
		pkg(t, root, "babel_helper", "@babel/helper-module-transforms", "7.28.6",
			ref("@babel/core", "babel_core")),
	}
	if err := store.Build(tree, sources, []store.Ref{ref("@babel/core", "babel_core")}); err != nil {
		t.Fatal(err)
	}
	both := [][2]string{
		{"babel_core", "@babel/helper-module-transforms"},
		{"babel_helper", "@babel/core"},
	}
	for _, c := range both {
		p := filepath.Join(tree, store.StoreRoot, store.StoreDir(c[0]), "node_modules", c[1], "package.json")
		if !resolves(t, p) {
			t.Errorf("%s cannot resolve %s", c[0], c[1])
		}
	}
}

// An npm alias rebinds a package under another name. It holds no files, so the
// package stays a singleton -- two copies would be two module instances, which
// is how "invalid hook call" happens.
func TestOnePackageUnderTwoNamesAtTwoVersions(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "aspect_c_2_0_2", "@aspect-test/c", "2.0.2"),
		pkg(t, root, "aspect_c_1_0_0", "@aspect-test/c", "1.0.0"),
	}
	// The lockfile binds one of them under another name; that is carried on the
	// reference rather than needing an entry of its own.
	if err := store.Build(tree, sources, []store.Ref{
		ref("@aspect-test/c", "aspect_c_2_0_2"),
		ref("@aspect-test/c1", "aspect_c_1_0_0"),
	}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"@aspect-test/c":  "2.0.2",
		"@aspect-test/c1": "1.0.0",
	} {
		data, err := os.ReadFile(filepath.Join(tree, name, "package.json"))
		if err != nil {
			t.Errorf("%s does not resolve: %v", name, err)
			continue
		}
		if !strings.Contains(string(data), `"version":"`+want+`"`) {
			t.Errorf("%s resolved to the wrong version: %s", name, data)
		}
	}
}

func TestMissingClosureMemberIsAnError(t *testing.T) {
	root := t.TempDir()
	sources := []store.Source{
		pkg(t, root, "react_dom", "react-dom", "18.3.1", ref("react", "react_18_3_1")),
	}
	err := store.Build(filepath.Join(root, "node_modules"), sources,
		[]store.Ref{ref("react-dom", "react_dom")})
	if err == nil {
		t.Fatal("a dangling dependency should fail the build, not produce a broken tree")
	}
	if !strings.Contains(err.Error(), "closure") {
		t.Errorf("the error should say what is missing and why; got: %v", err)
	}
}
