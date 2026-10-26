package store_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"tools/please_js/store"
)

func fileID(t *testing.T, path string) (ino, nlink uint64) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	return uint64(st.Ino), uint64(st.Nlink)
}

func sameDevice(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return fa.Sys().(*syscall.Stat_t).Dev == fb.Sys().(*syscall.Stat_t).Dev
}

// Two trees that stage one pin share the pin's inodes, still resolve, and a
// write in either tree cannot change the pin.
func TestTwoTreesSharePinInodes(t *testing.T) {
	root := t.TempDir()
	src := pkg(t, root, "react_18", "react", "18.3.1")
	if err := os.WriteFile(filepath.Join(src.Dir, "index.js"), []byte("module.exports = 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("index.js", filepath.Join(src.Dir, "self")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src.Dir, "escape")); err != nil {
		t.Fatal(err)
	}

	links := []store.Ref{ref("react", "react_18")}
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	if err := store.Build(a, []store.Source{src}, links, store.Store); err != nil {
		t.Fatal(err)
	}
	if err := store.Build(b, []store.Source{src}, links, store.Store); err != nil {
		t.Fatal(err)
	}

	placed := func(tree, name string) string {
		return filepath.Join(tree, store.StoreRoot, store.StoreDir("react_18"), "node_modules", "react", name)
	}
	for _, name := range []string{"package.json", "index.js"} {
		pin := filepath.Join(src.Dir, name)
		ia, na := fileID(t, placed(a, name))
		ib, _ := fileID(t, placed(b, name))
		ip, _ := fileID(t, pin)
		if ia != ib || ia != ip {
			t.Errorf("%s inodes differ: pin %d, a %d, b %d", name, ip, ia, ib)
		}
		if na < 3 {
			t.Errorf("%s should be linked from the pin and both trees, nlink %d", name, na)
		}
	}
	if !resolves(t, placed(a, "package.json")) || !resolves(t, filepath.Join(a, "react", "package.json")) {
		t.Error("the shared tree should still resolve react")
	}

	// A package symlink is preserved, not turned into a second hardlink of its
	// target, and one that points outside the package is not pulled in.
	for _, tree := range []string{a, b} {
		self := placed(tree, "self")
		fi, err := os.Lstat(self)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("self should stay a symlink, mode %v", fi.Mode())
		}
		if got, _ := os.Readlink(self); got != "index.js" {
			t.Errorf("self points at %q, want index.js", got)
		}
		escape := placed(tree, "escape")
		fi, err = os.Lstat(escape)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatal("a symlink out of the package was hardlinked instead of preserved")
		}
	}
	if _, n := fileID(t, outside); n != 1 {
		t.Errorf("the outside file gained a link, nlink %d", n)
	}

	index := placed(a, "index.js")
	fi, err := os.Lstat(index)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o222 != 0 {
		t.Errorf("shared file should not be writable, mode %v", fi.Mode())
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Error("the executable bit should survive")
	}
	before, err := os.ReadFile(filepath.Join(src.Dir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte("pwned"), 0o644); err == nil {
		t.Fatal("writing a staged file should fail; it would change the pin")
	}
	after, err := os.ReadFile(filepath.Join(src.Dir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("pin changed through the tree: %q", after)
	}
}

// A declarations twin overlays the package. The files it replaces must come
// from the twin, and neither write may land in the pin.
func TestTwinMergeDoesNotWriteThePin(t *testing.T) {
	root := t.TempDir()
	src := pkg(t, root, "react_18", "react", "18.3.1")
	if err := os.WriteFile(filepath.Join(src.Dir, "index.js"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	twinDir := filepath.Join(root, "twin")
	if err := os.MkdirAll(twinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	twinManifest := `{"name":"react","version":"18.3.1","types":"index.d.ts"}`
	if err := os.WriteFile(filepath.Join(twinDir, "package.json"), []byte(twinManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(twinDir, "index.d.ts"), []byte("export {};"), 0o644); err != nil {
		t.Fatal(err)
	}
	twin := store.Source{
		Dir:  twinDir,
		Meta: store.Meta{Package: "react", Version: "18.3.1", Name: "react_18_types", Role: "types"},
	}
	tree := filepath.Join(root, "node_modules")
	if err := store.Build(tree, []store.Source{src, twin}, []store.Ref{ref("react", "react_18")}, store.Store); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(tree, store.StoreRoot, store.StoreDir("react_18"), "node_modules", "react")
	got, err := os.ReadFile(filepath.Join(base, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != twinManifest {
		t.Fatalf("twin manifest should win, got %s", got)
	}
	pinManifest, err := os.ReadFile(filepath.Join(src.Dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pinManifest) == twinManifest {
		t.Fatal("merging the twin wrote the package pin")
	}
	bodyIno, _ := fileID(t, filepath.Join(base, "index.js"))
	pinIno, _ := fileID(t, filepath.Join(src.Dir, "index.js"))
	if bodyIno != pinIno {
		t.Error("the package body should still share the pin inode")
	}
	manIno, _ := fileID(t, filepath.Join(base, "package.json"))
	twinIno, _ := fileID(t, filepath.Join(twinDir, "package.json"))
	if manIno != twinIno {
		t.Error("the merged manifest should share the twin's inode")
	}
	if err := os.WriteFile(filepath.Join(base, "package.json"), []byte("pwned"), 0o644); err == nil {
		t.Fatal("writing the merged manifest should fail")
	}
	if data, _ := os.ReadFile(filepath.Join(twinDir, "package.json")); string(data) != twinManifest {
		t.Fatalf("twin pin changed: %s", data)
	}
}

func TestCrossDeviceLinkFallsBackToAPrivateCopy(t *testing.T) {
	pinRoot, err := os.MkdirTemp("/dev/shm", "js-rules-pin-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(pinRoot) })
	treeRoot := t.TempDir()
	if sameDevice(t, pinRoot, treeRoot) {
		t.Skip("pin and tree are on the same device")
	}

	src := pkg(t, pinRoot, "react_18", "react", "18.3.1")
	if err := os.WriteFile(filepath.Join(src.Dir, "index.js"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(treeRoot, "node_modules")
	if err := store.Build(tree, []store.Source{src}, []store.Ref{ref("react", "react_18")}, store.Store); err != nil {
		t.Fatal(err)
	}

	placed := filepath.Join(tree, store.StoreRoot, store.StoreDir("react_18"), "node_modules", "react", "index.js")
	if !resolves(t, filepath.Join(tree, "react", "package.json")) {
		t.Error("a copied tree should still resolve")
	}
	got, err := os.ReadFile(placed)
	if err != nil || string(got) != "body" {
		t.Fatalf("copy = %q, %v", got, err)
	}
	pi, _ := fileID(t, filepath.Join(src.Dir, "index.js"))
	ti, _ := fileID(t, placed)
	if pi == ti {
		t.Fatal("cross-device staging should not share the pin inode")
	}
	if err := os.WriteFile(placed, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	pin, err := os.ReadFile(filepath.Join(src.Dir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pin) != "body" {
		t.Fatalf("private copy wrote the pin: %q", pin)
	}
}
