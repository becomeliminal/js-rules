// A thousand-package graph through parse, generate and link, with hard upper
// bounds. The predecessor of this stack had a warm build go from 270ms to 50s
// -- a 185x regression from carrying the closure through exported_deps -- and
// only a test at this size would have caught it before a user did.
//
// The fixtures are synthesised, not recorded: a checked-in real lockfile would
// be someone's dependency list frozen in 2026, and the shape is what matters --
// wide fan-out, deep chains, peer-style duplicate resolutions.
//
// The bounds are deliberately loose. Each sits an order of magnitude above the
// measured time on ordinary hardware, so they fail on a regression of the kind
// worth catching and never on a slow laptop.
package scale_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tools/please_js/generate"
	"tools/please_js/lockfile"
	"tools/please_js/store"
)

const packages = 1000

// synthesise writes a pnpm lockfile with the shapes that exist in real trees:
// a root with a hundred direct dependencies, chains ten deep, shared leaves
// reached by many parents, and fifty names carrying two resolutions.
func synthesise(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("lockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    dependencies:\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "      pkg%d:\n        specifier: 1.0.0\n        version: 1.0.0\n", i)
	}
	b.WriteString("\npackages:\n\n")
	for i := 0; i < packages; i++ {
		fmt.Fprintf(&b, "  'pkg%d@1.0.0':\n    resolution: {integrity: sha512-%040d}\n", i, i)
	}
	// Fifty duplicate resolutions, the peer-group shape.
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "  'pkg%d@2.0.0':\n    resolution: {integrity: sha512-dup%037d}\n", i, i)
	}
	b.WriteString("\nsnapshots:\n\n")
	for i := 0; i < packages; i++ {
		fmt.Fprintf(&b, "  'pkg%d@1.0.0':\n", i)
		// A chain (i -> i+100) and a shared leaf, so the closure walk is real.
		if i+100 < packages-1 {
			fmt.Fprintf(&b, "    dependencies:\n      pkg%d: 1.0.0\n      pkg%d: 1.0.0\n", i+100, packages-1)
		} else if i+100 == packages-1 {
			// The chain target IS the shared leaf here; naming it twice would be
			// a duplicate YAML key.
			fmt.Fprintf(&b, "    dependencies:\n      pkg%d: 1.0.0\n", i+100)
		}
	}
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "  'pkg%d@2.0.0': {}\n", i)
	}
	path := filepath.Join(t.TempDir(), "pnpm-lock.yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func bounded(t *testing.T, name string, limit time.Duration, fn func()) {
	t.Helper()
	start := time.Now()
	fn()
	took := time.Since(start)
	t.Logf("%s: %v (bound %v)", name, took, limit)
	if took > limit {
		t.Errorf("%s took %v, bound is %v -- a regression of the kind the old repo shipped", name, took, limit)
	}
}

func TestAThousandPackagesStayFast(t *testing.T) {
	path := synthesise(t)

	var lock *lockfile.Lockfile
	bounded(t, "parse", 2*time.Second, func() {
		var err error
		if lock, err = lockfile.Parse(path); err != nil {
			t.Fatal(err)
		}
	})

	var plan *generate.Plan
	bounded(t, "generate", 2*time.Second, func() {
		var err error
		if plan, err = generate.Build(lock, generate.Scope{}); err != nil {
			t.Fatal(err)
		}
	})
	if got := len(plan.Entries); got != packages+50 {
		t.Fatalf("want %d entries, got %d", packages+50, got)
	}
	if got := len(plan.Closure["."]); got != packages {
		t.Fatalf("the closure should reach every chained package, got %d", got)
	}

	// The tree itself: a store build over a thousand real directories.
	dir := t.TempDir()
	var sources []store.Source
	for i := range plan.Entries {
		e := plan.Entries[i]
		d := filepath.Join(dir, "src", e.Target)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "index.js"), []byte("//"), 0o644); err != nil {
			t.Fatal(err)
		}
		refs := make([]store.Ref, 0, len(e.Deps))
		for as, entry := range e.Deps {
			refs = append(refs, store.Ref{As: as, Entry: entry})
		}
		sources = append(sources, store.Source{
			Dir:  d,
			Meta: store.Meta{Name: e.Target, Package: e.Package},
			Deps: refs,
		})
	}
	links := plan.Links(".")

	bounded(t, "store link", 30*time.Second, func() {
		if err := store.Build(filepath.Join(dir, "store"), sources, links, store.Store); err != nil {
			t.Fatal(err)
		}
	})
	bounded(t, "hoisted link", 30*time.Second, func() {
		if err := store.Build(filepath.Join(dir, "hoisted"), sources, links, store.Hoisted); err != nil {
			t.Fatal(err)
		}
	})

	// Correctness at size, not only speed: no dangling links in the store.
	dangling := 0
	filepath.Walk(filepath.Join(dir, "store"), func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			if _, statErr := os.Stat(p); statErr != nil {
				dangling++
			}
		}
		return nil
	})
	if dangling != 0 {
		t.Errorf("%d dangling symlinks in a thousand-package store", dangling)
	}
}
