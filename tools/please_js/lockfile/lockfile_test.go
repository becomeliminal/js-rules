package lockfile_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"tools/please_js/lockfile"
)

func TestSplitKey(t *testing.T) {
	// The cases that make this non-trivial: a scope's '@' is not a version
	// separator, and a peer suffix can nest and carry '@' of its own.
	tests := []struct {
		key                  string
		name, version, peers string
	}{
		{"react@18.3.1", "react", "18.3.1", ""},
		{"@aspect-test/c@1.0.0", "@aspect-test/c", "1.0.0", ""},
		{
			"@aspect-test/d@2.0.0(@aspect-test/c@2.0.2)",
			"@aspect-test/d", "2.0.0", "(@aspect-test/c@2.0.2)",
		},
		{
			"@testing-library/react@15.2.6(react-dom@19.2.4(react@19.2.4))(react@19.2.4)",
			"@testing-library/react", "15.2.6",
			"(react-dom@19.2.4(react@19.2.4))(react@19.2.4)",
		},
		{"react-dom@19.2.4(react@19.2.4)", "react-dom", "19.2.4", "(react@19.2.4)"},
	}

	for _, tt := range tests {
		name, version, peers := lockfile.SplitKey(tt.key)
		if name != tt.name || version != tt.version || peers != tt.peers {
			t.Errorf("SplitKey(%q)\n got  name=%q version=%q peers=%q\n want name=%q version=%q peers=%q",
				tt.key, name, version, peers, tt.name, tt.version, tt.peers)
		}
	}
}

