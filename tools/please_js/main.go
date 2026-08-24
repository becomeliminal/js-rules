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

	"github.com/thought-machine/go-flags"

	"tools/please_js/generate"
	"tools/please_js/lockfile"
)

var opts = struct {
	Usage string

	Update struct {
		Lockfile       string `short:"l" long:"lockfile" default:"pnpm-lock.yaml" description:"pnpm-lock.yaml to translate"`
		Out            string `short:"o" long:"out" default:"third_party/js/BUILD" description:"BUILD file to write"`
		Subinclude     string `long:"subinclude" default:"///js//build_defs:npm" description:"subinclude path for the generated file"`
		Registry       string `long:"registry" default:"https://registry.npmjs.org" description:"npm registry"`
		Workers        int    `long:"workers" default:"8" description:"concurrent downloads while hashing"`
		SkipHashes     bool   `long:"skip-hashes" description:"do not fetch tarballs to record hashes; the result is unverified"`
	} `command:"update" description:"Translate a pnpm lockfile into npm_repo targets"`
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
	cmd, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Println(err)
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cmd == nil {
		fmt.Fprintln(os.Stderr, "no command given; try --help")
		os.Exit(1)
	}

	if err := update(); err != nil {
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

	if err := generate.WriteBUILD(opts.Update.Out, plan, opts.Update.Subinclude, sums); err != nil {
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
