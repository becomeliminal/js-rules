// Package lockfile parses pnpm-lock.yaml v9 into the graph the build rules need.
//
// A v9 lockfile splits what other lockfiles combine:
//
//	packages:   name@version -> static facts (integrity, hasBin, os, cpu)
//	snapshots:  snapshot key -> resolved dependencies
//
// The snapshot key carries the resolution that produced the package, appended
// as parenthesised peer groups which may nest:
//
//	'@testing-library/react@15.2.6(react-dom@19.2.4(react@19.2.4))(react@19.2.4)'
//
// That key is the package's identity: the same name and version resolved
// against different peers are different snapshots, and therefore different
// store entries that coexist. Everything downstream keys off it.
package lockfile

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Lockfile is the parsed form of pnpm-lock.yaml.
type Lockfile struct {
	Version   string
	Importers map[string]Importer
	Packages  map[string]Package
	Snapshots map[string]Snapshot
}

// Importer is one project in the pnpm workspace, keyed by its path.
type Importer struct {
	Dependencies         map[string]ImporterDep
	DevDependencies      map[string]ImporterDep
	OptionalDependencies map[string]ImporterDep
}

// ImporterDep is one entry in an importer's dependency map.
type ImporterDep struct {
	// Specifier is what package.json asked for: a semver range, "workspace:*",
	// or "npm:<name>@<version>" where the import name is an alias.
	Specifier string `yaml:"specifier"`
	// Version is what pnpm resolved it to: a snapshot key, or "link:<path>"
	// for a workspace package.
	Version string `yaml:"version"`
}

// Package is the static metadata for one name@version, independent of how it
// was resolved. Registry facts live here; the dependency graph lives in
// Snapshot.
type Package struct {
	Resolution struct {
		Integrity string `yaml:"integrity"`
		Tarball   string `yaml:"tarball"`
	} `yaml:"resolution"`
	HasBin     bool     `yaml:"hasBin"`
	Deprecated string   `yaml:"deprecated"`
	CPU        []string `yaml:"cpu"`
	OS         []string `yaml:"os"`
	Engines    map[string]string
}

// Snapshot is one resolved instance of a package: its dependencies as pnpm
// resolved them for this particular peer combination.
type Snapshot struct {
	Dependencies         map[string]string `yaml:"dependencies"`
	OptionalDependencies map[string]string `yaml:"optionalDependencies"`
}

// rawLockfile mirrors the on-disk shape. Importer dependency maps are decoded
// through yaml.Node so an empty importer ("." with {}) does not fail.
type rawLockfile struct {
	LockfileVersion string                     `yaml:"lockfileVersion"`
	Importers       map[string]rawImporter     `yaml:"importers"`
	Packages        map[string]Package         `yaml:"packages"`
	Snapshots       map[string]Snapshot        `yaml:"snapshots"`
	Dependencies    map[string]ImporterDep     `yaml:"dependencies"`
	DevDependencies map[string]ImporterDep     `yaml:"devDependencies"`
}

type rawImporter struct {
	Dependencies         map[string]ImporterDep `yaml:"dependencies"`
	DevDependencies      map[string]ImporterDep `yaml:"devDependencies"`
	OptionalDependencies map[string]ImporterDep `yaml:"optionalDependencies"`
}

// Parse reads a pnpm-lock.yaml.
func Parse(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile: %w", err)
	}

	var raw rawLockfile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// v9 moved the dependency graph out of `packages` and into `snapshots`.
	// Refusing older versions is deliberate: silently reading a v6 lockfile
	// would produce a graph with no peer resolution and no way to notice.
	major := strings.SplitN(raw.LockfileVersion, ".", 2)[0]
	if major != "9" {
		return nil, fmt.Errorf(
			"lockfile version %q is not supported; please_js needs v9 (pnpm 9+). "+
				"Run `pnpm import` to convert an npm or yarn lockfile",
			raw.LockfileVersion)
	}

	lock := &Lockfile{
		Version:   raw.LockfileVersion,
		Importers: map[string]Importer{},
		Packages:  raw.Packages,
		Snapshots: raw.Snapshots,
	}
	for path, imp := range raw.Importers {
		lock.Importers[path] = Importer{
			Dependencies:         imp.Dependencies,
			DevDependencies:      imp.DevDependencies,
			OptionalDependencies: imp.OptionalDependencies,
		}
	}
	if lock.Packages == nil {
		lock.Packages = map[string]Package{}
	}
	if lock.Snapshots == nil {
		lock.Snapshots = map[string]Snapshot{}
	}
	return lock, nil
}

// SnapshotKeys returns every snapshot key in a stable order.
func (l *Lockfile) SnapshotKeys() []string {
	keys := make([]string, 0, len(l.Snapshots))
	for k := range l.Snapshots {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SplitKey separates a snapshot key into the package name, the version, and the
// parenthesised peer suffix that distinguishes this resolution from another.
//
//	"@aspect-test/d@2.0.0(@aspect-test/c@2.0.2)"
//	  -> "@aspect-test/d", "2.0.0", "(@aspect-test/c@2.0.2)"
//
// The scope's leading '@' is not a version separator, and the peer suffix can
// itself contain '@' and nested parens, so neither a split on '@' nor a split
// on '(' is sufficient on its own.
func SplitKey(key string) (name, version, peers string) {
	rest := key
	// A peer suffix is a balanced '(' group that runs to the end of the key.
	if i := peerSuffixStart(rest); i >= 0 {
		peers = rest[i:]
		rest = rest[:i]
	}
	// Scoped names begin with '@', so look for the version separator after it.
	at := strings.LastIndex(rest, "@")
	if at <= 0 {
		return rest, "", peers
	}
	return rest[:at], rest[at+1:], peers
}

// peerSuffixStart returns the index where the trailing run of balanced peer
// groups begins, or -1 when the key carries none.
//
// The groups nest and there can be several in a row, so scanning forward for
// the first '(' is wrong: in
//
//	react@15.2.6(react-dom@19.2.4(react@19.2.4))(react@19.2.4)
//
// the first group closes well before the end. Walking right to left and
// consuming one balanced group at a time finds the true start of the run.
func peerSuffixStart(s string) int {
	end, start := len(s), -1
	for end > 0 && s[end-1] == ')' {
		depth, i := 0, end-1
		for ; i >= 0; i-- {
			switch s[i] {
			case ')':
				depth++
			case '(':
				depth--
			}
			if depth == 0 && s[i] == '(' {
				break
			}
		}
		if i < 0 {
			return -1 // unbalanced; treat the key as having no suffix
		}
		start, end = i, i
	}
	return start
}

// IsLink reports whether a resolved version refers to a workspace package
// rather than something fetched from a registry.
func IsLink(version string) bool { return strings.HasPrefix(version, "link:") }
