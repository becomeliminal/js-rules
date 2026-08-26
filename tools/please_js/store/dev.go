package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DevLink is one first-party package a development server should serve from its
// sources rather than its built output.
//
// The information originates in the library's own rule -- the only place that
// knows its sources -- travels through lib.json, and is acted on at run time,
// because a symlink made in a sandboxed build action points at a temporary path
// that dies with the action.
type DevLink struct {
	Package  string   `json:"package"`
	SrcDir   string   `json:"srcDir"`
	SrcEntry string   `json:"srcEntry"`
	Srcs     []string `json:"srcs"`
}

// WriteDevLinks records what devlink will build, in the runtime directory where
// the launcher can find it.
func WriteDevLinks(path string, links []DevLink) error {
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Devlink materialises each package as its sources: a generated manifest whose
// entry is the source entry, and a symlink per source file into the repository.
//
// The package directory is rebuilt from nothing on every start, so a source
// removed from the library stops being served rather than lingering as a link
// nobody declared -- the same rule link_srcs follows, for the same reason.
func Devlink(tree, root, specPath string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", specPath, err)
	}
	var links []DevLink
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("parsing %s: %w", specPath, err)
	}

	for _, l := range links {
		dir := filepath.Join(tree, filepath.FromSlash(l.Package))
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		// The entry points at the source as written -- index.ts, not a
		// compiled index.js -- which is what makes a TypeScript library
		// hot-load with no compile step: the server transforms what it serves.
		if err := WritePackageJSON(
			filepath.Join(dir, "package.json"), l.Package, l.SrcEntry, "", nil); err != nil {
			return err
		}
		for _, src := range l.Srcs {
			at := filepath.Join(dir, filepath.FromSlash(src))
			if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
				return err
			}
			// Into the source tree, never out of it: the link lives in plz-out
			// and points at the file the developer edits.
			target := filepath.Join(root, filepath.FromSlash(l.SrcDir), filepath.FromSlash(src))
			if err := os.Symlink(target, at); err != nil {
				return fmt.Errorf("linking %s: %w", at, err)
			}
		}
	}
	return nil
}

// Packages lists the first-party package names found under dir, one lib.json
// each. A development server's config needs the names -- to exclude them from
// pre-bundling and to un-ignore them for the watcher -- and the names are
// knowable at build time even though the links are not buildable then.
func Packages(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if info.IsDir() || info.Name() != "lib.json" {
			return nil
		}
		meta, err := ReadMeta(path)
		if err != nil {
			return err
		}
		out = append(out, meta.Package)
		return nil
	})
	return out, err
}