func TestIsLink(t *testing.T) {
	if !lockfile.IsLink("link:../lib") {
		t.Error("link:../lib should be a workspace link")
	}
	if lockfile.IsLink("1.0.0") {
		t.Error("a plain version is not a workspace link")
	}
}

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func parseNPMFixture(t *testing.T, body string) *lockfile.Lockfile {
	t.Helper()
	lock, err := lockfile.Parse(writeTemp(t, "package-lock.json", body))
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func keysOf(m map[string]lockfile.Snapshot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// npm records a filesystem rather than a graph, so resolution is node's own
// algorithm applied to the paths npm wrote: beside the importer, then each
// ancestor, then the root.
func TestNPMNestingIsResolvedTheWayNodeWould(t *testing.T) {
	lock := parseNPMFixture(t, `{
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"name": "root", "dependencies": {"a": "^1", "shared": "^2"}},
	    "node_modules/a": {"version": "1.0.0", "dependencies": {"shared": "^1"}},
	    "node_modules/a/node_modules/shared": {"version": "1.5.0"},
	    "node_modules/shared": {"version": "2.0.0"}
	  }
	}`)

	// Two resolutions of one package, which is the whole reason npm nests. They
	// must not collapse into one entry.
	if len(lock.Snapshots) != 3 {
		t.Fatalf("want three entries, got %d: %v", len(lock.Snapshots), keysOf(lock.Snapshots))
	}

	// a sees the nested one, because it is nearer.
	var aKey string
	for k := range lock.Snapshots {
		if strings.HasPrefix(k, "a@1.0.0") {
			aKey = k
		}
	}
	got := lock.Snapshots[aKey].Dependencies["shared"]
	if !strings.HasPrefix(got, "shared@1.5.0") {
		t.Errorf("a resolved shared to %q; the nested copy is nearer", got)
	}

	// The root sees the hoisted one.
	root := lock.Importers["."].Dependencies["shared"].Version
	if !strings.HasPrefix(root, "shared@2.0.0") {
		t.Errorf("the root resolved shared to %q", root)
	}
}

// Same name and version at two paths, resolving different dependencies, is two
// resolutions. Merging them would give one package the other's graph, which is
// the mistake worth never making.
func TestSameVersionWithDifferentDepsStaysTwoEntries(t *testing.T) {
	lock := parseNPMFixture(t, `{
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"name": "root", "dependencies": {"a": "^1", "b": "^1"}},
	    "node_modules/a": {"version": "1.0.0", "dependencies": {"dep": "*"}},
	    "node_modules/dep": {"version": "2.0.0"},
	    "node_modules/b": {"version": "1.0.0", "dependencies": {"a": "^1"}},
	    "node_modules/b/node_modules/a": {"version": "1.0.0", "dependencies": {"dep": "*"}},
	    "node_modules/b/node_modules/dep": {"version": "1.0.0"}
	  }
	}`)
	var got []string
	for k, snap := range lock.Snapshots {
		if strings.HasPrefix(k, "a@1.0.0") {
			got = append(got, snap.Dependencies["dep"])
		}
	}
	sort.Strings(got)
	if len(got) != 2 {
		t.Fatalf("want both copies of a@1.0.0, got %d: %v", len(got), keysOf(lock.Snapshots))
	}
	if !strings.HasPrefix(got[0], "dep@1.0.0") || !strings.HasPrefix(got[1], "dep@2.0.0") {
		t.Errorf("the two copies should see different deps, got %v", got)
	}
}

// A package with no dependencies resolves nothing, so nothing about where it
// sits can change what it is. Those collapse -- 42 of the 49 duplicated names
// in a real 1,283-package lockfile are exactly this.
func TestAPackageThatResolvesNothingCollapses(t *testing.T) {
	lock := parseNPMFixture(t, `{
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"name": "root", "dependencies": {"a": "^1", "b": "^1"}},
	    "node_modules/a": {"version": "1.0.0"},
	    "node_modules/b": {"version": "1.0.0", "dependencies": {"a": "^1"}},
	    "node_modules/b/node_modules/a": {"version": "1.0.0"}
	  }
	}`)
	n := 0
	for k := range lock.Snapshots {
		if strings.HasPrefix(k, "a@1.0.0") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want one entry for a package with no dependencies, got %d: %v", n, keysOf(lock.Snapshots))
	}
}

// A workspace package is a link to a path in the repo, which is pnpm's link: by
// another spelling -- and everything downstream already knows that spelling.
func TestWorkspacePackagesBecomeLinks(t *testing.T) {
	lock := parseNPMFixture(t, `{
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"name": "root", "dependencies": {"@me/shared": "*"}},
	    "packages/shared": {"name": "@me/shared", "version": "1.0.0"},
	    "node_modules/@me/shared": {"resolved": "packages/shared", "link": true}
	  }
	}`)
	got := lock.Importers["."].Dependencies["@me/shared"].Version
	if got != "link:packages/shared" {
		t.Errorf("got %q, want link:packages/shared", got)
	}
	// And it is not fetched, because the repo builds it.
	for k := range lock.Packages {
		if strings.Contains(k, "@me/shared") {
			t.Errorf("a workspace package should not be fetched: %s", k)
		}
	}
}

// v1 put the graph somewhere else entirely. Reading half of it silently is
// worse than saying which npm writes what this reads.
func TestOldLockfilesAreRefusedByName(t *testing.T) {
	_, err := lockfile.Parse(writeTemp(t, "package-lock.json", `{"lockfileVersion": 1, "packages": {}}`))
	if err == nil || !strings.Contains(err.Error(), "lockfileVersion 1") {
		t.Errorf("got %v", err)
	}
}

// An optional dependency the tree does not contain was skipped on purpose --
// a platform it does not support -- so its absence is the lockfile working.
func TestASkippedOptionalDependencyIsNotAGap(t *testing.T) {
	lock := parseNPMFixture(t, `{
	  "lockfileVersion": 3,
	  "packages": {
	    "": {"name": "root", "dependencies": {"a": "^1"}},
	    "node_modules/a": {"version": "1.0.0", "optionalDependencies": {"linux-only": "^1"}}
	  }
	}`)
	for k, snap := range lock.Snapshots {
		if strings.HasPrefix(k, "a@") && len(snap.OptionalDependencies) != 0 {
			t.Errorf("got %v, want nothing recorded for a dependency not in the tree", snap.OptionalDependencies)
		}
	}
}
