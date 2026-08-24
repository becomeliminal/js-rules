package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/store"
)

// pkg writes a fake package directory and returns a Source for it.
func pkg(t *testing.T, root, name, version, key string, deps map[string]string) store.Source {
	t.Helper()
	dir := filepath.Join(root, "src", store.StoreKey(key))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"` + name + `","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return store.Source{
		Dir:  dir,
		Meta: store.Meta{Package: name, Version: version, Key: key, Deps: deps},
	}
}

// resolves reports whether a path exists after following every symlink. Testing
// the link text alone would pass for a link that points somewhere plausible but
// wrong, which is exactly the failure this package exists to prevent.
func resolves(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func TestScopedAndUnscopedLinksResolve(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "react", "18.3.1", "react@18.3.1", nil),
		pkg(t, root, "@scope/thing", "1.0.0", "@scope/thing@1.0.0", nil),
	}
	links := []store.Link{
		{Alias: "react", Key: "react@18.3.1"},
		// The case that broke twice by hand: a scoped alias sits inside a scope
		// directory, so the store root is one level further up.
		{Alias: "@scope/thing", Key: "@scope/thing@1.0.0"},
	}

	if err := store.Build(tree, sources, links); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"react", "@scope/thing"} {
		if !resolves(t, filepath.Join(tree, alias, "package.json")) {
			target, _ := os.Readlink(filepath.Join(tree, alias))
			t.Errorf("%s does not resolve; link points at %q", alias, target)
		}
	}
}

func TestDepsResolveFromInsideTheirPackage(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "react-dom", "18.3.1", "react-dom@18.3.1_react@18.3.1", map[string]string{
			"react":        "react@18.3.1",
			"@scope/thing": "@scope/thing@1.0.0",
		}),
		pkg(t, root, "react", "18.3.1", "react@18.3.1", nil),
		pkg(t, root, "@scope/thing", "1.0.0", "@scope/thing@1.0.0", nil),
	}
	links := []store.Link{{Alias: "react-dom", Key: "react-dom@18.3.1_react@18.3.1"}}

	if err := store.Build(tree, sources, links); err != nil {
		t.Fatal(err)
	}

	// Node resolves a dependency by walking up to the nearest node_modules, so
	// the dep must be reachable as a sibling from inside the package.
	base := filepath.Join(tree, store.StoreRoot,
		store.StoreKey("react-dom@18.3.1_react@18.3.1"), "node_modules")
	for _, dep := range []string{"react", "@scope/thing"} {
		if !resolves(t, filepath.Join(base, dep, "package.json")) {
			target, _ := os.Readlink(filepath.Join(base, dep))
			t.Errorf("react-dom cannot resolve %s; link points at %q", dep, target)
		}
	}
}

func TestOnePackageUnderTwoNamesAtTwoVersions(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "@aspect-test/c", "2.0.2", "@aspect-test/c@2.0.2", nil),
		pkg(t, root, "@aspect-test/c", "1.0.0", "@aspect-test/c@1.0.0", nil),
	}
	links := []store.Link{
		{Alias: "@aspect-test/c", Key: "@aspect-test/c@2.0.2"},
		{Alias: "@aspect-test/c1", Key: "@aspect-test/c@1.0.0"},
	}

	if err := store.Build(tree, sources, links); err != nil {
		t.Fatal(err)
	}

	// The alias is not the package name, and both versions must survive.
	for alias, want := range map[string]string{
		"@aspect-test/c":  "2.0.2",
		"@aspect-test/c1": "1.0.0",
	} {
		data, err := os.ReadFile(filepath.Join(tree, alias, "package.json"))
		if err != nil {
			t.Errorf("%s does not resolve: %v", alias, err)
			continue
		}
		if want := `"version":"` + want + `"`; !strings.Contains(string(data), want) {
			t.Errorf("%s resolved to the wrong version: %s", alias, data)
		}
	}
}

func TestMissingClosureMemberIsAnError(t *testing.T) {
	root := t.TempDir()
	sources := []store.Source{
		pkg(t, root, "react-dom", "18.3.1", "react-dom@18.3.1", map[string]string{
			"react": "react@18.3.1",
		}),
	}
	err := store.Build(filepath.Join(root, "node_modules"), sources,
		[]store.Link{{Alias: "react-dom", Key: "react-dom@18.3.1"}})
	if err == nil {
		t.Fatal("a dangling dependency should fail the build, not produce a broken tree")
	}
	if !strings.Contains(err.Error(), "closure") {
		t.Errorf("the error should say what is missing and why; got: %v", err)
	}
}
