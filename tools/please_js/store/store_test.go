package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"tools/please_js/store"
)

func pkg(t *testing.T, root, entry, name, version string, deps ...store.Ref) store.Source {
	t.Helper()
	dir := filepath.Join(root, "src", entry)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"` + name + `","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return store.Source{
		Dir:  dir,
		Meta: store.Meta{Package: name, Version: version, Name: entry},
		Deps: deps,
	}
}

// ref names an entry as it appears in a node_modules directory.
func ref(as, entry string) store.Ref { return store.Ref{As: as, Entry: entry} }

// resolves follows every symlink. Asserting on link text would pass for a link
// that points somewhere plausible but wrong, which is the failure this package
// exists to prevent.
func resolves(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func TestScopedAndUnscopedLinksResolve(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "react_18_3_1", "react", "18.3.1"),
		// A scoped package sits inside a scope directory, so the store root is
		// one level further up than for an unscoped one.
		pkg(t, root, "scope_thing_1_0_0", "@scope/thing", "1.0.0"),
	}
	if err := store.Build(tree, sources, []store.Ref{
		ref("react", "react_18_3_1"),
		ref("@scope/thing", "scope_thing_1_0_0"),
	}, store.Store); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"react", "@scope/thing"} {
		if !resolves(t, filepath.Join(tree, name, "package.json")) {
			target, _ := os.Readlink(filepath.Join(tree, name))
			t.Errorf("%s does not resolve; link points at %q", name, target)
		}
	}
}

func TestDepsResolveFromInsideTheirPackage(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "react_dom_18_3_1", "react-dom", "18.3.1",
			ref("react", "react_18_3_1"), ref("@scope/thing", "scope_thing_1_0_0")),
		pkg(t, root, "react_18_3_1", "react", "18.3.1"),
		pkg(t, root, "scope_thing_1_0_0", "@scope/thing", "1.0.0"),
	}
	if err := store.Build(tree, sources, []store.Ref{ref("react-dom", "react_dom_18_3_1")}, store.Store); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(tree, store.StoreRoot, store.StoreDir("react_dom_18_3_1"), "node_modules")
	for _, dep := range []string{"react", "@scope/thing"} {
		if !resolves(t, filepath.Join(base, dep, "package.json")) {
			target, _ := os.Readlink(filepath.Join(base, dep))
			t.Errorf("react-dom cannot resolve %s; link points at %q", dep, target)
		}
	}
}

// npm's cycles are real and in core packages: @babel/core and
// @babel/helper-module-transforms require each other. Please rejects a cyclic
// build graph, so dependencies are names resolved here rather than build edges.
func TestCyclicDependenciesAreFine(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "babel_core", "@babel/core", "7.29.0",
			ref("@babel/helper-module-transforms", "babel_helper")),
		pkg(t, root, "babel_helper", "@babel/helper-module-transforms", "7.28.6",
			ref("@babel/core", "babel_core")),
	}
	if err := store.Build(tree, sources, []store.Ref{ref("@babel/core", "babel_core")}, store.Store); err != nil {
		t.Fatal(err)
	}
	both := [][2]string{
		{"babel_core", "@babel/helper-module-transforms"},
		{"babel_helper", "@babel/core"},
	}
	for _, c := range both {
		p := filepath.Join(tree, store.StoreRoot, store.StoreDir(c[0]), "node_modules", c[1], "package.json")
		if !resolves(t, p) {
			t.Errorf("%s cannot resolve %s", c[0], c[1])
		}
	}
}

// An npm alias rebinds a package under another name. It holds no files, so the
// package stays a singleton -- two copies would be two module instances, which
// is how "invalid hook call" happens.
func TestOnePackageUnderTwoNamesAtTwoVersions(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	sources := []store.Source{
		pkg(t, root, "aspect_c_2_0_2", "@aspect-test/c", "2.0.2"),
		pkg(t, root, "aspect_c_1_0_0", "@aspect-test/c", "1.0.0"),
	}
	// The lockfile binds one of them under another name; that is carried on the
	// reference rather than needing an entry of its own.
	if err := store.Build(tree, sources, []store.Ref{
		ref("@aspect-test/c", "aspect_c_2_0_2"),
		ref("@aspect-test/c1", "aspect_c_1_0_0"),
	}, store.Store); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"@aspect-test/c":  "2.0.2",
		"@aspect-test/c1": "1.0.0",
	} {
		data, err := os.ReadFile(filepath.Join(tree, name, "package.json"))
		if err != nil {
			t.Errorf("%s does not resolve: %v", name, err)
			continue
		}
		if !strings.Contains(string(data), `"version":"`+want+`"`) {
			t.Errorf("%s resolved to the wrong version: %s", name, data)
		}
	}
}

