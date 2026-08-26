package generate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/generate"
	"tools/please_js/lockfile"
)

// A package declaring install scripts is not a reason to run them. Someone
// naming it is, and only the packages named are marked.
func TestAllowHooksMarksOnlyWhatWasNamed(t *testing.T) {
	plan := &generate.Plan{Entries: []generate.Entry{
		{Target: "a_1", Package: "a"},
		{Target: "b_1", Package: "b"},
		{Target: "b_1_peer", Package: "b"},
	}}
	if err := plan.AllowHooks([]string{"b"}); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range plan.Entries {
		got[e.Target] = e.RunHooks
	}
	// Both resolutions of b, because one package under two peer resolutions is
	// one decision, not two.
	if got["a_1"] || !got["b_1"] || !got["b_1_peer"] {
		t.Errorf("marked %v", got)
	}
}

// A name that matches nothing is a typo or a package that moved, and the thing
// it silently fails to do is run a build step something needs.
func TestAllowHooksRefusesANameItCannotResolve(t *testing.T) {
	plan := &generate.Plan{Entries: []generate.Entry{{Target: "a_1", Package: "a"}}}
	err := plan.AllowHooks([]string{"a", "typo"})
	if err == nil {
		t.Fatal("expected an error naming the unresolved package")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error should name what was not found, got: %v", err)
	}
}

// The same package can be a devDependency at the top level and a transitive
// dependency of something shipped. Filtering the finished closure would drop it
// and break production, so the roots are filtered and the closure recomputed.
func TestNoDevKeepsWhatProductionAlsoReaches(t *testing.T) {
	lock := &lockfile.Lockfile{
		Importers: map[string]lockfile.Importer{
			".": {
				Dependencies:    map[string]lockfile.ImporterDep{"shipped": {Version: "1.0.0"}},
				DevDependencies: map[string]lockfile.ImporterDep{"shared": {Version: "1.0.0"}},
			},
		},
		Snapshots: map[string]lockfile.Snapshot{
			"shipped@1.0.0": {Dependencies: map[string]string{"shared": "1.0.0"}},
			"shared@1.0.0":  {},
			"devonly@1.0.0": {},
		},
		Packages: map[string]lockfile.Package{
			"shipped@1.0.0": {}, "shared@1.0.0": {}, "devonly@1.0.0": {},
		},
	}
	lock.Importers["."].DevDependencies["devonly"] = lockfile.ImporterDep{Version: "1.0.0"}

	plan, err := generate.Build(lock, generate.Scope{NoDev: true})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]bool{}
	for _, target := range plan.Closure["."] {
		in[target] = true
	}
	if !in["shipped_1.0.0"] {
		t.Error("a production dependency was dropped")
	}
	if !in["shared_1.0.0"] {
		t.Error("a package production reaches was dropped because it is also a devDependency")
	}
	if in["devonly_1.0.0"] {
		t.Error("a dev-only package survived no_dev")
	}
}

// Node resolves by walking up, and a staged action sits inside the repo, so the
// repo's own node_modules is on that path. A declared dependency still resolves
// from the nearer staged tree; an undeclared one falls through to this and the
// build passes here and nowhere else.
func TestStrayModulesReportsOnlyWhatIsThere(t *testing.T) {
	dir := t.TempDir()
	if got := generate.StrayModules(dir); got != "" {
		t.Errorf("nothing there, but got: %s", got)
	}

	// A file rather than a directory is not a module tree.
	if err := os.WriteFile(filepath.Join(dir, "node_modules"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := generate.StrayModules(dir); got != "" {
		t.Errorf("a file is not a tree, but got: %s", got)
	}

	os.Remove(filepath.Join(dir, "node_modules"))
	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := generate.StrayModules(dir)
	if !strings.Contains(got, "node_modules") || !strings.Contains(got, "undeclared") {
		t.Errorf("the warning should name the path and say what goes wrong, got: %s", got)
	}
}
