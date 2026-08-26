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
	"bytes"
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
	Package string            `json:"package"`
	Version string            `json:"version"`
	Name    string            `json:"name"` // this entry's identity: its target name
	Bins    map[string]string `json:"bins,omitempty"`
	// Unsupported marks a package whose os/cpu constraints exclude this
	// platform. The target still exists so the build file stays the same
	// everywhere, but nothing was fetched and nothing is placed.
	Unsupported bool `json:"unsupported,omitempty"`
}

// Ref is one entry appearing in a node_modules directory, under the name source
// code imports it by.
//
// That name is usually the package's own, but npm lets a manifest bind a
// package under another ("my-react": "npm:react@18"), so it is carried
// alongside rather than derived. A list of these, not a map: the order is the
// lockfile's and nothing here needs to look one up by name.
type Ref struct {
	As    string // the name it appears under
	Entry string // the entry it resolves to
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
	Dir  string // directory holding the package's files
	Meta Meta
	Deps []Ref // what belongs in this package's own node_modules
	// Origin says where this came from, for diagnostics only. A first-party
	// package and a fetched one are indistinguishable once placed -- which is
	// the design -- so the only time the difference matters is when two of them
	// collide and someone has to be told which is which.
	Origin string
}

// Layout decides where a package's files sit, and the difference is not a
// preference: it is which resolution algorithm the tree is for.
//
// Store is pnpm's shape. Each resolution owns one directory under .plz, its
// dependencies are symlinks beside it, and reaching them means following those
// links. Nothing is ever duplicated, and a tool must resolve symlinks for the
// tree to make sense.
//
// Hoisted is npm's. A package sits at the top level unless its name is already
// taken, in which case it sits beside the dependent that needs it, and
// resolution is a plain walk up the directory chain with no symlink anywhere.
//
// The second exists because some tools cannot follow symlinks without also
// resolving away paths that must stay as written -- a development server
// serving a developer's real files is the case that forces it. Hoisting costs
// a copy only where two resolutions of one name are both reachable: measured
// against a real 1,283-package lockfile, 67 extra placements of 1,279, or 5%.
type Layout int

const (
	Store Layout = iota
	Hoisted
)

// place decides where every reachable resolution goes in a hoisted tree.
//
// Top level when the name is free, beside the dependent when it is not. npm
// hoists as high as it can rather than only to the top, which nests less; this
// nests a little more and is correct for the same reason -- a package's own
// node_modules is searched before any ancestor's, so a name placed lower is
// still the one its dependent finds.
func place(sources []Source, links []Ref) (map[string][]string, error) {
	byName := make(map[string]Source, len(sources))
	for _, s := range sources {
		byName[s.Meta.Name] = s
	}

	// A path here is relative to the tree root, which is itself a node_modules,
	// so the top level is just the import name.
	taken := map[string]string{}
	placements := map[string][]string{}

	type item struct{ entry, as, parent string }
	queue := make([]item, 0, len(links))
	for _, ref := range links {
		queue = append(queue, item{entry: ref.Entry, as: ref.As})
	}

	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]

		src, ok := byName[it.entry]
		if !ok {
			return nil, fmt.Errorf(
				"%q is not in the closure -- npm_link needs every entry reachable from the "+
					"packages it links, not only the direct ones", it.entry)
		}
		if src.Meta.Unsupported {
			continue
		}

		at := it.as
		if held, dup := taken[at]; dup && held != it.entry {
			// The name is spoken for by a different resolution, so this one
			// lives beside the package that asked for it.
			at = it.parent + "/node_modules/" + it.as
		}
		if held, done := taken[at]; done {
			if held == it.entry {
				continue // already placed, and its dependencies already queued
			}
			return nil, fmt.Errorf("%s and %s would both be at %s", held, it.entry, at)
		}
		taken[at] = it.entry
		placements[it.entry] = append(placements[it.entry], at)

		for _, dep := range src.Deps {
			queue = append(queue, item{entry: dep.Entry, as: dep.As, parent: at})
		}
	}
	return placements, nil
}