func TestMissingClosureMemberIsAnError(t *testing.T) {
	root := t.TempDir()
	sources := []store.Source{
		pkg(t, root, "react_dom", "react-dom", "18.3.1", ref("react", "react_18_3_1")),
	}
	err := store.Build(filepath.Join(root, "node_modules"), sources,
		[]store.Ref{ref("react-dom", "react_dom")}, store.Store)
	if err == nil {
		t.Fatal("a dangling dependency should fail the build, not produce a broken tree")
	}
	if !strings.Contains(err.Error(), "closure") {
		t.Errorf("the error should say what is missing and why; got: %v", err)
	}
}

// A package excluded by its own os/cpu constraints is described but never
// placed, and a dependency on it simply vanishes -- npm records these as
// optional precisely so a package copes with their absence. Twenty native
// compilers ship with TypeScript 7 and nineteen of them cannot run here.
func TestUnsupportedPlatformIsDescribedButNotPlaced(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "node_modules")

	wrapper := pkg(t, root, "typescript_7", "typescript", "7.0.2",
		ref("@ts/linux-x64", "ts_linux"), ref("@ts/darwin-arm64", "ts_darwin"))
	linux := pkg(t, root, "ts_linux", "@ts/linux-x64", "7.0.2")
	darwin := store.Source{Meta: store.Meta{
		Package: "@ts/darwin-arm64", Name: "ts_darwin", Unsupported: true,
	}}

	if err := store.Build(tree, []store.Source{wrapper, linux, darwin},
		[]store.Ref{ref("typescript", "typescript_7")}, store.Store); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(tree, store.StoreRoot, store.StoreDir("typescript_7"), "node_modules")
	if !resolves(t, filepath.Join(base, "@ts/linux-x64", "package.json")) {
		t.Error("the usable platform binary should be linked")
	}
	if _, err := os.Lstat(filepath.Join(base, "@ts/darwin-arm64")); err == nil {
		t.Error("a link was made to a package that was never fetched")
	}
	if _, err := os.Stat(filepath.Join(tree, store.StoreRoot, store.StoreDir("ts_darwin"))); err == nil {
		t.Error("an unsupported package took up a store entry")
	}
}

// CommonJS honours "main"; ESM does not. Without an "exports" map node refuses
// a directory import with ERR_UNSUPPORTED_DIR_IMPORT, so a library that emits
// modules cannot be imported by name at all.
func TestGeneratedManifestIsImportableAsESM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	if err := store.WritePackageJSON(path, "scope/thing", "index.js", "index.d.ts",
		map[string]any{"type": "module"}); err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	exports, ok := m["exports"].(map[string]any)
	if !ok {
		t.Fatal("no exports map; an ESM consumer cannot import this package by name")
	}
	root, ok := exports["."].(map[string]any)
	if !ok {
		t.Fatal(`exports has no "." entry`)
	}
	if root["default"] != "./index.js" {
		t.Errorf(`exports["."].default = %v, want ./index.js`, root["default"])
	}
	if root["types"] != "./index.d.ts" {
		t.Errorf(`exports["."].types = %v, want ./index.d.ts`, root["types"])
	}
	// exports is an allowlist, so declaring it without a wildcard would stop
	// every subpath import of the package from resolving.
	if exports["./*"] != "./*" {
		t.Error("no subpath wildcard; declaring exports would break pkg/sub imports")
	}
	if m["type"] != "module" {
		t.Errorf("extra fields should survive; type = %v", m["type"])
	}
}

// A Go map would sort "default" ahead of "types" and node would never reach the
// declarations, so assert the bytes rather than the decoded object -- decoding
// into a map is exactly the mistake this guards against.
func TestExportConditionsAreOrdered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	if err := store.WritePackageJSON(path, "@test/x", "index.js", "index.d.ts", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	types, def := strings.Index(got, `"types"`), strings.Index(got, `"default"`)
	if types < 0 || def < 0 {
		t.Fatalf("expected both conditions, got:\n%s", got)
	}
	if types > def {
		t.Errorf("\"default\" is the fallback and must come last, got:\n%s", got)
	}
}

