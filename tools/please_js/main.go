// please_js turns a pnpm lockfile into Please build targets.
//
// It replaces pnpm the way please_go replaces `go build` and please_rust
// replaces cargo: Please already owns the dependency graph, so the tool's job
// is to translate a resolved lockfile into that graph, not to resolve anything
// itself. It never runs node -- node is a runtime, not a compiler, and nothing
// here needs JavaScript executed.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thought-machine/go-flags"

	"tools/please_js/generate"
	"tools/please_js/hooks"
	"tools/please_js/junit"
	"tools/please_js/lockfile"
	"tools/please_js/store"
)

var opts = struct {
	Usage string

	Update struct {
		Lockfile       string `short:"l" long:"lockfile" default:"pnpm-lock.yaml" description:"pnpm-lock.yaml to translate"`
		Out            string `short:"o" long:"out" default:"third_party/js/BUILD" description:"BUILD file to write"`
		Subinclude     string `long:"subinclude" default:"///js//build_defs:npm" description:"subinclude path for the generated file"`
		LockLabel      string `long:"lock-label" default:"//:pnpm-lock.yaml" description:"label the generated npm_link rules read the lockfile from"`
		Registry       string `long:"registry" default:"https://registry.npmjs.org" description:"npm registry"`
		Workers        int    `long:"workers" default:"8" description:"concurrent downloads while hashing"`
		NoDev          bool     `long:"no-dev" description:"leave devDependencies out of the tree"`
		NoOptional     bool     `long:"no-optional" description:"leave optionalDependencies out of the tree"`
		LifecycleHooks []string `long:"lifecycle-hooks" description:"a package whose own install scripts may run; repeatable, and nothing runs without it"`
		SkipHashes     bool   `long:"skip-hashes" description:"do not fetch tarballs to record hashes; the result is unverified"`
	} `command:"update" description:"Translate a pnpm lockfile into npm_repo targets"`

	Describe struct {
		Name    string   `long:"name" required:"true" description:"this entry's identity, its target name"`
		Package string   `long:"package" required:"true" description:"npm package name"`
		Version string   `long:"version" description:"exact version"`
		OS      []string `long:"os" description:"operating systems this package supports; empty means any"`
		CPU     []string `long:"cpu" description:"architectures this package supports; empty means any"`
		Types   string   `long:"types" description:"declarations entry, for the generated package.json"`
		Bin     []string `long:"bin" description:"an executable the package's own manifest omits, as name=path"`
		Set     []string `long:"set" description:"an extra package.json field, as key=value"`
		Main    string   `long:"main" description:"entry file, for the generated package.json of a first-party library"`
		SrcDir   string   `long:"src-dir" description:"repo-relative directory holding this library's sources"`
		SrcEntry string   `long:"src-entry" description:"the entry as written, e.g. index.ts"`
		Src      []string `long:"src" description:"a source file, relative to src-dir; repeatable"`
		Dir     string   `long:"dir" description:"the fetched package directory"`
		Out     string   `long:"out" required:"true" description:"description file to write"`
	} `command:"describe" description:"Record what a fetched package is, for npm_link to assemble"`

	Link struct {
		Hoist      []string `long:"hoist" description:"a package exposed at the top level although nothing links it there; the escape hatch for a package that imports what it never declared"`
		NoDev      bool     `long:"no-dev" description:"leave devDependencies out; must match how the build file was generated"`
		NoOptional bool     `long:"no-optional" description:"leave optionalDependencies out; must match how the build file was generated"`
		Hoisted    bool     `long:"hoisted" description:"write npm's layout rather than pnpm's: no symlinks, resolution by walking up"`
		Workspace  []string `long:"workspace" description:"a workspace package this repo builds, as npm-name:directory"`
		Lockfile string   `long:"lock" required:"true" description:"the pnpm lockfile describing the graph"`
		Project  string   `long:"project" default:"." description:"which pnpm workspace project's tree to build"`
		Source   []string `long:"source" description:"a staged package, as metadata-path:package-dir"`
		Out      string   `long:"out" required:"true" description:"node_modules root to build"`
	} `command:"link" description:"Assemble a node_modules tree from staged packages"`

	Overlay struct {
		Tree string   `long:"tree" required:"true" description:"an existing node_modules tree"`
		Lib  []string `long:"lib" description:"a first-party library, as metadata-path:directory"`
		Out  string   `long:"out" required:"true" description:"node_modules root to write"`
		Dev  bool     `long:"dev" description:"record libraries with sources for devlink instead of copying their built output"`
	} `command:"overlay" description:"Add first-party libraries to a node_modules tree"`

	Devlink struct {
		Tree string `long:"tree" required:"true" description:"the node_modules tree to write packages into"`
		Root string `long:"root" required:"true" description:"the repository root the symlinks point into"`
		Spec string `long:"spec" required:"true" description:"the devlinks.json overlay --dev wrote"`
	} `command:"devlink" description:"Serve first-party packages from their sources"`

	Packages struct {
		Dir string `long:"dir" default:"." description:"directory to scan for lib.json"`
	} `command:"packages" description:"List the first-party package names staged under a directory"`

	Hooks struct {
		Dir  string   `long:"dir" required:"true" description:"the fetched package"`
		Env  []string `long:"env" description:"an environment entry, as KEY=value; the hook sees these and nothing else"`
		List bool     `long:"list" description:"print what the package declares without running it"`
	} `command:"hooks" description:"Run the install scripts a package brought with it"`

	Publish struct {
		Dir     string   `long:"dir" required:"true" description:"the built package"`
		Version string   `long:"version" required:"true" description:"the version to publish as"`
		Set     []string `long:"set" description:"a package.json field, as key=value; JSON values are kept as JSON"`
	} `command:"publish" description:"Patch a built package's manifest for release"`

	JUnit struct {
		In    string `long:"in" required:"true" description:"results as node's runner wrote them"`
		Out   string `long:"out" required:"true" description:"results for Please to read"`
		Suite string `long:"suite" default:"test" description:"name for the suite loose tests are put in"`
	} `command:"junit" description:"Make node's test results readable by Please"`

	ResolveBin struct {
		Tree    string `long:"tree" required:"true" description:"a node_modules tree"`
		Package string `long:"package" required:"true" description:"the package publishing the executable"`
		Bin     string `long:"bin" description:"which executable; needed only when the package publishes several"`
	} `command:"resolve-bin" description:"Print the path to an executable a package publishes"`
}{
	Usage: `
please_js translates a pnpm lockfile into Please targets.

  update    pnpm-lock.yaml -> one npm_repo target per resolved package

A lockfile's integrity field is sha512 and Please verifies sha1 or sha256, so
update fetches each tarball once: it checks the sha512 against the lockfile,
which is the supply-chain guarantee, and records the sha256 for Please to
enforce on every later fetch. Pass --skip-hashes to skip that, at the cost of
unverified downloads.
`,
}

