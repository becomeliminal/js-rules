package lockfile

import (
	"encoding/json"
	"fmt"
	"strings"
)

// npm's lockfile says where a package sits rather than what it resolved to.
//
// pnpm records a snapshot per resolution, with the peers folded into the key,
// so the graph is already explicit. npm records a filesystem: "node_modules/a"
// is hoisted, "node_modules/a/node_modules/b" is b nested inside a because
// something else needed a different b. Dependencies are ranges, not versions.
//
// Resolving one is therefore not semver work -- the lockfile has already
// decided -- it is node's own algorithm applied to those paths: look beside the
// package, then in each parent, until the root. That is what npm guarantees
// about the tree it wrote, so reading it back is exact rather than approximate.
type npmLock struct {
	LockfileVersion int                   `json:"lockfileVersion"`
	Packages        map[string]npmPackage `json:"packages"`
}

type npmPackage struct {
	// Name is set only when the package is installed under a different one --
	// npm's alias form, `npm install x-cjs@npm:x@7`. The path then says what to
	// import it as and this says what to fetch, and confusing the two asks the
	// registry for a package that does not exist.
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Resolved             string            `json:"resolved"`
	Integrity            string            `json:"integrity"`
	Dev                  bool              `json:"dev"`
	Optional             bool              `json:"optional"`
	Link                 bool              `json:"link"`
	Bin                  json.RawMessage   `json:"bin"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
}

// importName returns the name a path is imported as.
//
// Everything after the last node_modules segment, so a scope survives and
// nesting does not: "a/node_modules/@scope/b" is "@scope/b".
func importName(path string) string {
	i := strings.LastIndex(path, "node_modules/")
	if i < 0 {
		return path
	}
	return path[i+len("node_modules/"):]
}

// registryName returns the name to fetch, which is not always the name to
// import.
//
// `npm install wrap-ansi-cjs@npm:wrap-ansi@7` installs one package under
// another name, and npm records the real one in the entry. Three of the 1,283
// packages in one real workspace are aliases like this, and asking the registry
// for the alias gets a 404.
func registryName(path string, pkg npmPackage) string {
	if pkg.Name != "" {
		return pkg.Name
	}
	return importName(path)
}

// resolveFrom finds which entry the package at `from` gets for `dep`.
//
// Node's algorithm, which is what npm laid the tree out for: beside the
// importer first, then each ancestor, then the root.
func resolveFrom(pkgs map[string]npmPackage, from, dep string) (string, bool) {
	prefix := from
	for {
		candidate := "node_modules/" + dep
		if prefix != "" {
			candidate = prefix + "/node_modules/" + dep
		}
		if _, ok := pkgs[candidate]; ok {
			return candidate, true
		}
		if prefix == "" {
			return "", false
		}
		if i := strings.LastIndex(prefix, "/node_modules/"); i >= 0 {
			prefix = prefix[:i]
		} else {
			prefix = ""
		}
	}
}

// parseNPM converts a package-lock.json into the same shape the pnpm parser
// produces, so nothing downstream knows which format a repo uses.
func parseNPM(data []byte, path string) (*Lockfile, error) {
	var raw npmLock
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// v1 put the graph in a "dependencies" tree instead, and v2 carries both
	// for compatibility. Refusing v1 outright is deliberate: reading half a
	// lockfile silently is worse than saying which npm to run.
	if raw.LockfileVersion < 2 {
		return nil, fmt.Errorf(
			"%s is lockfileVersion %d; this reads 2 and 3, which npm 7 and later write",
			path, raw.LockfileVersion)
	}

	lock := &Lockfile{
		Version:   fmt.Sprintf("%d", raw.LockfileVersion),
		Importers: map[string]Importer{},
		Packages:  map[string]Package{},
		Snapshots: map[string]Snapshot{},
	}

	// The key for a path. A name@version that appears once needs nothing more;
	// one that appears at several paths is genuinely several resolutions, and
	// the path distinguishes them the way a peer group does in pnpm. Carried in
	// parentheses so SplitKey reads it as the disambiguator it is.
	// A path outside node_modules is a project in the repo, not something to
	// fetch: the root, or a workspace. They are read further down as importers.
	// Before aliases were handled, such an entry was keyed by its path and so
	// never collided with anything, which hid the fact that it was here at all.
	fetchable := func(path string, pkg npmPackage) bool {
		return path != "" && !pkg.Link && pkg.Version != "" && strings.Contains(path, "node_modules/")
	}

	counts := map[string]int{}
	// A package with no dependencies of its own has the same identity wherever
	// it sits: nothing about its position can change what it resolves, because
	// it resolves nothing. Those collapse. Anything with dependencies keeps its
	// path, because its position is exactly what decides them -- and merging
	// two that differ would give one package the other's graph, which is the
	// mistake worth never making.
	positionless := map[string]bool{}
	for path, pkg := range raw.Packages {
		if !fetchable(path, pkg) {
			continue
		}
		base := registryName(path, pkg) + "@" + pkg.Version
		counts[base]++
		hasDeps := len(pkg.Dependencies) > 0 || len(pkg.OptionalDependencies) > 0
		if _, seen := positionless[base]; !seen {
			positionless[base] = true
		}
		if hasDeps {
			positionless[base] = false
		}
	}
	keyOf := func(p string) string {
		pkg := raw.Packages[p]
		base := registryName(p, pkg) + "@" + pkg.Version
		if counts[base] > 1 && !positionless[base] {
			return base + "(" + p + ")"
		}
		return base
	}

	for p, pkg := range raw.Packages {
		if !fetchable(p, pkg) {
			continue
		}
		key := keyOf(p)

		entry := Package{}
		entry.Resolution.Integrity = pkg.Integrity
		entry.Resolution.Tarball = pkg.Resolved
		entry.HasBin = len(pkg.Bin) > 0
		entry.OS = pkg.OS
		entry.CPU = pkg.CPU
		lock.Packages[key] = entry

		deps := map[string]string{}
		optional := map[string]string{}
		for dep := range pkg.Dependencies {
			if at, ok := resolveFrom(raw.Packages, p, dep); ok {
				deps[dep] = keyOf(at)
			}
		}
		for dep := range pkg.OptionalDependencies {
			// An optional dependency the tree does not contain was skipped on
			// purpose -- a platform it does not support, usually -- so its
			// absence is the lockfile working, not a gap in it.
			if at, ok := resolveFrom(raw.Packages, p, dep); ok {
				optional[dep] = keyOf(at)
			}
		}
		lock.Snapshots[key] = Snapshot{Dependencies: deps, OptionalDependencies: optional}
	}

	// The root project, and any workspace: an entry outside node_modules is a
	// path in the repo rather than a fetched package.
	for p, pkg := range raw.Packages {
		if strings.Contains(p, "node_modules") {
			continue
		}
		imp := Importer{
			Dependencies:         map[string]ImporterDep{},
			DevDependencies:      map[string]ImporterDep{},
			OptionalDependencies: map[string]ImporterDep{},
		}
		add := func(into map[string]ImporterDep, from map[string]string) {
			for dep, spec := range from {
				at, ok := resolveFrom(raw.Packages, p, dep)
				if !ok {
					continue
				}
				if raw.Packages[at].Link {
					// npm records a workspace package as a link to a path in
					// the repo, which is pnpm's link: by another spelling.
					into[dep] = ImporterDep{Specifier: spec, Version: "link:" + raw.Packages[at].Resolved}
					continue
				}
				into[dep] = ImporterDep{Specifier: spec, Version: keyOf(at)}
			}
		}
		add(imp.Dependencies, pkg.Dependencies)
		add(imp.DevDependencies, pkg.DevDependencies)
		add(imp.OptionalDependencies, pkg.OptionalDependencies)

		name := p
		if name == "" {
			name = "."
		}
		lock.Importers[name] = imp
	}

	if len(lock.Importers) == 0 {
		return nil, fmt.Errorf("%s has no root project", path)
	}
	return lock, nil
}
