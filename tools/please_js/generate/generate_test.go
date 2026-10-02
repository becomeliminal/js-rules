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
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", sums, generate.Scope{}, false, false); err != nil {
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
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", nil, generate.Scope{NoDev: true}, true, false); err != nil {
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
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", nil, generate.Scope{}, false, false); err != nil {
		t.Fatal(err)
	}
	out, _ = os.ReadFile(path)
	if strings.Contains(string(out), "hoisted") {
		t.Errorf("hoisted emission is opt-in:\n%s", out)
	}
}

// A workspace project elsewhere in the repo gets nothing in the generated file
// but its pins and the lockfile export: npm_project resolves which pins it
// needs from the lockfile at build time, so no list for it exists anywhere.
func TestWriteBUILDGivesAProjectElsewhereOnlyThePinsAndTheLockfile(t *testing.T) {
	// GIVEN a workspace whose only project lives outside the tree's package
	dir := t.TempDir()
	path := filepath.Join(dir, "BUILD")
	plan := &generate.Plan{
		Entries:   []generate.Entry{{Target: "p_1", Package: "p", Version: "1.0.0"}},
		Closure:   map[string][]string{".": nil, "../../x402stack/frontend": {"p_1"}},
		Direct:    map[string]map[string]string{},
		Workspace: map[string]map[string]string{},
	}

	// WHEN the BUILD file is written
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "pnpm-lock.yaml", nil, generate.Scope{}, true, false); err != nil {
		t.Fatal(err)
	}

	// THEN it holds the pin and the lockfile export, and nothing that lists
	// the project's packages or names the project
	out, _ := os.ReadFile(path)
	for _, want := range []string{`name = "p_1"`, `name = "lockfile"`, `"pnpm-lock.yaml"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("BUILD file should contain %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"npm_link", "../", "x402stack", "projects", `":p_1"`} {
		if strings.Contains(string(out), unwanted) {
			t.Errorf("BUILD file should not contain %q:\n%s", unwanted, out)
		}
	}
}

// The root project's two layouts share one statement of the closure, so a
// repin changes one list and the trees cannot disagree.
func TestWriteBUILDStatesTheRootClosureOnce(t *testing.T) {
	// GIVEN a single-project tree with the hoisted twin requested
	dir := t.TempDir()
	path := filepath.Join(dir, "BUILD")
	plan := &generate.Plan{
		Entries:   []generate.Entry{{Target: "p_1", Package: "p", Version: "1.0.0"}},
		Closure:   map[string][]string{".": {"p_1"}},
		Direct:    map[string]map[string]string{},
		Workspace: map[string]map[string]string{},
	}

	// WHEN the BUILD file is written
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", nil, generate.Scope{}, true, false); err != nil {
		t.Fatal(err)
	}

	// THEN the closure label appears once, in CLOSURE, and both links use it
	out, _ := os.ReadFile(path)
	if n := strings.Count(string(out), `":p_1"`); n != 1 {
		t.Errorf("the closure should be stated once, found %d mentions of :p_1:\n%s", n, out)
	}
	if n := strings.Count(string(out), "packages = CLOSURE"); n != 2 {
		t.Errorf("both trees should reference CLOSURE, found %d:\n%s", n, out)
	}
	// AND a single-project tree gains no projects machinery
	if strings.Contains(string(out), "projects") {
		t.Errorf("a tree with no other project should be unchanged:\n%s", out)
	}
}

// For a BUILD file generated inside a build action, each rule carries the
// lockfile's own integrity, so nothing has to be downloaded to write it.
func TestWriteBUILDRecordsLockfileIntegrity(t *testing.T) {
	// GIVEN a plan whose entry carries the lockfile's integrity
	path := filepath.Join(t.TempDir(), "BUILD")
	plan := &generate.Plan{
		Entries:   []generate.Entry{{Target: "ms_2.1.3", Package: "ms", Version: "2.1.3", Integrity: "sha512-abc=="}},
		Closure:   map[string][]string{},
		Direct:    map[string]map[string]string{},
		Workspace: map[string]map[string]string{},
	}

	// WHEN it is written in integrity mode
	if err := generate.WriteBUILD(path, plan, "///js//build_defs:npm", "lock", nil, generate.Scope{}, false, true); err != nil {
		t.Fatal(err)
	}

	// THEN the rule carries the integrity and no download hash
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), `integrity = "sha512-abc=="`) || strings.Contains(string(out), "hashes") {
		t.Errorf("expected the lockfile integrity and no hashes:\n%s", out)
	}
}
