package resolve

import (
	"fmt"
	"os"
)

// Args holds the arguments for the resolve subcommand.
type Args struct {
	Lockfile       string
	Out            string
	NoDev          bool
	SubincludePath string
}

// Run executes the resolve subcommand.
func Run(args Args) error {
	lock, err := parseLockfile(args.Lockfile)
	if err != nil {
		return err
	}

	// Collect all top-level packages (skip root, nested, dev) and version-conflict targets
	packages, conflictTargets := collectPackages(lock.Packages, args.NoDev)
	breakCycles(packages, conflictTargets)

	// Generate output directory
	if err := os.MkdirAll(args.Out, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write .plzconfig with plugin declaration
	if err := writePlzConfig(args.Out); err != nil {
		return fmt.Errorf("failed to write .plzconfig: %w", err)
	}

	// Generate BUILD files with explicit subinclude
	for _, pkg := range packages {
		if err := writeBuildFile(args.Out, pkg, args.SubincludePath); err != nil {
			return fmt.Errorf("failed to write BUILD for %s: %w", pkg.Name, err)
		}
	}

	// Append version-conflict targets to existing BUILD files
	for _, ct := range conflictTargets {
		if err := appendConflictTarget(args.Out, ct); err != nil {
			return fmt.Errorf("failed to write conflict target %s: %w", ct.TargetName, err)
		}
	}

	total := len(packages) + len(conflictTargets)
	fmt.Fprintf(os.Stderr, "Generated %d npm_module rules (%d version-conflict targets)\n", total, len(conflictTargets))
	return nil
}
