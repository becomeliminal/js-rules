// Package hooks runs the install scripts a package brought with it.
//
// npm calls these lifecycle hooks, and they are the reason a large part of the
// ecosystem works at all: a package that compiles native code, downloads a
// binary or generates a file does it here. They are also arbitrary code from a
// third party, executed because the package asked, which is how a supply chain
// attack succeeds. So nothing here decides whether to run anything -- the rule
// does, from an allowlist someone wrote down, and this package only knows how.
package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Order is npm's, and it is not alphabetical or arbitrary: preinstall runs
// before the package is considered installed, install replaces the default
// build step, and postinstall runs once the package is in place. A package that
// declares several relies on that sequence.
var Order = []string{"preinstall", "install", "postinstall"}

// Script is one hook: the name npm knows it by and the command it runs.
type Script struct {
	Name    string
	Command string
}

// Read returns the hooks a package declares, in the order npm runs them.
//
// A package with no scripts, or no manifest at all, has none -- that is the
// common case and not an error. Most of the registry declares nothing.
func Read(dir string) ([]Script, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing %s/package.json: %w", dir, err)
	}

	var out []Script
	for _, name := range Order {
		if cmd, ok := manifest.Scripts[name]; ok && cmd != "" {
			out = append(out, Script{Name: name, Command: cmd})
		}
	}
	return out, nil
}

// Run executes a package's hooks in order, in the package directory.
//
// env is the whole environment, not an addition to it: a hook sees what it was
// given and nothing else, so it cannot quietly depend on the machine it built
// on. PATH is the caller's business -- an install script that shells out to
// node expects to find it, because npm puts it there.
//
// A failing hook stops the sequence. npm does the same, and the alternative is
// a package that reports success while half-installed.
func Run(dir string, env []string, out io.Writer) error {
	scripts, err := Read(dir)
	if err != nil {
		return err
	}
	for _, s := range scripts {
		fmt.Fprintf(out, "%s: %s\n", s.Name, s.Command)
		cmd := exec.Command("sh", "-c", s.Command)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s script failed: %w", s.Name, err)
		}
	}
	return nil
}