// A package absent from the tree and a package publishing no executables both
// reach ResolveBin as "no bins", and the difference matters: the second is a
// mistake about the package, the first is nearly always the wrong tree.
func TestResolveBinSaysWhichMistakeWasMade(t *testing.T) {
	tree := t.TempDir()
	present := filepath.Join(tree, "quiet")
	if err := os.MkdirAll(present, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(present, "package.json"), []byte(`{"name":"quiet"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := store.ResolveBin(tree, "absent", "")
	if err == nil || !strings.Contains(err.Error(), "no package absent") {
		t.Errorf("a package not in the tree should say so, got: %v", err)
	}

	_, err = store.ResolveBin(tree, "quiet", "")
	if err == nil || !strings.Contains(err.Error(), "publishes no executables") {
		t.Errorf("a package with no bins should say so, got: %v", err)
	}
}

// A first-party package replacing a registry one is reasonable to want and
// terrible to get by accident, so it is an error naming both rather than a
// silent last-one-wins that depends on staging order.
func TestTwoPackagesWithOneNameNameBoth(t *testing.T) {
	dir := t.TempDir()
	src := func(origin string) store.Source {
		d := filepath.Join(dir, strings.ReplaceAll(origin, " ", "_"))
		os.MkdirAll(d, 0o755)
		return store.Source{Dir: d, Meta: store.Meta{Name: "clash", Package: "clash"}, Origin: origin}
	}
	err := store.Build(filepath.Join(dir, "out"),
		[]store.Source{src("the lockfile"), src("this repo")}, nil, store.Store)
	if err == nil {
		t.Fatal("expected a collision")
	}
	for _, want := range []string{"clash", "the lockfile", "this repo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// Some packages ship a runnable file and never declare it, because their own
// install script would have made the link. Nothing runs install scripts by
// default, so the declaration has to come from somewhere.
func TestDeclareBinsAddsToAManifest(t *testing.T) {
	dir := t.TempDir()
	write := func(manifest string) {
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]string {
		data, err := os.ReadFile(filepath.Join(dir, "package.json"))
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Bin map[string]string `json:"bin"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%v: %s", err, data)
		}
		return m.Bin
	}

	write(`{"name":"thing","version":"1.0.0"}`)
	if err := store.DeclareBins(dir, map[string]string{"thing": "cli.js"}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got["thing"] != "cli.js" {
		t.Errorf("got %v", got)
	}

	// npm allows a bare string, meaning one executable named after the package.
	// Merging into that has to widen it first, or the existing one is discarded.
	write(`{"name":"@scope/thing","bin":"./main.js"}`)
	if err := store.DeclareBins(dir, map[string]string{"extra": "extra.js"}); err != nil {
		t.Fatal(err)
	}
	got := read()
	if got["thing"] != "./main.js" {
		t.Errorf("the package's own executable was discarded: %v", got)
	}
	if got["extra"] != "extra.js" {
		t.Errorf("the declared executable is missing: %v", got)
	}
}

func TestDeclareBinsRefusesAManifestItCannotUnderstand(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"bin":42}`), 0o644)
	err := store.DeclareBins(dir, map[string]string{"a": "b"})
	if err == nil || !strings.Contains(err.Error(), "neither an object nor a string") {
		t.Errorf("got %v", err)
	}
}

// The hoisted layout is npm's: a package at the top level unless its name is
// taken, resolution by walking up, and not one symlink anywhere. It exists for
// tools that cannot follow symlinks without also resolving away paths that must
// stay as written.
func TestHoistedPutsEachNameWhereAWalkWillFindIt(t *testing.T) {
	dir := t.TempDir()
	pkg := func(name string) string {
		d := filepath.Join(dir, "src", strings.ReplaceAll(name, "/", "+"))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "index.js"), []byte("// "+name), 0o644)
		return d
	}
	src := func(entry, name string, deps ...store.Ref) store.Source {
		return store.Source{Dir: pkg(entry), Meta: store.Meta{Name: entry, Package: name}, Deps: deps}
	}

	// app -> shared@2, and app -> old, which needs shared@1. Exactly the case
	// the store exists for, and the one hoisting has to nest.
	sources := []store.Source{
		src("app_1", "app", store.Ref{As: "shared", Entry: "shared_2"}, store.Ref{As: "old", Entry: "old_1"}),
		src("shared_2", "shared"),
		src("old_1", "old", store.Ref{As: "shared", Entry: "shared_1"}),
		src("shared_1", "shared"),
	}
	root := filepath.Join(dir, "out")
	if err := store.Build(root, sources, []store.Ref{{As: "app", Entry: "app_1"}}, store.Hoisted); err != nil {
		t.Fatal(err)
	}

	// Not one symlink: that is the whole point, since the tool this is for
	// cannot follow them without also resolving away the sources.
	var links int
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			links++
		}
		return nil
	})
	if links != 0 {
		t.Errorf("a hoisted tree has no symlinks, found %d", links)
	}

	// The winner is at the top, where every package walking up will find it.
	for _, at := range []string{"app", "shared", "old"} {
		if _, err := os.Stat(filepath.Join(root, at, "index.js")); err != nil {
			t.Errorf("%s is not at the top level: %v", at, err)
		}
	}

	// The loser sits beside the only package that asks for it, which is what
	// makes a walk up from `old` find its own shared before the top-level one.
	nested := filepath.Join(root, "old", "node_modules", "shared", "index.js")
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("the conflicting resolution should sit beside its dependent: %v", err)
	}
}

// A package needed by two dependents that cannot see the hoisted one is copied
// to both. That is the entire cost of this layout, and it is worth knowing it
// is bounded by name conflicts rather than by dependents.
func TestHoistingCopiesOnlyWhereNamesCollide(t *testing.T) {
	dir := t.TempDir()
	pkg := func(name string) string {
		d := filepath.Join(dir, "src", name)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "index.js"), []byte("// "+name), 0o644)
		return d
	}
	src := func(entry, name string, deps ...store.Ref) store.Source {
		return store.Source{Dir: pkg(entry), Meta: store.Meta{Name: entry, Package: name}, Deps: deps}
	}
	// Three packages all depending on one shared version: one copy, not three.
	sources := []store.Source{
		src("a_1", "a", store.Ref{As: "dep", Entry: "dep_1"}),
		src("b_1", "b", store.Ref{As: "dep", Entry: "dep_1"}),
		src("c_1", "c", store.Ref{As: "dep", Entry: "dep_1"}),
		src("dep_1", "dep"),
	}
	root := filepath.Join(dir, "out")
	links := []store.Ref{{As: "a", Entry: "a_1"}, {As: "b", Entry: "b_1"}, {As: "c", Entry: "c_1"}}
	if err := store.Build(root, sources, links, store.Hoisted); err != nil {
		t.Fatal(err)
	}
	var copies int
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && filepath.Base(p) == "dep" {
			copies++
		}
		return nil
	})
	if copies != 1 {
		t.Errorf("one resolution, one copy; found %d", copies)
	}
}

// Devlink materialises a first-party package as its sources: a manifest whose
// entry is the source as written, and one symlink per file into the repository.
func TestDevlinkServesSources(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "node_modules")
	root := filepath.Join(dir, "repo")
	os.MkdirAll(tree, 0o755)

	spec := filepath.Join(dir, "devlinks.json")
	links := []store.DevLink{{
		Package:  "@test/greeter",
		SrcDir:   "lib/greeter",
		SrcEntry: "index.ts",
		Srcs:     []string{"index.ts", "deep/util.ts"},
	}}
	if err := store.WriteDevLinks(spec, links); err != nil {
		t.Fatal(err)
	}
	if err := store.Devlink(tree, root, spec); err != nil {
		t.Fatal(err)
	}

	// The manifest's entry is the source as written -- index.ts, not a
	// compiled index.js -- because the server transforms what it serves.
	data, err := os.ReadFile(filepath.Join(tree, "@test/greeter/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"./index.ts"`) {
		t.Errorf("the entry should be the source, got:\n%s", data)
	}

	// Each source is a link into the repository, nested paths included.
	for _, src := range []string{"index.ts", "deep/util.ts"} {
		at := filepath.Join(tree, "@test/greeter", src)
		target, err := os.Readlink(at)
		if err != nil {
			t.Fatalf("%s should be a symlink: %v", at, err)
		}
		want := filepath.Join(root, "lib/greeter", src)
		if target != want {
			t.Errorf("%s points at %s, want %s", src, target, want)
		}
	}
}

