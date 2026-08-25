// Package generate turns a parsed lockfile into BUILD file content.
package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"tools/please_js/lockfile"
	"tools/please_js/store"
)

// Package store target names try to stay readable, so a store path is
// recognisable in a stack trace, while staying short enough that the deeply
// nested store layout does not run into filesystem name limits. pnpm shortens
// on the same grounds; these bounds follow theirs.
const (
	maxTargetLength = 100
	hashLength      = 8
)

// Entry is one store target to emit.
type Entry struct {
	Target    string            // Please target name
	Package   string            // npm package name
	Version   string            // exact version
	Key       string            // store key, carrying the peer resolution
	Deps      map[string]string // import name -> target name
	Integrity string            // from the lockfile, verbatim
	HasBin    bool
	OS        []string
	CPU       []string

	// RunHooks is set from an allowlist a person wrote, never from anything the
	// lockfile or the package says. A package declaring install scripts is not a
	// reason to run them -- that is the decision, and it belongs to whoever owns
	// the repo rather than to whoever published the package.
	RunHooks bool
}

// AllowHooks marks the named packages as permitted to run their own install
// scripts.
//
// Names are npm package names rather than target names, because that is what a
// person reads in a lockfile and what the ecosystem talks about -- and because
// one package can resolve to several targets under different peers, all of
// which should be allowed together or not at all.
//
// A name that matches nothing is an error. The alternative is a silent no-op,
// and the thing it silently fails to do is run a build step the package needs,
// which surfaces much later as a missing file.
func (p *Plan) AllowHooks(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	want := make(map[string]bool, len(packages))
	for _, name := range packages {
		want[name] = true
	}
	seen := make(map[string]bool, len(packages))
	for i := range p.Entries {
		if want[p.Entries[i].Package] {
			p.Entries[i].RunHooks = true
			seen[p.Entries[i].Package] = true
		}
	}
	var missing []string
	for _, name := range packages {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("--lifecycle-hooks names %v, which the lockfile does not resolve", missing)
	}
	return nil
}

// Plan is everything derived from one lockfile.
type Plan struct {
	Entries []Entry
	// Closure maps an importer path to every target reachable from it. The
	// link rule needs this in full: an entry reachable only through another
	// package's dep symlink still has to be staged, or the symlink dangles.
	Closure map[string][]string
	// Direct maps an importer path to the packages linked at its top level,
	// by the name source code imports them under.
	Direct map[string]map[string]string
}

// Build derives the plan from a lockfile.
func Build(lock *lockfile.Lockfile) (*Plan, error) {
	plan := &Plan{
		Closure: map[string][]string{},
		Direct:  map[string]map[string]string{},
	}

	// One entry per snapshot: the same package resolved against different
	// peers is a different snapshot, and therefore a separate store entry
	// that coexists with the others.
	targets := map[string]string{} // snapshot key -> target name
	for _, key := range lock.SnapshotKeys() {
		if _, version, _ := lockfile.SplitKey(key); version == "" {
			return nil, fmt.Errorf("snapshot %q has no version", key)
		}
		targets[key] = TargetName(key)
	}

	for _, key := range lock.SnapshotKeys() {
		name, version, _ := lockfile.SplitKey(key)
		snap := lock.Snapshots[key]

		deps := map[string]string{}
		for alias, depKey := range allDeps(snap) {
			// A dependency value is a version, which is only a snapshot key
			// once the package name is prepended.
			full := depKey
			if _, ok := lock.Snapshots[full]; !ok {
				full = alias + "@" + depKey
			}
			if _, ok := lock.Snapshots[full]; !ok {
				// pnpm records optional dependencies that were not installed
				// on this platform. Skipping them is correct: the package is
				// expected to cope with their absence.
				continue
			}
			deps[alias] = targets[full]
		}

		// Static facts live under name@version, without the peer suffix.
		meta := lock.Packages[name+"@"+version]
		plan.Entries = append(plan.Entries, Entry{
			Target:    targets[key],
			Package:   name,
			Version:   version,
			Key:       key,
			Deps:      deps,
			Integrity: meta.Resolution.Integrity,
			HasBin:    meta.HasBin,
			OS:        meta.OS,
			CPU:       meta.CPU,
		})
	}

	for path, imp := range lock.Importers {
		direct := map[string]string{}
		for alias, dep := range allImporterDeps(imp) {
			if lockfile.IsLink(dep.Version) {
				// Workspace packages are built from source in the consumer's
				// own repo, so they are not fetched here.
				continue
			}
			key := dep.Version
			if _, ok := lock.Snapshots[key]; !ok {
				key = alias + "@" + dep.Version
			}
			if _, ok := lock.Snapshots[key]; !ok {
				continue
			}
			direct[alias] = targets[key]
		}
		plan.Direct[path] = direct

		seen := map[string]bool{}
		for _, dep := range allImporterDeps(imp) {
			key := dep.Version
			if _, ok := lock.Snapshots[key]; !ok {
				continue
			}
			collect(lock, key, targets, seen)
		}
		for alias, dep := range allImporterDeps(imp) {
			key := alias + "@" + dep.Version
			if _, ok := lock.Snapshots[key]; ok {
				collect(lock, key, targets, seen)
			}
		}
		closure := make([]string, 0, len(seen))
		for t := range seen {
			closure = append(closure, t)
		}
		sort.Strings(closure)
		plan.Closure[path] = closure
	}

	return plan, nil
}