func main() {
	parser := flags.NewParser(&opts, flags.HelpFlag|flags.PassDoubleDash)
	_, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Println(err)
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if parser.Active == nil {
		fmt.Fprintln(os.Stderr, "no command given; try --help")
		os.Exit(1)
	}

	run := map[string]func() error{
		"update":      update,
		"describe":    describe,
		"link":        link,
		"overlay":     overlay,
		"devlink":     devlink,
		"packages":    listPackages,
		"hooks":       runHooks,
		"junit":       convertJUnit,
		"publish":     publish,
		"resolve-bin": resolveBin,
	}[parser.Active.Name]
	if run == nil {
		fmt.Fprintf(os.Stderr, "unknown command %q\n", parser.Active.Name)
		os.Exit(1)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "please_js: %v\n", err)
		os.Exit(1)
	}
}

// runHooks executes what a package declared, and nothing decides here whether
// it should: the rule passes an allowlisted package or it does not call this at
// all. PATH is deliberately carried through from the caller -- an install
// script that shells out to node expects to find it, because npm puts it there.
func runHooks() error {
	if opts.Hooks.List {
		scripts, err := hooks.Read(opts.Hooks.Dir)
		if err != nil {
			return err
		}
		for _, s := range scripts {
			fmt.Printf("%s\t%s\n", s.Name, s.Command)
		}
		return nil
	}
	env := append([]string{"PATH=" + os.Getenv("PATH")}, opts.Hooks.Env...)
	return hooks.Run(opts.Hooks.Dir, env, os.Stdout)
}

