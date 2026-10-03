package store

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LinkSet is one directory of a run that a development program reads straight
// from the repository: the program's own sources, or a first-party package
// served from its sources rather than its built output.
//
// Every link lives in plz-out and points into the source tree, never the other
// way round, and every link is to a file: a directory symlinked into the run
// would route each later link to a child THROUGH it, writing into the source
// tree. Directories in the run are real ones, made to hold the links.
//
// The information is gathered at build time -- a library's sources travel
// through its lib.json, the program's through link_srcs -- and acted on at run
// time, because a symlink made in a sandboxed build action points at a
// temporary path that dies with the action.
type LinkSet struct {
	// Into is where the links land, relative to the run directory.
	Into string `json:"into"`
	// From is the Please package the sources belong to, relative to the
	// repository root. Srcs and Live are relative to it.
	From string `json:"from"`
	// Srcs are the files the rule declared.
	Srcs []string `json:"srcs"`
	// Live are the directories under From whose contents are linked as they
	// are, not as they were declared: a file created after the build, in a
	// directory that holds declared sources, is linked too, at start and --
	// by a development server's watcher -- while it runs. See LiveDirs.
	Live []string `json:"live,omitempty"`
	// Package and SrcEntry are set for a first-party library, whose directory
	// also needs a manifest naming the source entry.
	Package  string `json:"package,omitempty"`
	SrcEntry string `json:"srcEntry,omitempty"`
}

// LinkSpec is everything a run links from the repository.
type LinkSpec struct {
	Sets []LinkSet `json:"sets"`
	// Skip is carried in the spec rather than restated by each reader, so the
	// links made at start and the links a watcher makes later follow one rule.
	Skip SkipRules `json:"skip"`
}

// SkipRules name what in a live directory is never linked, by path segment.
type SkipRules struct {
	Dirs     []string `json:"dirs"`
	Prefixes []string `json:"prefixes"`
	Suffixes []string `json:"suffixes"`
}

// DefaultSkip leaves out what a source directory holds that is not source:
// installed trees, dotfiles (.DS_Store, a stray .git), and editors' temporary
// and backup files, which appear and vanish on every save.
var DefaultSkip = SkipRules{
	Dirs:     []string{"node_modules"},
	Prefixes: []string{".", "#"},
	Suffixes: []string{"~", ".swp", ".swx"},
}

// Skips reports whether a path, relative to a live directory's package, is
// left out.
func (s SkipRules) Skips(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		for _, d := range s.Dirs {
			if seg == d {
				return true
			}
		}
		for _, p := range s.Prefixes {
			if strings.HasPrefix(seg, p) {
				return true
			}
		}
		for _, x := range s.Suffixes {
			if strings.HasSuffix(seg, x) {
				return true
			}
		}
	}
	return false
}

// LiveDirs is the one rule for what is linked live: each top-level directory
// of the package that holds a declared source -- src/, public/ -- and
// everything under it, new subdirectories included. Files at the package's own
// top level are linked only as declared, because that is where a package keeps
// what is not source: its BUILD file, package.json, a dist/ or an installed
// node_modules.
func LiveDirs(srcs []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, s := range srcs {
		top, _, nested := strings.Cut(filepath.ToSlash(s), "/")
		if !nested || seen[top] {
			continue
		}
		seen[top] = true
		dirs = append(dirs, top)
	}
	sort.Strings(dirs)
	return dirs
}

// WriteLinks records what Link will build, in the run directory where the
// launcher can find it.
func WriteLinks(path string, spec LinkSpec) error {
	if spec.Sets == nil {
		spec.Sets = []LinkSet{}
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Link materialises every set in the run directory: the declared sources, and
// whatever the live directories hold now.
//
// A library's directory is rebuilt from nothing, so a source removed from it
// stops being served. The program's own directory cannot be -- it holds the
// copied config and the linked third-party tree -- so instead every link in it
// that points into the package is removed first and the current set made
// again. Either way a file deleted since the last start is not served.
func Link(rundir, root, specPath string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", specPath, err)
	}
	var spec LinkSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("parsing %s: %w", specPath, err)
	}

	for _, set := range spec.Sets {
		into := filepath.Join(rundir, filepath.FromSlash(set.Into))
		from := filepath.Join(root, filepath.FromSlash(set.From))

		if set.Package != "" {
			if err := os.RemoveAll(into); err != nil {
				return err
			}
			if err := os.MkdirAll(into, 0o755); err != nil {
				return err
			}
			// The entry points at the source as written -- index.ts, not a
			// compiled index.js -- which is what makes a TypeScript library
			// hot-load with no compile step: the server transforms what it
			// serves.
			if err := WritePackageJSON(
				filepath.Join(into, "package.json"), set.Package, set.SrcEntry, "", nil); err != nil {
				return err
			}
		} else if err := unlinkInto(into, from); err != nil {
			return err
		}

		files := map[string]bool{}
		for _, s := range set.Srcs {
			files[filepath.FromSlash(s)] = true
		}
		for _, dir := range set.Live {
			err := filepath.WalkDir(filepath.Join(from, filepath.FromSlash(dir)), func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(from, p)
				if err != nil {
					return err
				}
				if spec.Skip.Skips(rel) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if !d.IsDir() {
					files[rel] = true
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}

		rels := make([]string, 0, len(files))
		for rel := range files {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		for _, rel := range rels {
			if err := linkOne(filepath.Join(from, rel), filepath.Join(into, rel)); err != nil {
				return err
			}
		}
	}
	return nil
}

// linkOne links one source file into the run. A declared source that is a
// directory -- Please's glob returns them alongside their contents -- or that
// no longer exists is passed over: the directories are made as the files in
// them are linked, and a file deleted since the build has nothing to serve. A
// real file already at the link's place -- the copied config, a generated
// manifest -- is the build's, and is left alone.
func linkOne(target, at string) error {
	info, err := os.Stat(target)
	if os.IsNotExist(err) || (err == nil && info.IsDir()) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing, err := os.Lstat(at); err == nil {
		if existing.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		if err := os.Remove(at); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(target, at); err != nil {
		return fmt.Errorf("linking %s: %w", at, err)
	}
	return nil
}

// unlinkInto removes every symlink under dir that points into from. Only
// those: the third-party tree is linked into the same run directory and points
// into plz-out, and node_modules is not walked at all.
func unlinkInto(dir, from string) error {
	prefix := from + string(filepath.Separator)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(target, prefix) {
			return os.Remove(p)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
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
