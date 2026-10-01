package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StrayModules reports a node_modules directory that node would search after
// the tree Please builds.
//
// Node resolves by walking up the filesystem, and a staged action sits inside
// the repo, so the repo's own node_modules is on that path -- measured, not
// assumed:
//
//	plz-out/tmp/<pkg>/<target>._build/node_modules   the staged tree
//	...
//	<repo>/node_modules                              a stray one
//
// A declared dependency still resolves from the staged tree, which is nearer.
// The problem is an undeclared one: it falls through, finds the stray tree, and
// the build passes here and nowhere else. That is the failure worth naming,
// because nothing about it looks wrong locally.
//
// A warning rather than an error. A checked-out node_modules is how editors
// offer completions, so plenty of people have one on purpose, and refusing to
// work would be wrong. Returns an empty string when there is nothing to say.
func StrayModules(root string) string {
	path := filepath.Join(root, "node_modules")
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return fmt.Sprintf(
		"%s exists, and node searches it after the tree Please builds.\n"+
			"  A declared dependency still resolves from the built tree, which is nearer.\n"+
			"  An undeclared one resolves from here instead, so the build passes on this\n"+
			"  machine and fails on any other. Remove it, or keep it knowing that.",
		path)
}

// IgnoredPnpmField reports a `pnpm` field in a tree's package.json.
//
// pnpm 10 moved workspace settings -- packageExtensions, overrides, patches --
// to pnpm-workspace.yaml, and the pnpm this toolchain drives no longer reads
// them from package.json. The failure mode is the quiet kind: the field sits
// there looking like an applied fix while the resolution it was meant to
// change proceeds without it. Found the hard way, twenty minutes at a time.
//
// A warning rather than an error, because pnpm itself accepts the file; it is
// only the settings that go nowhere. Returns an empty string when there is
// nothing to say.
func IgnoredPnpmField(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var manifest map[string]json.RawMessage
	if json.Unmarshal(data, &manifest) != nil {
		return ""
	}
	if _, ok := manifest["pnpm"]; !ok {
		return ""
	}
	return fmt.Sprintf(
		"%s has a \"pnpm\" field, which this pnpm does not read.\n"+
			"  Workspace settings (packageExtensions, overrides, patchedDependencies, ...)\n"+
			"  live in pnpm-workspace.yaml since pnpm 10; the field in package.json is\n"+
			"  silently ignored, so whatever it was meant to change has not changed.",
		filepath.Join(dir, "package.json"))
}
