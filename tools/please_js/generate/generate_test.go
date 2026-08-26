package generate_test

import (
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
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

// A private scope is how organisations publish internally, and the mapping
// lives beside the lockfile rather than in it -- pnpm reads .npmrc, this reads
// flags. The lockfile's own URL is more specific than any mapping and wins.
func TestScopeRegistriesPointScopedPackages(t *testing.T) {
	plan := &generate.Plan{Entries: []generate.Entry{
		{Target: "a", Package: "@acme/a"},
		{Target: "b", Package: "@acme/b", URL: "https://exact.example/b.tgz"},
		{Target: "c", Package: "@other/c"},
		{Target: "d", Package: "plain"},
	}}
	if err := plan.ApplyScopeRegistries([]string{"@acme=https://registry.acme.dev/"}); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range plan.Entries {
		got[e.Target] = e.Registry
	}
	if got["a"] != "https://registry.acme.dev" {
		t.Errorf("@acme/a should use the scope registry, got %q", got["a"])
	}
	if got["b"] != "" {
		t.Errorf("a lockfile-recorded URL is more specific than a mapping, got %q", got["b"])
	}
	if got["c"] != "" || got["d"] != "" {
		t.Errorf("unmapped packages keep the default: %v", got)
	}
	if err := plan.ApplyScopeRegistries([]string{"acme=nope"}); err == nil {
		t.Error("a mapping without @scope=url shape should be refused")
	}
}

// The ticket's fake registry, as a real server: the token must arrive in the
// request, and must exist nowhere but the request -- it reaches the hasher via
// a flag whose value came from the environment, and is never written anywhere.
func TestHasherSendsHeadersToThePrivateRegistry(t *testing.T) {
	tarball := []byte("not really a tarball, but bytes hash all the same")
	sum512 := sha512.Sum512(tarball)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum512[:])

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write(tarball)
	}))
	defer server.Close()

	h := &generate.Hasher{
		Registry: server.URL,
		Headers:  []string{"Authorization: Bearer sekrit-token"},
		Workers:  1,
	}
	sums, err := h.Resolve([]generate.Entry{
		{Target: "p_1", Package: "p", Version: "1.0.0", Integrity: integrity},
	}, func(done, total int) {})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sekrit-token" {
		t.Errorf("the registry saw %q", gotAuth)
	}
	if len(sums) != 1 || sums[0] == "" {
		t.Errorf("hashing should still work through the header path: %v", sums)
	}
	// And the token reaches no output: the emitted build file carries hashes
	// and URLs, never headers.
	dir := t.TempDir()
	path := filepath.Join(dir, "BUILD")
	plan := &generate.Plan{
		Entries: []generate.Entry{{Target: "p_1", Package: "p", Version: "1.0.0",
			URL: server.URL + "/p/-/p-1.0.0.tgz", Registry: "https://reg.example"}},
		Closure: map[string][]string{}, Direct: map[string]map[string]string{},
		Workspace: map[string]map[string]string{},
	}
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", sums, generate.Scope{}, false); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "sekrit") {
		t.Error("the token leaked into a generated file")
	}
	if !strings.Contains(string(out), `url = "`+server.URL) {
		t.Errorf("the exact URL should be emitted:\n%s", out)
	}
}

// The hoisted twin is emitted from the same closure as the store-layout tree,
// so the two can never disagree about what a repin staged.
func TestWriteBUILDHoistedLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BUILD")
	plan := &generate.Plan{
		Entries: []generate.Entry{{Target: "p_1", Package: "p", Version: "1.0.0"}},
		Closure: map[string][]string{".": {"p_1"}},
		Direct:  map[string]map[string]string{},
		Workspace: map[string]map[string]string{},
	}
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", nil, generate.Scope{NoDev: true}, true); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	for _, want := range []string{`name = "node_modules"`, `name = "hoisted"`, "hoisted = True"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("generated file should contain %q:\n%s", want, out)
		}
	}
	// Policy flags carry onto both trees: a hoisted tree that silently kept
	// devDependencies would stage what the other tree refused.
	if strings.Count(string(out), "no_dev = True") != 2 {
		t.Errorf("no_dev should be on both link targets:\n%s", out)
	}
	// Without the flag, no hoisted target appears.
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", nil, generate.Scope{}, false); err != nil {
		t.Fatal(err)
	}
	out, _ = os.ReadFile(path)
	if strings.Contains(string(out), "hoisted") {
		t.Errorf("hoisted emission is opt-in:\n%s", out)
	}
}
