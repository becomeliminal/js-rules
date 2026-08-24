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
	"strings"
)

// StoreRoot is the directory inside node_modules holding one copy of every
// resolved package. The leading dot keeps tools that scan node_modules for
// packages from mistaking it for one.
const StoreRoot = ".plz"

// Meta describes one fetched package. npm_repo writes it beside the package;
// npm_link reads every one it is given and assembles the tree from them.
//
// Deps is a list of names, not a map of import-name to name, for the same
// reason go_library takes a list: the name a dependency is imported under is
// read from that dependency's own description. It is also not a list of Please
// labels. npm permits dependency cycles -- @babel/core and
// @babel/helper-module-transforms require each other, as do browserslist and
// update-browserslist-db -- and Please rejects a cyclic build graph outright.
// Go and Rust never hit this because both languages forbid import cycles.
// Since npm_link is handed the whole closure anyway, edges between packages
// would buy nothing, so a dependency is named here and resolved at link time.
type Meta struct {
	Package string   `json:"package"`
	Version string   `json:"version"`
	Name    string   `json:"name"`              // this entry's identity: its target name
	AliasOf string   `json:"aliasOf,omitempty"` // set when this entry rebinds another
	Deps    []string `json:"deps,omitempty"`    // names of entries in this package's node_modules
	Bins    map[string]string `json:"bins,omitempty"`
}

// StoreDir is the directory an entry occupies within the store.
//
// The entry's identity is its Please target name, which is already unique per
// resolution: the generator folds the peer resolution into it, and Please
// enforces uniqueness within a package. pnpm has to invent a key here because
// it has no target names; we do, so the name is the key.
func StoreDir(name string) string { return strings.ReplaceAll(name, "/", "+") }

// Source is one staged package directory together with its description.
type Source struct {
	Dir  string // directory holding the package's files; empty for an alias
	Meta Meta
}

// Build assembles a node_modules tree at root.
//
// links names the entries to expose at the top level. An entry is exposed under
// its own package name, so rebinding a package under another name is done by
// linking an alias entry rather than by naming it here -- which is why this
// takes a list and not a map.
func Build(root string, sources []Source, links []string) error {
	byName := make(map[string]Source, len(sources))
	for _, s := range sources {
		if _, dup := byName[s.Meta.Name]; dup {
			return fmt.Errorf("two store entries are both named %q", s.Meta.Name)
		}
		byName[s.Meta.Name] = s
	}

	// An alias holds no files of its own; it points at the entry it rebinds, so
	// the package is still present exactly once. Following the chain rather
	// than copying is what keeps a package a singleton, which matters because
	// two copies of something like React are two module instances.
	resolve := func(name string) (Source, error) {
		seen := map[string]bool{}
		for {
			s, ok := byName[name]
			if !ok {
				return Source{}, fmt.Errorf(
					"%q is not in the closure -- npm_link needs every entry reachable from "+
						"the packages it links, not only the direct ones", name)
			}
			if s.Meta.AliasOf == "" {
				return s, nil
			}
			if seen[name] {
				return Source{}, fmt.Errorf("alias cycle through %q", name)
			}
			seen[name] = true
			name = s.Meta.AliasOf
		}
	}

	// Every package's files, once, at a path keyed by its identity.
	for _, s := range sources {
		if s.Meta.AliasOf != "" {
			continue
		}
		dst := filepath.Join(root, StoreRoot, StoreDir(s.Meta.Name), "node_modules", s.Meta.Package)
		if err := copyTree(s.Dir, dst); err != nil {
			return fmt.Errorf("staging %s: %w", s.Meta.Name, err)
		}
	}

	// A package's own dependencies, as siblings of it inside its store entry.
	// Node resolves by walking up from a file to the nearest node_modules, so
	// this is what makes a dependency reachable from inside the package without
	// any resolver hook. The name a dependency appears under comes from that
	// dependency's own description, never from the dependent.
	for _, s := range sources {
		if s.Meta.AliasOf != "" {
			continue
		}
		for _, depName := range s.Meta.Deps {
			dep, err := resolve(depName)
			if err != nil {
				return fmt.Errorf("%s depends on %w", s.Meta.Name, err)
			}
			alias := byName[depName].Meta.Package
			from := filepath.Join(root, StoreRoot, StoreDir(s.Meta.Name), "node_modules", alias)
			to := filepath.Join(root, StoreRoot, StoreDir(dep.Meta.Name), "node_modules", dep.Meta.Package)
			if err := symlink(from, to); err != nil {
				return fmt.Errorf("linking %s into %s: %w", alias, s.Meta.Name, err)
			}
		}
	}

	// The top level: what this project can import by name. Two entries wanting
	// the same name is a real condition -- two versions of one package, only
	// one of which can hold the plain name -- and it has to be refused rather
	// than resolved by whichever happens to be linked last.
	taken := map[string]string{}
	for _, name := range links {
		entry, ok := byName[name]
		if !ok {
			return fmt.Errorf("%q is linked at the top level but is not in the closure", name)
		}
		target, err := resolve(name)
		if err != nil {
			return err
		}
		if prev, dup := taken[entry.Meta.Package]; dup {
			return fmt.Errorf(
				"%s and %s would both be imported as %q; link an npm_alias to give one of them another name",
				prev, name, entry.Meta.Package)
		}
		taken[entry.Meta.Package] = name
		from := filepath.Join(root, entry.Meta.Package)
		to := filepath.Join(root, StoreRoot, StoreDir(target.Meta.Name), "node_modules", target.Meta.Package)
		if err := symlink(from, to); err != nil {
			return fmt.Errorf("linking %s: %w", entry.Meta.Package, err)
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