// Rebuilt from nothing every start, so a source removed from the library stops
// being served rather than lingering as a link nobody declared.
func TestDevlinkRebuildsFromNothing(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "node_modules")
	root := filepath.Join(dir, "repo")
	os.MkdirAll(tree, 0o755)

	spec := filepath.Join(dir, "devlinks.json")
	write := func(srcs ...string) {
		if err := store.WriteDevLinks(spec, []store.DevLink{{
			Package: "lib", SrcDir: "lib", SrcEntry: "index.js", Srcs: srcs,
		}}); err != nil {
			t.Fatal(err)
		}
		if err := store.Devlink(tree, root, spec); err != nil {
			t.Fatal(err)
		}
	}
	write("index.js", "old.js")
	write("index.js")
	if _, err := os.Lstat(filepath.Join(tree, "lib/old.js")); err == nil {
		t.Error("a removed source should stop being served")
	}
	if _, err := os.Lstat(filepath.Join(tree, "lib/index.js")); err != nil {
		t.Errorf("the surviving source should still be there: %v", err)
	}
}

// The names a development server's config needs, from the same lib.jsons the
// overlay reads -- knowable at build time even though the links are not
// buildable then.
func TestPackagesListsWhatIsStaged(t *testing.T) {
	dir := t.TempDir()
	put := func(rel, pkg string) {
		d := filepath.Join(dir, rel)
		os.MkdirAll(d, 0o755)
		if err := store.WriteMeta(filepath.Join(d, "lib.json"), store.Meta{Name: "x", Package: pkg}); err != nil {
			t.Fatal(err)
		}
	}
	put("a", "@test/a")
	put("b/nested", "@test/b")
	// Inside node_modules is a staged tree, not a first-party library.
	put("node_modules/decoy", "@test/decoy")

	got, err := store.Packages(dir)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "@test/a,@test/b" {
		t.Errorf("got %v", got)
	}
}