// collect walks a snapshot's dependencies, recording every target reachable
// from it.
func collect(lock *lockfile.Lockfile, key string, targets map[string]string, seen map[string]bool) {
	target, ok := targets[key]
	if !ok || seen[target] {
		return
	}
	seen[target] = true
	for alias, depKey := range allDeps(lock.Snapshots[key]) {
		full := depKey
		if _, ok := lock.Snapshots[full]; !ok {
			full = alias + "@" + depKey
		}
		collect(lock, full, targets, seen)
	}
}

func allDeps(s lockfile.Snapshot) map[string]string {
	out := map[string]string{}
	for k, v := range s.Dependencies {
		out[k] = v
	}
	for k, v := range s.OptionalDependencies {
		out[k] = v
	}
	return out
}

func allImporterDeps(i lockfile.Importer) map[string]lockfile.ImporterDep {
	out := map[string]lockfile.ImporterDep{}
	for k, v := range i.Dependencies {
		out[k] = v
	}
	for k, v := range i.DevDependencies {
		out[k] = v
	}
	for k, v := range i.OptionalDependencies {
		out[k] = v
	}
	return out
}

// TargetName converts a snapshot key into a Please target name.
//
// Readability matters here -- the target name shows up in build output and in
// the store path -- so the package name and version survive intact where they
// can, and only the peer suffix, which is long and rarely interesting, is
// hashed away when it pushes the name past the limit.
func TargetName(key string) string {
	name, version, peers := lockfile.SplitKey(key)
	base := sanitise(name) + "_" + sanitise(version)
	if peers == "" {
		return truncate(base)
	}
	return truncate(base + "_" + shortHash(peers))
}

func sanitise(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			// Collapse runs, so @scope/name does not become scope__name.
			if !prevUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_-.")
}

func truncate(s string) string {
	if len(s) <= maxTargetLength {
		return s
	}
	return s[:maxTargetLength-hashLength-1] + "_" + shortHash(s)
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:hashLength]
}

// Refs returns what belongs in each package's own node_modules, keyed by the
// entry it belongs to. The name a dependency appears under comes from the
// lockfile, which is also where an npm alias is recorded.
func (p *Plan) Refs() map[string][]store.Ref {
	out := make(map[string][]store.Ref, len(p.Entries))
	for _, e := range p.Entries {
		refs := make([]store.Ref, 0, len(e.Deps))
		for _, as := range sortedKeysOf(e.Deps) {
			refs = append(refs, store.Ref{As: as, Entry: e.Deps[as]})
		}
		out[e.Target] = refs
	}
	return out
}

// Links returns the packages a project imports at its top level, under the
// names its source code uses.
func (p *Plan) Links(project string) []store.Ref {
	direct := p.Direct[project]
	refs := make([]store.Ref, 0, len(direct))
	for _, as := range sortedKeysOf(direct) {
		refs = append(refs, store.Ref{As: as, Entry: direct[as]})
	}
	return refs
}

func sortedKeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
