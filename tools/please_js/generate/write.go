package generate

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/please-build/buildtools/build"
)

// WriteBUILD emits the third_party BUILD file for a plan.
//
// Emission goes through the same AST Please parses with, so the output is
// stable against plz fmt and a regenerated file diffs only where the lockfile
// actually changed.
func WriteBUILD(path string, plan *Plan, subincludePath string, sums []string) error {
	f := &build.File{Path: path, Type: build.TypeBuild}

	f.Stmt = append(f.Stmt, &build.CallExpr{
		X:    &build.Ident{Name: "subinclude"},
		List: []build.Expr{&build.StringExpr{Value: subincludePath}},
	})

	for i, e := range plan.Entries {
		call := &build.CallExpr{X: &build.Ident{Name: "npm_repo"}, ForceMultiLine: true}
		str(call, "name", e.Target)
		if e.Package != e.Target {
			str(call, "pkg", e.Package)
		}
		str(call, "version", e.Version)

		// Only emit a key when it differs from the default the rule derives,
		// so a lockfile without peer resolution produces a quiet BUILD file.
		if e.Key != e.Package+"@"+e.Version {
			str(call, "key", e.Key)
		}
		if len(e.Deps) > 0 {
			dict(call, "deps", targetLabels(e.Deps))
		}
		if i < len(sums) && sums[i] != "" {
			list(call, "hashes", []string{sums[i]})
		}
		// Executables are deliberately not recorded here. The lockfile's
		// hasBin says only that some exist; the names and paths live in the
		// package's own manifest, which npm_repo reads at build time.
		f.Stmt = append(f.Stmt, call)
	}

	// One link target per pnpm workspace project. The closure is emitted in
	// full rather than derived at build time: a store entry reachable only
	// through another package's dep symlink still has to be staged, and the
	// alternatives for deriving it either miss those entries or reintroduce
	// the exported_deps hash oscillation.
	for _, path := range sortedKeys(plan.Closure) {
		closure := plan.Closure[path]
		if len(closure) == 0 {
			continue
		}
		call := &build.CallExpr{X: &build.Ident{Name: "npm_link"}, ForceMultiLine: true}
		str(call, "name", linkName(path))
		list(call, "store", labels(closure))
		dict(call, "packages", targetLabels(plan.Direct[path]))
		list(call, "visibility", []string{"PUBLIC"})
		f.Stmt = append(f.Stmt, call)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, build.Format(f), 0o644)
}

// linkName names the tree for a workspace project. The root project gets the
// bare name, so the common single-project case reads as node_modules.
func linkName(importerPath string) string {
	if importerPath == "." || importerPath == "" {
		return "node_modules"
	}
	return "node_modules_" + sanitise(importerPath)
}

func labels(targets []string) []string {
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = ":" + t
	}
	return out
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func targetLabels(deps map[string]string) map[string]string {
	out := make(map[string]string, len(deps))
	for alias, target := range deps {
		out[alias] = ":" + target
	}
	return out
}

func str(call *build.CallExpr, name, value string) {
	call.List = append(call.List, &build.AssignExpr{
		LHS: &build.Ident{Name: name}, Op: "=",
		RHS: &build.StringExpr{Value: value},
	})
}

func list(call *build.CallExpr, name string, values []string) {
	if len(values) == 0 {
		return
	}
	exprs := make([]build.Expr, len(values))
	for i, v := range values {
		exprs[i] = &build.StringExpr{Value: v}
	}
	call.List = append(call.List, &build.AssignExpr{
		LHS: &build.Ident{Name: name}, Op: "=",
		RHS: &build.ListExpr{List: exprs, ForceMultiLine: len(values) > 1},
	})
}

func dict(call *build.CallExpr, name string, m map[string]string) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]*build.KeyValueExpr, len(keys))
	for i, k := range keys {
		entries[i] = &build.KeyValueExpr{
			Key:   &build.StringExpr{Value: k},
			Value: &build.StringExpr{Value: m[k]},
		}
	}
	call.List = append(call.List, &build.AssignExpr{
		LHS: &build.Ident{Name: name}, Op: "=",
		RHS: &build.DictExpr{List: entries, ForceMultiLine: len(entries) > 1},
	})
}