// publish patches rather than regenerates, so the exports map the library
// already produced survives intact -- getting that wrong makes a package either
// unimportable or untyped, and neither failure happens until someone installs it.
func publish() error {
	fields := map[string]any{}
	for _, kv := range opts.Publish.Set {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("--set %q is not key=value", kv)
		}
		// A value that parses as JSON is kept as JSON, so repository and
		// keywords can be objects and lists rather than strings that look like
		// them. Anything else is a plain string.
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			fields[k] = parsed
		} else {
			fields[k] = v
		}
	}
	return store.Publish(opts.Publish.Dir, opts.Publish.Version, fields)
}

// convertJUnit exists because node writes a <testcase> directly under
// <testsuites> for any test not inside a describe, and Please reads only the
// ones inside a <testsuite>. Most test files have no describe.
func convertJUnit() error {
	return junit.Convert(opts.JUnit.In, opts.JUnit.Out, opts.JUnit.Suite)
}

func update() error {
	lock, err := lockfile.Parse(opts.Update.Lockfile)
	if err != nil {
		return err
	}

	plan, err := generate.Build(lock, generate.Scope{
		NoDev:      opts.Update.NoDev,
		NoOptional: opts.Update.NoOptional,
	})
	if err != nil {
		return err
	}

	// An allowlist, and only an allowlist. A package declaring install scripts
	// is not a reason to run them; someone naming the package is. Naming one
	// that declares none is worth saying out loud rather than ignoring, because
	// it usually means a typo or a package that changed.
	if err := plan.AllowHooks(opts.Update.LifecycleHooks); err != nil {
		return err
	}

	var sums []string
	if !opts.Update.SkipHashes {
		h := &generate.Hasher{Registry: opts.Update.Registry, Workers: opts.Update.Workers}
		sums, err = h.Resolve(plan.Entries, func(done, total int) {
			fmt.Fprintf(os.Stderr, "\rhashing %d/%d", done, total)
		})
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
	}

	if err := generate.WriteBUILD(opts.Update.Out, plan, opts.Update.Subinclude, opts.Update.LockLabel, sums, generate.Scope{NoDev: opts.Update.NoDev, NoOptional: opts.Update.NoOptional}); err != nil {
		return err
	}

	// Reported here because update runs at the repo root, where the stray tree
	// is visible; a build action is sandboxed and cannot see it.
	if warning := generate.StrayModules("."); warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	fmt.Fprintf(os.Stderr, "wrote %s: %d packages\n", opts.Update.Out, len(plan.Entries))
	for path, closure := range plan.Closure {
		label := path
		if label == "." {
			label = "(root)"
		}
		fmt.Fprintf(os.Stderr, "  %s: %d direct, %d in closure\n",
			label, len(plan.Direct[path]), len(closure))
	}
	return nil
}

