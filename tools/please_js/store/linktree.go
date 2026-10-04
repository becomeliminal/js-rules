package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LinkTree makes a third-party tree resolvable from dst, a node_modules that
// may already hold first-party libraries, without copying it.
//
// A program's runtime used to carry its own copy of the whole tree: tens of
// thousands of files per test or dev server, re-uploaded under remote
// execution for every program in every app. The tree already exists as its
// own build output, so the launcher links it in at start instead.
//
// In pnpm's store layout every top-level entry is a symlink into the tree's
// .plz store, and node resolves a package's own dependencies from its real
// path there, as siblings inside its store entry. So one absolute link per
// top-level name -- pointing at the tree's entry -- resolves exactly as the
// tree itself does. Scope directories are made real, so a first-party
// @scope/lib can sit beside third-party @scope packages.
//
// npm's flat layout cannot be linked this way: it resolves by walking up
// directories named node_modules, and the tree's output directory is not one.
// It is copied instead, as before.
//
// Links from an earlier start are replaced, so the result reflects the tree
// as it is now, and several starts may link the same directory at once. A name held by both a first-party library and the tree is an
// error, as it is when libraries are overlaid at build time.
func LinkTree(tree, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if tree == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(tree, ".plz")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return copyTree(tree, dst)
	}
	abs, err := filepath.Abs(tree)
	if err != nil {
		return err
	}
	return linkDir(abs, dst, true)
}

// linkDir links every entry of src into dst. At the top level, a scope
// directory is descended into rather than linked whole.
func linkDir(src, dst string, top bool) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		from := filepath.Join(src, name)
		to := filepath.Join(dst, name)

		if top && strings.HasPrefix(name, "@") && e.IsDir() {
			if fi, err := os.Lstat(to); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				if err := os.Remove(to); err != nil {
					return err
				}
			}
			if err := os.MkdirAll(to, 0o755); err != nil {
				return err
			}
			if err := linkDir(from, to, false); err != nil {
				return err
			}
			continue
		}

		fi, err := os.Lstat(to)
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			// A link from an earlier start, or from another start of the same
			// program running now. One that already points here is left alone.
			if target, err := os.Readlink(to); err == nil && target == from {
				continue
			}
		case err == nil:
			rel := strings.TrimPrefix(to, filepath.Dir(filepath.Dir(to))+string(filepath.Separator))
			return fmt.Errorf("%s is a first-party library and also a third-party package in the tree; "+
				"set `package` on the library to import it under another name", rel)
		case !os.IsNotExist(err):
			return err
		}
		// Made under a name of this process's own and renamed into place: the
		// rename replaces an older link in one step, so programs started
		// together never find the name missing or taken.
		tmp := fmt.Sprintf("%s.%d.tmp", to, os.Getpid())
		if err := os.Symlink(from, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, to); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return nil
}
