package store_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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

// Deduplicating against the top level is wrong when an ancestor shadows it.
// The shape that found this in production (@noble/hashes, LIM-3704):
//
//	app -> hashes@2, account@1, ox@2, curves@2   (all take the top level)
//	account -> hashes@1 (nests), ox@1 (nests)
//	ox@1 -> curves@1 (nests, "curves" is taken)
//	curves@1 -> hashes@2
//
// curves@1 sits at account/node_modules/ox/node_modules/curves. A walk up
// from there meets account/node_modules/hashes -- which is hashes@1 -- before
// the top-level hashes@2 it deduplicated against. The placer has to notice
// the interception and nest a copy of hashes@2 beside curves@1.
func TestHoistedNestsThroughAShadowingAncestor(t *testing.T) {
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
	sources := []store.Source{
		src("app_1", "app",
			store.Ref{As: "@noble/hashes", Entry: "hashes_2"},
			store.Ref{As: "account", Entry: "account_1"},
			store.Ref{As: "ox", Entry: "ox_2"},
			store.Ref{As: "curves", Entry: "curves_2"}),
		src("hashes_2", "@noble/hashes"),
		src("ox_2", "ox"),
		src("curves_2", "curves"),
		src("account_1", "account",
			store.Ref{As: "@noble/hashes", Entry: "hashes_1"},
			store.Ref{As: "ox", Entry: "ox_1"}),
		src("hashes_1", "@noble/hashes"),
		src("ox_1", "ox", store.Ref{As: "curves", Entry: "curves_1"}),
		src("curves_1", "curves", store.Ref{As: "@noble/hashes", Entry: "hashes_2"}),
	}
	root := filepath.Join(dir, "out")
	if err := store.Build(root, sources, []store.Ref{{As: "app", Entry: "app_1"}}, store.Hoisted); err != nil {
		t.Fatal(err)
	}

	// The walk node does from curves@1 must find hashes@2, not the hashes@1
	// account nests one level up. Asserted by walking, not by a fixed path:
	// where the fix copy lands is the placer's business, what it resolves to
	// is the contract.
	var curves string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && filepath.Base(p) == "index.js" {
			if data, _ := os.ReadFile(p); string(data) == "// curves_1" {
				curves = filepath.Dir(p)
			}
		}
		return nil
	})
	if curves == "" {
		t.Fatal("curves@1 was not placed")
	}
	got := ""
	for dir := curves; strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		if data, err := os.ReadFile(filepath.Join(dir, "node_modules", "@noble", "hashes", "index.js")); err == nil {
			got = string(data)
			break
		}
	}
	if got == "" {
		data, _ := os.ReadFile(filepath.Join(root, "@noble", "hashes", "index.js"))
		got = string(data)
	}
	if got != "// hashes_2" {
		t.Errorf("curves@1 resolves @noble/hashes to %q, want hashes_2", got)
	}

	// The shadow itself is untouched: account still finds its own hashes@1.
	data, _ := os.ReadFile(filepath.Join(root, "account", "node_modules", "@noble", "hashes", "index.js"))
	if string(data) != "// hashes_1" {
		t.Errorf("account's own resolution is %q, want hashes_1", string(data))
	}
}

// A family of packages nested under one parent, all needing a version of a
// name whose top-level slot holds another version, share ONE copy at the
// highest free spot -- npm's rule -- instead of each member nesting its own.
// The real case: two generations of the Solana SDK in one tree, where nesting
// beside each dependent copied one package 597 times (6.9 GB, 885k files).
func TestHoistedSharesAConflictingVersionAcrossSiblings(t *testing.T) {
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

	// GIVEN app -> types@7 (takes the top) and program, and program -> five
	// family members that each need types@5 and each other
	members := []string{"m1", "m2", "m3", "m4", "m5"}
	var sources []store.Source
	var programDeps []store.Ref
	for _, m := range members {
		deps := []store.Ref{{As: "types", Entry: "types_5"}}
		for _, other := range members {
			if other != m {
				deps = append(deps, store.Ref{As: other, Entry: other + "_5"})
			}
		}
		sources = append(sources, src(m+"_5", m, deps...))
		programDeps = append(programDeps, store.Ref{As: m, Entry: m + "_5"})
	}
	sources = append(sources,
		src("app_1", "app", store.Ref{As: "types", Entry: "types_7"}, store.Ref{As: "program", Entry: "program_1"}),
		src("program_1", "program", programDeps...),
		src("types_7", "types"),
		src("types_5", "types"),
	)
	// The members also exist at the top under other versions, so they nest too.
	for _, m := range members {
		sources = append(sources, src(m+"_7", m))
	}
	links := []store.Ref{{As: "app", Entry: "app_1"}}
	for _, m := range members {
		links = append(links, store.Ref{As: m, Entry: m + "_7"})
	}
	root := filepath.Join(dir, "out")

	// WHEN the hoisted tree is built
	if err := store.Build(root, sources, links, store.Hoisted); err != nil {
		t.Fatal(err)
	}

	// THEN types@5 exists once, shared, not once per member
	var copies int
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && filepath.Base(p) == "index.js" {
			if data, _ := os.ReadFile(p); string(data) == "// types_5" {
				copies++
			}
		}
		return nil
	})
	if copies != 1 {
		t.Errorf("types@5 should be placed once and shared by the family, found %d copies", copies)
	}
}