// describe records what a fetched package is, so npm_link can assemble a tree
// from a set of them without re-deriving anything from the directory layout.
func describe() error {
	o := opts.Describe

	// A package whose constraints exclude this platform is described but not
	// placed. Every fast JavaScript tool now ships as a small wrapper plus one
	// native binary per platform -- TypeScript 7 has twenty -- so fetching the
	// whole set would mean hundreds of megabytes of compilers that cannot run.
	if !generate.HostPlatform().Supports(o.OS, o.CPU) {
		return store.WriteMeta(o.Out, store.Meta{
			Name: o.Name, Package: o.Package, Version: o.Version, Unsupported: true,
		})
	}

	// Declared executables are patched into the package's manifest before it is
	// read, so everything downstream sees one story. A package that ships a
	// runnable file and never declares it cannot otherwise be run, because
	// nothing here runs the install script that would have made the link.
	if o.Dir != "" && len(o.Bin) > 0 {
		declared := map[string]string{}
		for _, kv := range o.Bin {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("--bin %q is not name=path", kv)
			}
			declared[k] = v
		}
		if err := store.DeclareBins(o.Dir, declared); err != nil {
			return err
		}
	}

	// Executables come from the package's own manifest, which is where npm puts
	// them -- the lockfile only records that some exist.
	var bins map[string]string
	if o.Dir != "" {
		var err error
		if bins, err = store.ReadBins(o.Dir, o.Package); err != nil {
			return err
		}
	}

	extra := map[string]any{}
	for _, kv := range o.Set {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("--set %q is not key=value", kv)
		}
		extra[k] = v
	}

	// A first-party library needs a manifest for node to resolve it as a
	// directory. It is generated here rather than authored, the way please_go
	// writes an importconfig rather than asking for one.
	if o.Main != "" && o.Dir != "" {
		if err := store.WritePackageJSON(
			filepath.Join(o.Dir, "package.json"), o.Package, o.Main, o.Types, extra); err != nil {
			return err
		}
	}

	return store.WriteMeta(o.Out, store.Meta{
		Name:     o.Name,
		Package:  o.Package,
		Version:  o.Version,
		Bins:     bins,
		SrcDir:   o.SrcDir,
		SrcEntry: o.SrcEntry,
		Srcs:     o.Src,
	})
}

