package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/store"
)

// storeTree builds a pnpm store-layout tree where a depends on b, plus a
// scoped @scope/other, and returns its root.
func storeTree(t *testing.T, layout store.Layout) string {
	t.Helper()
	dir := t.TempDir()
	pkg := func(name string) string {
		d := filepath.Join(dir, "src", strings.ReplaceAll(name, "/", "+"))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "index.js"), []byte("// "+name), 0o644)
		return d
	}
	src := func(entry, name string, deps ...store.Ref) store.Source {
		return store.Source{Dir: pkg(entry), Meta: store.Meta{Name: entry, Package: name}, Deps: deps}
	}
	sources := []store.Source{
		src("a_1", "a", store.Ref{As: "b", Entry: "b_1"}),
		src("b_1", "b"),
		src("scope_other_1", "@scope/other"),
	}
	root := filepath.Join(dir, "tree")
	links := []store.Ref{{As: "a", Entry: "a_1"}, {As: "@scope/other", Entry: "scope_other_1"}}
	if err := store.Build(root, sources, links, layout); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLinkTreeResolvesThroughTheStoreWithoutCopying(t *testing.T) {
	// GIVEN a store-layout tree, and a runtime node_modules holding a
	// first-party library in the same scope as a third-party package
	tree := storeTree(t, store.Store)
	dst := filepath.Join(t.TempDir(), "node_modules")
	os.MkdirAll(filepath.Join(dst, "@scope", "lib"), 0o755)
	os.WriteFile(filepath.Join(dst, "@scope", "lib", "index.js"), []byte("// lib"), 0o644)

	// WHEN the tree is linked in
	if err := store.LinkTree(tree, dst); err != nil {
		t.Fatal(err)
	}

	// THEN a is a link, and resolving its own dependency b from a's real path
	// finds b as a sibling in the store -- exactly how node resolves it
	fi, err := os.Lstat(filepath.Join(dst, "a"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a should be a link into the tree: %v", err)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(dst, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(real), "b", "index.js")); err != nil || string(data) != "// b_1" {
		t.Errorf("a's dependency b should resolve beside it in the store: %q %v", data, err)
	}
	// AND the scope is a real directory holding both the library and a link
	if fi, _ := os.Lstat(filepath.Join(dst, "@scope")); fi.Mode()&os.ModeSymlink != 0 {
		t.Error("@scope should be a real directory so a first-party package can sit in it")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "@scope", "lib", "index.js")); string(data) != "// lib" {
		t.Error("the first-party library should be untouched")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "@scope", "other", "index.js")); string(data) != "// scope_other_1" {
		t.Errorf("@scope/other should resolve through its link, got %q", data)
	}
	// AND nothing of the tree was copied: every top-level entry is a link
	entries, _ := os.ReadDir(dst)
	for _, e := range entries {
		if e.Name() == "@scope" {
			continue
		}
		if fi, _ := os.Lstat(filepath.Join(dst, e.Name())); fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s was copied, not linked", e.Name())
		}
	}
}

func TestLinkTreeCanRunAgainOnTheSameDirectory(t *testing.T) {
	// GIVEN a runtime the tree was already linked into, as at a previous start
	tree := storeTree(t, store.Store)
	dst := filepath.Join(t.TempDir(), "node_modules")
	if err := store.LinkTree(tree, dst); err != nil {
		t.Fatal(err)
	}

	// WHEN it is linked again
	err := store.LinkTree(tree, dst)

	// THEN the old links are replaced, not reported as collisions
	if err != nil {
		t.Errorf("relinking should succeed: %v", err)
	}
}

func TestLinkTreeRefusesALibraryThatTakesAThirdPartyName(t *testing.T) {
	// GIVEN a first-party library that would be imported as "a", which the
	// tree also provides
	tree := storeTree(t, store.Store)
	dst := filepath.Join(t.TempDir(), "node_modules")
	os.MkdirAll(filepath.Join(dst, "a"), 0o755)

	// WHEN the tree is linked in
	err := store.LinkTree(tree, dst)

	// THEN it fails, naming the collision and the fix
	if err == nil || !strings.Contains(err.Error(), "first-party library") || !strings.Contains(err.Error(), "`package`") {
		t.Errorf("expected a collision naming the fix, got %v", err)
	}
}

func TestLinkTreeCopiesAFlatTree(t *testing.T) {
	// GIVEN a tree in npm's flat layout, which resolves by walking up
	// directories named node_modules and so cannot be reached through links
	tree := storeTree(t, store.Hoisted)
	dst := filepath.Join(t.TempDir(), "node_modules")

	// WHEN it is linked in
	if err := store.LinkTree(tree, dst); err != nil {
		t.Fatal(err)
	}

	// THEN it is copied: real directories, and b still resolves from a
	if fi, _ := os.Lstat(filepath.Join(dst, "a")); fi == nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Error("a flat tree's entries should be copied, not linked")
	}
	if _, err := os.Stat(filepath.Join(dst, "b", "index.js")); err != nil {
		t.Errorf("b should be present at the top of the copied flat tree: %v", err)
	}
}

// A tree copied into a run directory whose node_modules still links packages
// into a build output must replace those links, never write through them: the
// output is shared, and rewriting it in place corrupts every later build.
func TestLinkTreeCopyNeverWritesThroughLinks(t *testing.T) {
	dir := t.TempDir()
	store_ := filepath.Join(dir, "store", "vite")
	writeFiles(t, store_, "package.json")
	dst := filepath.Join(dir, "run", "node_modules")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(store_, filepath.Join(dst, "vite")); err != nil {
		t.Fatal(err)
	}
	flat := filepath.Join(dir, "flat")
	writeFiles(t, flat, "vite/package.json")
	if err := os.WriteFile(filepath.Join(flat, "vite/package.json"), []byte("flat"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := store.LinkTree(flat, dst); err != nil {
		t.Fatal(err)
	}

	if data, _ := os.ReadFile(filepath.Join(store_, "package.json")); string(data) != "package.json" {
		t.Errorf("the store was written through the link: %q", data)
	}
	if fi, err := os.Lstat(filepath.Join(dst, "vite")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("the link should have been replaced by a real directory: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "vite/package.json")); string(data) != "flat" {
		t.Errorf("the copy should land in the run, got %q", data)
	}
}