// A version placed high must not land in the node_modules of a package that
// needs a different version itself: that package resolves its own slot first.
// The real case: browserify-sign needs safe-buffer 5.2.1 while a stream
// nested under it needs 5.1.2.
func TestHoistedNeverShadowsTheOwnerOfASlot(t *testing.T) {
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

	// GIVEN app -> sb@2, signer, stream@3; signer -> sb@2, stream@2 (nests,
	// stream@3 has the top); stream@2 -> sb@1
	sources := []store.Source{
		src("app_1", "app",
			store.Ref{As: "sb", Entry: "sb_2"},
			store.Ref{As: "signer", Entry: "signer_1"},
			store.Ref{As: "stream", Entry: "stream_3"}),
		src("signer_1", "signer",
			store.Ref{As: "sb", Entry: "sb_2"},
			store.Ref{As: "stream", Entry: "stream_2"}),
		src("stream_2", "stream", store.Ref{As: "sb", Entry: "sb_1"}),
		src("stream_3", "stream"),
		src("sb_1", "sb"),
		src("sb_2", "sb"),
	}
	root := filepath.Join(dir, "out")

	// WHEN the hoisted tree is built
	err := store.Build(root, sources, []store.Ref{
		{As: "sb", Entry: "sb_2"}, {As: "signer", Entry: "signer_1"}, {As: "stream", Entry: "stream_3"},
	}, store.Hoisted)

	// THEN it builds, signer still finds sb@2, and the nested stream finds sb@1
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(from, name string) string {
		for d := from; ; d = filepath.Dir(d) {
			if data, err := os.ReadFile(filepath.Join(d, "node_modules", name, "index.js")); err == nil {
				return string(data)
			}
			if d == root {
				data, _ := os.ReadFile(filepath.Join(root, name, "index.js"))
				return string(data)
			}
		}
	}
	if got := resolve(filepath.Join(root, "signer"), "sb"); got != "// sb_2" {
		t.Errorf("signer resolves sb to %q, want sb_2", got)
	}
	if got := resolve(filepath.Join(root, "signer", "node_modules", "stream"), "sb"); got != "// sb_1" {
		t.Errorf("the nested stream resolves sb to %q, want sb_1", got)
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

// A first-party package is materialised as its sources: a manifest whose entry
// is the source as written, and one symlink per file into the repository.
func TestLinkServesLibrarySources(t *testing.T) {
	dir := t.TempDir()
	rundir := filepath.Join(dir, "run")
	root := filepath.Join(dir, "repo")
	writeFiles(t, filepath.Join(root, "lib/greeter"), "index.ts", "deep/util.ts")

	spec := filepath.Join(dir, "links.json")
	writeLinks(t, spec, store.LinkSet{
		Into: "node_modules/@test/greeter", From: "lib/greeter",
		Srcs: []string{"index.ts", "deep/util.ts"}, Live: store.LiveDirs([]string{"index.ts", "deep/util.ts"}),
		Package: "@test/greeter", SrcEntry: "index.ts",
	})
	if err := store.Link(rundir, root, spec); err != nil {
		t.Fatal(err)
	}

	// The manifest's entry is the source as written -- index.ts, not a
	// compiled index.js -- because the server transforms what it serves.
	data, err := os.ReadFile(filepath.Join(rundir, "node_modules/@test/greeter/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"./index.ts"`) {
		t.Errorf("the entry should be the source, got:\n%s", data)
	}
	assertLinks(t, filepath.Join(rundir, "node_modules/@test/greeter"), filepath.Join(root, "lib/greeter"),
		"deep/util.ts", "index.ts")
}

// Rebuilt from nothing every start, so a source removed from the library stops
// being served rather than lingering as a link nobody declared.
func TestLinkRebuildsLibraryFromNothing(t *testing.T) {
	dir := t.TempDir()
	rundir := filepath.Join(dir, "run")
	root := filepath.Join(dir, "repo")
	writeFiles(t, filepath.Join(root, "lib"), "index.js", "old.js")
	spec := filepath.Join(dir, "links.json")
	link := func(srcs ...string) {
		writeLinks(t, spec, store.LinkSet{
			Into: "node_modules/lib", From: "lib", Srcs: srcs, Package: "lib", SrcEntry: "index.js",
		})
		if err := store.Link(rundir, root, spec); err != nil {
			t.Fatal(err)
		}
	}

	link("index.js", "old.js")
	link("index.js")

	assertLinks(t, filepath.Join(rundir, "node_modules/lib"), filepath.Join(root, "lib"), "index.js")
}

// The live rule: a top-level directory holding a declared source is linked as
// it is now, so a file -- or a whole directory of files -- created after the
// build is served without one. Files at the package's top level are linked only
// as declared: that is where BUILD, package.json, dist/ and node_modules live.
func TestLinkMirrorsLiveDirectories(t *testing.T) {
	dir := t.TempDir()
	rundir := filepath.Join(dir, "run")
	root := filepath.Join(dir, "repo")
	declared := []string{"index.html", "src", "src/main.tsx", "public/logo.svg"}
	writeFiles(t, filepath.Join(root, "app"),
		"index.html", "src/main.tsx", "public/logo.svg",
		// Created after the build: none of these were declared.
		"src/New.tsx", "src/feature/Panel.tsx", "public/new.png",
		// Never source, wherever it sits.
		"src/node_modules/x/index.js", "src/.DS_Store", "src/main.tsx~", "src/.main.tsx.swp",
		// Package top level, undeclared.
		"BUILD", "package.json", "dist/index.js",
	)

	spec := filepath.Join(dir, "links.json")
	writeLinks(t, spec, store.LinkSet{Into: ".", From: "app", Srcs: declared, Live: store.LiveDirs(declared)})
	if err := store.Link(rundir, root, spec); err != nil {
		t.Fatal(err)
	}

	assertLinks(t, rundir, filepath.Join(root, "app"),
		"index.html", "public/logo.svg", "public/new.png",
		"src/New.tsx", "src/feature/Panel.tsx", "src/main.tsx")
}

// A file deleted since the last start stops being served, but only links into
// the package are touched: the third-party tree linked into the same run
// directory, and the real files the build put there, survive.
func TestLinkRemovesStaleLinksOnly(t *testing.T) {
	dir := t.TempDir()
	rundir := filepath.Join(dir, "run")
	root := filepath.Join(dir, "repo")
	writeFiles(t, filepath.Join(root, "app"), "src/main.tsx", "src/gone.tsx")
	writeFiles(t, rundir, "vite.config.ts")
	writeFiles(t, filepath.Join(dir, "store"), "react/index.js")
	os.MkdirAll(filepath.Join(rundir, "node_modules"), 0o755)
	if err := os.Symlink(filepath.Join(dir, "store/react"), filepath.Join(rundir, "node_modules/react")); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(dir, "links.json")
	declared := []string{"src/main.tsx", "src/gone.tsx"}
	writeLinks(t, spec, store.LinkSet{Into: ".", From: "app", Srcs: declared, Live: store.LiveDirs(declared)})
	if err := store.Link(rundir, root, spec); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(root, "app/src/gone.tsx")); err != nil {
		t.Fatal(err)
	}
	if err := store.Link(rundir, root, spec); err != nil {
		t.Fatal(err)
	}

	assertLinks(t, rundir, filepath.Join(root, "app"), "src/main.tsx")
	if info, err := os.Lstat(filepath.Join(rundir, "vite.config.ts")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("the copied config should be left a real file: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(rundir, "node_modules/react")); err != nil || target != filepath.Join(dir, "store/react") {
		t.Errorf("the third-party tree's link should survive, got %q, %v", target, err)
	}
}

func TestLiveDirs(t *testing.T) {
	got := store.LiveDirs([]string{"index.html", "src/a.ts", "src/b/c.ts", "public/x.png", "vite.config.ts"})
	want := []string{"public", "src"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LiveDirs = %v, want %v", got, want)
	}
}

func writeFiles(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeLinks(t *testing.T, path string, sets ...store.LinkSet) {
	t.Helper()
	if err := store.WriteLinks(path, store.LinkSpec{Sets: sets, Skip: store.DefaultSkip}); err != nil {
		t.Fatal(err)
	}
}

// assertLinks checks that the symlinks under dir are exactly want, each
// pointing at the same path under from, and that no directory is a link.
func assertLinks(t *testing.T, dir, from string, want ...string) {
	t.Helper()
	var got []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" && p != dir {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		target, _ := os.Readlink(p)
		if target != filepath.Join(from, rel) {
			t.Errorf("%s points at %s, want %s", rel, target, filepath.Join(from, rel))
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			t.Errorf("%s is a directory link", rel)
		}
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links = %v, want %v", got, want)
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

// Remote execution stages every input read-only. A library and its
// declarations twin are overlaid into one directory, the twin's manifest over
// the library's, so the library's copy has to be writable even though its
// source was not -- a ts_library depending on another failed only on RBE.
func TestOverlayMergesATwinOverReadOnlySources(t *testing.T) {
	dir := t.TempDir()
	write := func(d string, files map[string]string) string {
		p := filepath.Join(dir, d)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(p, name), []byte(body), 0o444); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	pkg := write("pkg", map[string]string{"package.json": `{"name":"@x/lib"}`, "index.js": "module.exports = 1;"})
	twin := write("twin", map[string]string{"package.json": `{"name":"@x/lib","types":"index.d.ts"}`, "index.d.ts": "export {};"})
	out := filepath.Join(dir, "node_modules")

	err := store.Overlay(out, []store.Source{
		{Dir: twin, Meta: store.Meta{Name: "lib_types", Package: "@x/lib", Role: "types"}},
		{Dir: pkg, Meta: store.Meta{Name: "lib", Package: "@x/lib"}},
	})
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(out, "@x/lib/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"types"`) {
		t.Errorf("the twin's manifest should win, got %s", manifest)
	}
	for _, f := range []string{"index.js", "index.d.ts"} {
		if _, err := os.Stat(filepath.Join(out, "@x/lib", f)); err != nil {
			t.Errorf("%s missing after the merge: %v", f, err)
		}
	}
}

// A first-party library is imported by subpath the way bundlers write it --
// `@x/lib/uuid`, no extension -- and "./*": "./*" maps that to a file named
// `uuid`, which does not exist: TypeScript and node both apply exports targets
// literally. So every module the package holds gets an extensionless key, a
// directory's index answers for the directory, and the declarations twin adds
// the types condition. The wildcard stays last for explicit paths and assets.
func TestSubpathsResolveWithoutTheirExtension(t *testing.T) {
	write := func(dir string, files ...string) {
		for _, f := range files {
			p := filepath.Join(dir, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	exportsOf := func(dir string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "package.json"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	pkg := t.TempDir()
	write(pkg, "index.js", "uuid.js", "machine/index.js", "machine/actions.js", "styles.css")
	if err := store.WritePackageJSON(filepath.Join(pkg, "package.json"), "@x/lib", "index.js", "", nil); err != nil {
		t.Fatal(err)
	}
	got := exportsOf(pkg)
	for _, want := range []string{
		`"./uuid": {
      "default": "./uuid.js"
    }`,
		`"./machine": {
      "default": "./machine/index.js"
    }`,
		`"./machine/actions": {
      "default": "./machine/actions.js"
    }`,
		`"./*": "./*"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"./styles"`) {
		t.Errorf("an asset is reached by its own name, through the wildcard:\n%s", got)
	}
	if strings.Index(got, `"./*"`) < strings.Index(got, `"./uuid"`) {
		t.Errorf("the wildcard must come after the explicit subpaths:\n%s", got)
	}

	twin := t.TempDir()
	write(twin, "index.d.ts", "uuid.d.ts")
	if err := store.WritePackageJSON(filepath.Join(twin, "package.json"), "@x/lib", "index.js", "index.d.ts", nil); err != nil {
		t.Fatal(err)
	}
	if want := `"./uuid": {
      "types": "./uuid.d.ts",
      "default": "./uuid.js"
    }`; !strings.Contains(exportsOf(twin), want) {
		t.Errorf("the twin's manifest -- the one that wins the merge -- needs both conditions, missing %s in:\n%s", want, exportsOf(twin))
	}
}

// node and the bundlers resolve `./a` to a.js before a/index.js, so a file
// outranks a directory's index for the same key, whichever is walked first.
func TestAFileOutranksADirectoryIndexOfTheSameName(t *testing.T) {
	pkg := t.TempDir()
	for _, f := range []string{"index.js", "machine.js", "machine/index.js"} {
		p := filepath.Join(pkg, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WritePackageJSON(filepath.Join(pkg, "package.json"), "@x/lib", "index.js", "", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(pkg, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"./machine": {
      "default": "./machine.js"
    }`; !strings.Contains(string(data), want) {
		t.Errorf("missing %s in:\n%s", want, data)
	}
}
