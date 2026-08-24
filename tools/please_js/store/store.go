// Package store builds the pnpm-style symlinked node_modules tree.
//
// Assembling the tree in Go rather than in a rule's shell command is not a
// stylistic choice. Every path in the tree is relative and several are computed
// from how deeply an alias nests -- a scoped package sits a directory further
// down than an unscoped one -- and getting that arithmetic wrong produces a
// link that resolves somewhere plausible and surfaces much later as a missing
// module. Here it is one function with tests.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StoreRoot is the directory inside node_modules holding one copy of every
// resolved package. The leading dot keeps tools that scan node_modules for
// packages from mistaking it for one.
const StoreRoot = ".plz"

// Meta describes one fetched package. npm_repo writes it beside the package;
// npm_link reads every one it is given and assembles the tree from them.
type Meta struct {
	Package string            `json:"package"`
	Version string            `json:"version"`
	Key     string            `json:"key"`
	Deps    map[string]string `json:"deps,omitempty"` // import name -> dep's store key
	Bins    map[string]string `json:"bins,omitempty"` // bin name -> path within the package
}

// StoreKey flattens a key into a single directory name.
//
// A scoped package's key carries a path separator, which would otherwise nest
// the entry a level deeper than every path referencing it. pnpm and
// aspect_rules_js both map the separator to '+' for the same reason.
func StoreKey(key string) string { return strings.ReplaceAll(key, "/", "+") }

// Source is one staged package directory together with its metadata.
type Source struct {
	Dir  string // directory holding the package's files
	Meta Meta
}

// Link is a package to expose at the top level of node_modules, under the name
// source code imports it by. That name is not always the package name: a
// lockfile can bind one package under several aliases, at different versions.
type Link struct {
	Alias string
	Key   string
}

// Build assembles a node_modules tree at root.
func Build(root string, sources []Source, links []Link) error {
	byKey := make(map[string]Source, len(sources))
	for _, s := range sources {
		byKey[s.Meta.Key] = s
	}

	// Every package's files, once, at a path keyed by its resolution.
	for _, s := range sources {
		dst := filepath.Join(root, StoreRoot, StoreKey(s.Meta.Key), "node_modules", s.Meta.Package)
		if err := copyTree(s.Dir, dst); err != nil {
			return fmt.Errorf("staging %s: %w", s.Meta.Key, err)
		}
	}

	// A package's own dependencies, as siblings of it inside its store entry.
	// Node's resolution walks up from a file to the nearest node_modules, so
	// this is what makes a dependency resolvable from inside the package
	// without any resolver hook.
	for _, s := range sources {
		for _, alias := range sortedKeys(s.Meta.Deps) {
			depKey := s.Meta.Deps[alias]
			dep, ok := byKey[depKey]
			if !ok {
				return fmt.Errorf(
					"%s depends on %s, which is not in the closure -- npm_link needs every "+
						"store entry reachable from the packages it links, not just the direct ones",
					s.Meta.Key, depKey)
			}
			from := filepath.Join(root, StoreRoot, StoreKey(s.Meta.Key), "node_modules", alias)
			to := filepath.Join(root, StoreRoot, StoreKey(depKey), "node_modules", dep.Meta.Package)
			if err := symlink(from, to); err != nil {
				return fmt.Errorf("linking %s -> %s: %w", from, to, err)
			}
		}
	}

	// The top level: what this project can import by name.
	for _, l := range links {
		src, ok := byKey[l.Key]
		if !ok {
			return fmt.Errorf("link %q refers to %s, which is not in the closure", l.Alias, l.Key)
		}
		from := filepath.Join(root, l.Alias)
		to := filepath.Join(root, StoreRoot, StoreKey(l.Key), "node_modules", src.Meta.Package)
		if err := symlink(from, to); err != nil {
			return fmt.Errorf("linking %s: %w", l.Alias, err)
		}
	}
	return nil
}

// symlink points from at to, relative to from's own directory so the finished
// tree can be moved, staged into a sandbox or shipped to a remote worker.
func symlink(from, to string) error {
	dir := filepath.Dir(from)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, to)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(from); err != nil {
		return err
	}
	return os.Symlink(rel, from)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			// A package may ship symlinks of its own; preserve them verbatim
			// rather than following them out of the tree.
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
	})
}

// ReadMeta loads the description npm_repo wrote beside a package.
func ReadMeta(path string) (Meta, error) {
	var m Meta
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// WriteMeta records a package's description.
func WriteMeta(path string, m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ReadBins returns a package's executables, from the "bin" field of its own
// package.json.
//
// npm allows two shapes: a map of names to paths, or a bare string meaning one
// executable named after the package. A scoped package's executable takes the
// name after the scope, so @foo/bar installs as "bar".
func ReadBins(dir, pkgName string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // not every published artefact carries a manifest
		}
		return nil, err
	}

	var manifest struct {
		Bin json.RawMessage `json:"bin"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing %s/package.json: %w", dir, err)
	}
	if len(manifest.Bin) == 0 {
		return nil, nil
	}

	var asMap map[string]string
	if err := json.Unmarshal(manifest.Bin, &asMap); err == nil {
		return asMap, nil
	}

	var asString string
	if err := json.Unmarshal(manifest.Bin, &asString); err == nil {
		name := pkgName
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		return map[string]string{name: asString}, nil
	}
	return nil, fmt.Errorf("%s: \"bin\" is neither a string nor an object", dir)
}
