// Package tsconfig validates that a rule's mirrored compiler flags agree with
// the tsconfig they shadow. Every mirrored flag is a chance to disagree
// silently: the build passes --rootDir on the command line, the command line
// wins, and a rootDir in the tsconfig is ignored -- so the editor, which reads
// only the config, emits one layout in its head while the build emits another.
// The wrong emitted path is import paths inside node_modules, which is much
// worse than an error.
package tsconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

type showConfig struct {
	CompilerOptions struct {
		RootDir *string `json:"rootDir"`
		OutDir  *string `json:"outDir"`
	} `json:"compilerOptions"`
}

// Check reads `tsc --showConfig` output and compares the paths the config
// resolved against the ones the rule passed. showConfig normalises every
// inherited path relative to the -p config's own directory -- verified against
// tsc 5.9: a rootDir declared in an extended base two directories away comes
// out as "../base/src" -- so resolving against the config's directory is
// correct for a whole extends chain, not just a lone file.
func Check(showConfigJSON []byte, configPath, root string) error {
	var cfg showConfig
	if err := json.Unmarshal(showConfigJSON, &cfg); err != nil {
		return fmt.Errorf("tsconfig-check: parsing --showConfig output: %w", err)
	}
	dir := filepath.Dir(configPath)
	if root == "" {
		root = "."
	}
	if cfg.CompilerOptions.RootDir != nil {
		got := filepath.Clean(filepath.Join(dir, *cfg.CompilerOptions.RootDir))
		want := filepath.Clean(root)
		if got != want {
			return fmt.Errorf("the tsconfig resolves rootDir to %s, but the rule compiles with --rootDir %s, and the command line wins.\n"+
				"Emitted paths -- which become import paths inside node_modules -- follow the rule; the editor follows the config; they disagree.\n"+
				"Set the rule's `root` to match, or remove rootDir from the config chain", got, want)
		}
	}
	if cfg.CompilerOptions.OutDir != nil {
		return fmt.Errorf("the tsconfig sets outDir (%s), but the rule owns the output directory and overrides it on the command line.\n"+
			"Nothing is ever written there; remove outDir from the config chain rather than trusting a setting the build ignores",
			*cfg.CompilerOptions.OutDir)
	}
	return nil
}