// Build assembles a node_modules tree at root.
//
// links names the entries to expose at the top level. An entry is exposed under
// its own package name, so rebinding a package under another name is done by
// linking an alias entry rather than by naming it here -- which is why this
// takes a list and not a map.
func Build(root string, sources []Source, links []Ref, layout Layout) error {
	if layout == Hoisted {
		return buildHoisted(root, sources, links)
	}

	byName := make(map[string]Source, len(sources))
	for _, s := range sources {
		if prev, dup := byName[s.Meta.Name]; dup {
			// Deliberate and visible, never order-dependent. A first-party
			// package shadowing a registry one is a reasonable thing to want
			// and a terrible thing to get by accident, so it is an error that
			// names both rather than a silent last-one-wins.
			return fmt.Errorf("two packages are both %q: %s and %s. "+
				"If one is meant to replace the other, remove the one it replaces",
				s.Meta.Name, describeOrigin(prev), describeOrigin(s))
		}
		byName[s.Meta.Name] = s
	}

	find := func(entry string) (Source, error) {
		s, ok := byName[entry]
		if !ok {
			return Source{}, fmt.Errorf(
				"%q is not in the closure -- npm_link needs every entry reachable from the "+
					"packages it links, not only the direct ones", entry)
		}
		return s, nil
	}

	// Every package's files, once, at a path keyed by its identity.
	for _, s := range sources {
		if s.Meta.Unsupported {
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
	// any resolver hook.
	for _, s := range sources {
		if s.Meta.Unsupported {
			continue
		}
		for _, ref := range s.Deps {
			dep, err := find(ref.Entry)
			if err != nil {
				return fmt.Errorf("%s depends on %w", s.Meta.Name, err)
			}
			// A dependency excluded by its own constraints is simply absent.
			// npm records these as optional precisely so a package copes.
			if dep.Meta.Unsupported {
				continue
			}
			from := filepath.Join(root, StoreRoot, StoreDir(s.Meta.Name), "node_modules", ref.As)
			to := filepath.Join(root, StoreRoot, StoreDir(dep.Meta.Name), "node_modules", dep.Meta.Package)
			if err := symlink(from, to); err != nil {
				return fmt.Errorf("linking %s into %s: %w", ref.As, s.Meta.Name, err)
			}
		}
	}

	// The top level: what this project can import by name. Two entries wanting
	// the same name is a real condition -- two versions of one package, only one
	// of which can hold the plain name -- and it is refused rather than settled
	// by whichever is linked last.
	taken := map[string]string{}
	for _, ref := range links {
		entry, err := find(ref.Entry)
		if err != nil {
			return err
		}
		if entry.Meta.Unsupported {
			continue
		}
		if prev, dup := taken[ref.As]; dup {
			return fmt.Errorf("%s and %s would both be imported as %q", prev, ref.Entry, ref.As)
		}
		taken[ref.As] = ref.Entry
		from := filepath.Join(root, ref.As)
		to := filepath.Join(root, StoreRoot, StoreDir(entry.Meta.Name), "node_modules", entry.Meta.Package)
		if err := symlink(from, to); err != nil {
			return fmt.Errorf("linking %s: %w", ref.As, err)
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

// CopyTree copies a directory, preserving symlinks rather than following them
// out of the tree.
func CopyTree(src, dst string) error { return copyTree(src, dst) }

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

// Overlay copies first-party libraries into an existing tree.
//
// A first-party library is named by where it lives, the way a Go import path
// mirrors its directory, so //common/js/components is imported as
// "common/js/components". Node resolves a multi-segment name by path, so
// placing it at node_modules/common/js/components makes the import work with no
// aliasing, no tsconfig paths and no workspace protocol.
//
// These are copied rather than linked into the store: a first-party library has
// no resolution to disambiguate, so it needs no entry of its own.
func Overlay(root string, libs []Source) error {
	taken := map[string]string{}
	for _, lib := range libs {
		if prev, dup := taken[lib.Meta.Package]; dup {
			return fmt.Errorf(
				"%s and %s are both imported as %q; set `package` on one of them",
				prev, lib.Meta.Name, lib.Meta.Package)
		}
		taken[lib.Meta.Package] = lib.Meta.Name

		dst := filepath.Join(root, lib.Meta.Package)
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf(
				"%s is imported as %q, which a third-party package already occupies",
				lib.Meta.Name, lib.Meta.Package)
		}
		if err := copyTree(lib.Dir, dst); err != nil {
			return fmt.Errorf("overlaying %s: %w", lib.Meta.Name, err)
		}
	}
	return nil
}

// WritePackageJSON emits the manifest node needs to resolve a directory as a
// package.
//
// It is generated, never authored. Node finds index.js without one, but not
// "types" or "exports" -- so the build system writes the resolution metadata,
// exactly as please_go writes an importconfig rather than asking for one.
// ordered is a JSON object that keeps the order it was written in.
//
// Almost nothing in a manifest cares, but conditional exports do: node walks
// the conditions in file order and takes the first it recognises, so "default"
// must be last or it shadows every condition after it. encoding/json sorts a
// map's keys, and "default" sorts before "types".
type ordered []struct {
	K string
	V any
}

func (o ordered) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(kv.K)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(kv.V)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func WritePackageJSON(path, name, main, types string, extra map[string]any) error {
	// map[string]any rather than map[string]string: "type": "module" is a
	// string, but "sideEffects" and "exports" are not, and a manifest that
	// cannot express them would have to be hand-written instead of generated.
	manifest := map[string]any{"name": name, "version": "0.0.0"}
	if main != "" {
		manifest["main"] = main
	}
	if types != "" {
		manifest["types"] = types
	}

	// CommonJS honours "main"; ESM does not. Node's ESM resolver refuses a
	// directory import outright -- ERR_UNSUPPORTED_DIR_IMPORT -- so a library
	// without "exports" cannot be imported by name at all once it emits
	// modules. The conditional form serves both, and the "./*" entry is there
	// because "exports" is an allowlist: declaring it would otherwise make
	// every subpath import of this package stop resolving.
	if main != "" {
		// Ordered, not a map: conditions are matched in the order they appear in
		// the file, and "default" is the fallback, so it has to come last. A Go
		// map cannot express that -- encoding/json sorts keys, which puts
		// "default" first and makes "types" unreachable.
		root := ordered{{"default", "./" + main}}
		if types != "" {
			root = ordered{{"types", "./" + types}, {"default", "./" + main}}
		}
		manifest["exports"] = ordered{{".", root}, {"./*", "./*"}}
	}
	for k, v := range extra {
		manifest[k] = v
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ResolveBin returns the path to an executable a package publishes, relative to
// the tree root.
//
// The executable is named as the package's own package.json declares it, and
// looked up here rather than given as a path, so a typo fails the build with a
// message listing what the package does publish.
func ResolveBin(tree, pkg, bin string) (string, error) {
	dir := filepath.Join(tree, pkg)
	// A package that is absent from the tree and one that publishes nothing look
	// identical to ReadBins, which treats a missing manifest as "no executables".
	// They need different messages: the second is a mistake about the package,
	// the first is almost always a mistake about which tree was passed.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", fmt.Errorf("no package %s in %s; is that the tree publishing it?", pkg, tree)
	}
	bins, err := ReadBins(dir, pkg)
	if err != nil {
		return "", err
	}
	if len(bins) == 0 {
		return "", fmt.Errorf("%s publishes no executables", pkg)
	}
	if bin == "" {
		if len(bins) == 1 {
			for _, path := range bins {
				return filepath.Join(dir, path), nil
			}
		}
		return "", fmt.Errorf("%s publishes %s; say which with `bin`", pkg, names(bins))
	}
	path, ok := bins[bin]
	if !ok {
		return "", fmt.Errorf("%s publishes no executable %q; it publishes %s", pkg, bin, names(bins))
	}
	return filepath.Join(dir, path), nil
}

func names(m map[string]string) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// Publish rewrites a built package's manifest for release.
//
// It patches rather than regenerates. A library's package.json already carries
// the name, entry point, types and exports map that make it importable, and
// those were worked out once and are easy to get wrong -- the exports map
// especially, since a missing one makes the package unimportable under ESM and
// a mis-ordered one makes its types unreachable. Publishing changes what a
// registry needs and leaves the rest alone.
func Publish(dir, version string, fields map[string]any) error {
	path := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// Decoded into an ordered form, so the exports map's condition order
	// survives: node matches conditions in file order and treats "default" as
	// the fallback, so re-encoding through a map would move it and make
	// everything after it unreachable.
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	set := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		manifest[key] = raw
		return nil
	}
	if err := set("version", version); err != nil {
		return err
	}
	for k, v := range fields {
		if err := set(k, v); err != nil {
			return err
		}
	}

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func describeOrigin(s Source) string {
	if s.Origin != "" {
		return s.Origin
	}
	return s.Dir
}

// DeclareBins adds executables to a package's manifest.
//
// Some packages ship a runnable file and never declare it, usually because
// their own install script would have created the link. Nothing here runs
// install scripts by default, and a package that publishes no executables
// cannot be run at all -- so the declaration has to come from somewhere, and
// the honest place is the package's own manifest, patched.
//
// npm allows "bin" to be a bare string, meaning one executable named after the
// package. Merging into that form has to widen it to an object first, or the
// package ends up with one of its two executables silently discarded.
func DeclareBins(dir string, bins map[string]string) error {
	if len(bins) == 0 {
		return nil
	}
	path := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	existing := map[string]string{}
	if raw, ok := manifest["bin"]; ok {
		if err := json.Unmarshal(raw, &existing); err != nil {
			// The bare-string form: one executable, named after the package.
			var single string
			if err := json.Unmarshal(raw, &single); err != nil {
				return fmt.Errorf("%s has a bin field that is neither an object nor a string", path)
			}
			var name struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(data, &name)
			existing = map[string]string{path_Base(name.Name): single}
		}
	}
	for k, v := range bins {
		existing[k] = v
	}

	raw, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	manifest["bin"] = raw

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// path_Base is filepath.Base, named apart so the scoped-package case reads
// clearly: "@scope/thing" publishes an executable called "thing".
func path_Base(pkg string) string {
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		return pkg[i+1:]
	}
	return pkg
}

// buildHoisted writes the tree npm would have written: every reachable
// resolution copied to where a directory walk will find it, and not one symlink
// anywhere.
func buildHoisted(root string, sources []Source, links []Ref) error {
	placements, err := place(sources, links)
	if err != nil {
		return err
	}
	byName := make(map[string]Source, len(sources))
	for _, s := range sources {
		byName[s.Meta.Name] = s
	}
	for entry, paths := range placements {
		src := byName[entry]
		for _, at := range paths {
			if err := copyTree(src.Dir, filepath.Join(root, filepath.FromSlash(at))); err != nil {
				return fmt.Errorf("staging %s at %s: %w", entry, at, err)
			}
		}
	}
	return nil
}
