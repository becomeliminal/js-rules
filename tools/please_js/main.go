// please_js turns a pnpm lockfile into Please build targets.
//
// It replaces pnpm the way please_go replaces `go build` and please_rust
// replaces cargo: Please already owns the dependency graph, so the tool's job
// is to translate a resolved lockfile into that graph, not to resolve anything
// itself. It never runs node -- node is a runtime, not a compiler, and nothing
// here needs JavaScript executed.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/thought-machine/go-flags"

	"tools/please_js/generate"
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
		SkipHashes     bool   `long:"skip-hashes" description:"do not fetch tarballs to record hashes; the result is unverified"`
	} `command:"update" description:"Translate a pnpm lockfile into npm_repo targets"`

	Describe struct {
		Name    string   `long:"name" required:"true" description:"this entry's identity, its target name"`
		Package string   `long:"package" required:"true" description:"npm package name"`
		Version string   `long:"version" description:"exact version"`
		Dir     string   `long:"dir" description:"the fetched package directory"`
		Out     string   `long:"out" required:"true" description:"description file to write"`
	} `command:"describe" description:"Record what a fetched package is, for npm_link to assemble"`

	Link struct {
		Lockfile string   `long:"lock" required:"true" description:"the pnpm lockfile describing the graph"`
		Project  string   `long:"project" default:"." description:"which pnpm workspace project's tree to build"`
		Source   []string `long:"source" description:"a staged package, as metadata-path:package-dir"`
		Out      string   `long:"out" required:"true" description:"node_modules root to build"`
	} `command:"link" description:"Assemble a node_modules tree from staged packages"`
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
		"update":   update,
		"describe": describe,
		"link":     link,
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

func update() error {
	lock, err := lockfile.Parse(opts.Update.Lockfile)
	if err != nil {
		return err
	}

	plan, err := generate.Build(lock)
	if err != nil {
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

	if err := generate.WriteBUILD(opts.Update.Out, plan, opts.Update.Subinclude, opts.Update.LockLabel, sums); err != nil {
		return err
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

	// Executables come from the package's own manifest, which is where npm puts
	// them -- the lockfile only records that some exist.
	var bins map[string]string
	if o.Dir != "" {
		var err error
		if bins, err = store.ReadBins(o.Dir, o.Package); err != nil {
			return err
		}
	}

	return store.WriteMeta(o.Out, store.Meta{
		Name:    o.Name,
		Package: o.Package,
		Version: o.Version,
		Bins:    bins,
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
	plan, err := generate.Build(lock)
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
		sources = append(sources, store.Source{Dir: dir, Meta: meta, Deps: refs[meta.Name]})
	}

	return store.Build(opts.Link.Out, sources, plan.Links(opts.Link.Project))
}

func projects(plan *generate.Plan) []string {
	out := make([]string, 0, len(plan.Direct))
	for p := range plan.Direct {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