// link assembles a node_modules tree.
//
// The graph comes from the lockfile rather than from the rules: which packages
// a package needs, and the names it imports them under, are already recorded
// there, so npm_repo does not restate them and there is nothing to keep in
// sync. It also means an npm alias needs no special rule -- it is simply a
// reference whose name differs from the package's own.
func link() error {
	lock, err := lockfile.Parse(opts.Link.Lockfile)
	if err != nil {
		return err
	}
	// The same scope the build file was generated with. A tree assembled from a
	// wider closure than its package list would fail on a missing entry; a
	// narrower one would quietly stage packages nobody asked for.
	plan, err := generate.Build(lock, generate.Scope{
		NoDev:      opts.Link.NoDev,
		NoOptional: opts.Link.NoOptional,
	})
	if err != nil {
		return err
	}

	if _, ok := plan.Direct[opts.Link.Project]; !ok {
		return fmt.Errorf("%s has no project %q; it has %s",
			opts.Link.Lockfile, opts.Link.Project, strings.Join(projects(plan), ", "))
	}
	refs := plan.Refs()

	var sources []store.Source
	for _, s := range opts.Link.Source {
		metaPath, dir, ok := strings.Cut(s, ":")
		if !ok {
			return fmt.Errorf("--source %q is not metadata-path:package-dir", s)
		}
		meta, err := store.ReadMeta(metaPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", metaPath, err)
		}
		sources = append(sources, store.Source{
			Dir: dir, Meta: meta, Deps: refs[meta.Name],
			Origin: "the lockfile, as " + meta.Package + "@" + meta.Version,
		})
	}

	// Workspace packages are the repo's own, resolved through the lockfile as
	// link: rather than fetched. They enter the tree the same way a fetched
	// package does, so nothing downstream can tell them apart -- which is the
	// point: source code imports "@scope/thing" without knowing where it came
	// from.
	links := plan.Links(opts.Link.Project)
	provided := map[string]bool{}
	for _, w := range opts.Link.Workspace {
		name, dir, ok := strings.Cut(w, ":")
		if !ok {
			return fmt.Errorf("--workspace %q is not npm-name:directory", w)
		}
		sources = append(sources, store.Source{
			Dir:    dir,
			Meta:   store.Meta{Name: name, Package: name},
			Deps:   refs[name],
			Origin: "this repo, via workspace = {\"" + name + "\": ...}",
		})
		links = append(links, store.Ref{As: name, Entry: name})
		provided[name] = true
	}

	// A link: the lockfile records and nobody supplied would dangle, and a
	// dangling symlink fails at import time rather than here. Say so now.
	var missing []string
	for name := range plan.Workspace[opts.Link.Project] {
		if !provided[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		noun := "it"
		if len(missing) > 1 {
			noun = "them"
		}
		return fmt.Errorf("%s resolves %s through the workspace, and no target was given for %s; "+
			"pass workspace = {\"<name>\": \"//path/to:target\"}",
			opts.Link.Lockfile, strings.Join(missing, ", "), noun)
	}

	// The escape hatch for a package that imports what it never declared: its
	// victim is exposed at the top level, where the broken import's walk up the
	// tree will find it. Named by import name rather than target, because the
	// name is what the broken package writes -- but a name carried by two
	// resolutions is ambiguous, and picking one silently is how the wrong
	// version reaches production, so that is refused with both candidates.
	for _, name := range opts.Link.Hoist {
		var candidates []string
		for _, s := range sources {
			if s.Meta.Package == name && !s.Meta.Unsupported {
				candidates = append(candidates, s.Meta.Name)
			}
		}
		switch len(candidates) {
		case 0:
			return fmt.Errorf("--hoist %s names nothing in this tree; hoisting only exposes a package that is already in the closure", name)
		case 1:
			links = append(links, store.Ref{As: name, Entry: candidates[0]})
		default:
			sort.Strings(candidates)
			return fmt.Errorf("--hoist %s is ambiguous: %s all carry that name; this needs resolving in the lockfile",
				name, strings.Join(candidates, ", "))
		}
	}

	layout := store.Store
	if opts.Link.Hoisted {
		layout = store.Hoisted
	}
	return store.Build(opts.Link.Out, sources, links, layout)
}

func projects(plan *generate.Plan) []string {
	out := make([]string, 0, len(plan.Direct))
	for p := range plan.Direct {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// overlay adds first-party libraries to a third-party tree.
//
// The two are kept apart because they are resolved differently: third-party
// packages come from the lockfile and live in the store, where two versions can
// coexist; a first-party library is named by its location and has nothing to
// disambiguate.
func overlay() error {
	if err := copyTreeInto(opts.Overlay.Tree, opts.Overlay.Out); err != nil {
		return err
	}
	var libs []store.Source
	for _, l := range opts.Overlay.Lib {
		metaPath, dir, ok := strings.Cut(l, ":")
		if !ok {
			return fmt.Errorf("--lib %q is not metadata-path:directory", l)
		}
		meta, err := store.ReadMeta(metaPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", metaPath, err)
		}
		libs = append(libs, store.Source{Dir: dir, Meta: meta})
	}
	if !opts.Overlay.Dev {
		return store.Overlay(opts.Overlay.Out, libs)
	}

	// Development: a library whose lib.json records sources is served from
	// them, so nothing of it is copied here -- devlink builds it at run time.
	// One without source info still gets its built output, which is correct,
	// just not hot.
	var built []store.Source
	var links []store.DevLink
	for _, lib := range libs {
		if lib.Meta.SrcDir == "" {
			built = append(built, lib)
			continue
		}
		links = append(links, store.DevLink{
			Package:  lib.Meta.Package,
			SrcDir:   lib.Meta.SrcDir,
			SrcEntry: lib.Meta.SrcEntry,
			Srcs:     lib.Meta.Srcs,
		})
	}
	if err := store.Overlay(opts.Overlay.Out, built); err != nil {
		return err
	}
	return store.WriteDevLinks(filepath.Join(filepath.Dir(opts.Overlay.Out), "devlinks.json"), links)
}

func devlink() error {
	return store.Devlink(opts.Devlink.Tree, opts.Devlink.Root, opts.Devlink.Spec)
}

func listPackages() error {
	names, err := store.Packages(opts.Packages.Dir)
	if err != nil {
		return err
	}
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

func copyTreeInto(src, dst string) error {
	if src == "" {
		return os.MkdirAll(dst, 0o755)
	}
	return store.CopyTree(src, dst)
}

func resolveBin() error {
	path, err := store.ResolveBin(opts.ResolveBin.Tree, opts.ResolveBin.Package, opts.ResolveBin.Bin)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}
