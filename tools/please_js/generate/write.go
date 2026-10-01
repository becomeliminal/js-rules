package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/please-build/buildtools/build"
)

// WriteBUILD emits the third_party BUILD file for a plan.
//
// Emission goes through the same AST Please parses with, so the output is
// stable against plz fmt and a regenerated file diffs only where the lockfile
// actually changed.
// hoistedLink additionally emits each tree in npm's hoisted layout, under the
// name "hoisted" -- the layout a development server needs, since a store of
// symlinks and a server told to preserve them cannot coexist.
func WriteBUILD(path string, plan *Plan, subincludePath, lockLabel string, sums []string, scope Scope, hoistedLink bool) error {
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
		// The lockfile's exact URL wins; a scope registry is next; silence
		// means the default. Emitted so the build file works without the flags
		// that generated it.
		if e.URL != "" {
			str(call, "url", e.URL)
		} else if e.Registry != "" {
			str(call, "registry", e.Registry)
		}
		if e.RunHooks {
			call.List = append(call.List, &build.AssignExpr{
				LHS: &build.Ident{Name: "run_hooks"},
				Op:  "=",
				RHS: &build.Ident{Name: "True"},
			})
		}

		// Platform constraints are emitted so the same build file works
		// everywhere: the rule decides what to fetch, not the generator.
		if len(e.OS) > 0 {
			list(call, "os", e.OS)
		}
		if len(e.CPU) > 0 {
			list(call, "cpu", e.CPU)
		}

		// No dependencies here. Which packages a package needs, and the names
		// it imports them under, are in the lockfile, and npm_link reads it --
		// so this file stays as small as a go_repo call.
		if i < len(sums) && sums[i] != "" {
			list(call, "hashes", []string{sums[i]})
		}
		// Executables are deliberately not recorded here. The lockfile's
		// hasBin says only that some exist; the names and paths live in the
		// package's own manifest, which npm_repo reads at build time.
		f.Stmt = append(f.Stmt, call)
	}

	// The root project's trees, linked here because the root project IS this
	// package. The closure is emitted in full rather than derived at build
	// time: a store entry reachable only through another package's dep symlink
	// still has to be staged, and the alternatives for deriving it either miss
	// those entries or reintroduce the exported_deps hash oscillation.
	//
	// Every other workspace project is linked where its package.json lives, by
	// npm_project, from the data WriteProjects emits -- so no closure for a
	// project elsewhere in the repo is written into this file at all.
	if closure := plan.Closure["."]; len(closure) > 0 {
		packages := labels(closure)
		var ref build.Expr = listExpr(packages)
		if hoistedLink {
			// Stated once and referenced by both trees, so a repin's diff
			// touches one list and the two layouts cannot disagree.
			f.Stmt = append(f.Stmt, &build.AssignExpr{
				LHS: &build.Ident{Name: "CLOSURE"},
				Op:  "=",
				RHS: listExpr(packages),
			})
			ref = &build.Ident{Name: "CLOSURE"}
		}
		f.Stmt = append(f.Stmt, linkCall("node_modules", lockLabel, ref, scope, false))
		if hoistedLink {
			f.Stmt = append(f.Stmt, linkCall("hoisted", lockLabel, ref, scope, true))
		}
	}

	// What npm_project in another package needs to reach this tree: the data
	// file, and the lockfile, which a rule elsewhere can only name through a
	// target. Emitted only when there is such a project, so a single-project
	// tree's file is unchanged.
	if len(projectPaths(plan)) > 0 {
		f.Stmt = append(f.Stmt,
			exportCall("projects", ProjectsFile),
			exportCall("lockfile", lockLabel),
		)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, build.Format(f), 0o644)
}

// ProjectsFile is the generated data file npm_project loads, beside the
// generated BUILD file.
const ProjectsFile = "projects.build_defs"

// linkCall is one npm_link over the root project's closure.
func linkCall(name, lockLabel string, packages build.Expr, scope Scope, hoisted bool) *build.CallExpr {
	call := &build.CallExpr{X: &build.Ident{Name: "npm_link"}, ForceMultiLine: true}
	str(call, "name", name)
	// Emitted onto the rule as well as used here: the link step recomputes the
	// closure and has to reach the same answer, or it fails on an entry the
	// package list does not contain. Policy flags carry onto both layouts: a
	// hoisted tree that silently kept devDependencies would stage what the
	// other tree refused.
	for _, f := range []struct {
		name string
		on   bool
	}{{"no_dev", scope.NoDev}, {"no_optional", scope.NoOptional}, {"hoisted", hoisted}} {
		if f.on {
			call.List = append(call.List, &build.AssignExpr{
				LHS: &build.Ident{Name: f.name},
				Op:  "=",
				RHS: &build.Ident{Name: "True"},
			})
		}
	}
	str(call, "lock", lockLabel)
	call.List = append(call.List, &build.AssignExpr{
		LHS: &build.Ident{Name: "packages"},
		Op:  "=",
		RHS: packages,
	})
	list(call, "visibility", []string{"PUBLIC"})
	return call
}

func exportCall(name, src string) *build.CallExpr {
	call := &build.CallExpr{X: &build.Ident{Name: "filegroup"}, ForceMultiLine: true}
	str(call, "name", name)
	list(call, "srcs", []string{src})
	list(call, "visibility", []string{"PUBLIC"})
	return call
}

func listExpr(items []string) *build.ListExpr {
	l := &build.ListExpr{ForceMultiLine: true}
	for _, s := range items {
		l.List = append(l.List, &build.StringExpr{Value: s})
	}
	return l
}

// projectPaths is every workspace project other than the root that has
// anything to link, sorted.
func projectPaths(plan *Plan) []string {
	var out []string
	for _, path := range sortedKeys(plan.Closure) {
		if path != "." && path != "" && len(plan.Closure[path]) > 0 {
			out = append(out, path)
		}
	}
	return out
}

// WriteProjects emits the data npm_project reads, for every workspace project
// other than the root: keyed by the project's path from the repository root,
// which is exactly what package_name() returns in the package that holds its
// package.json -- so a project finds itself with no argument at all.
//
// The lockfile's own key for the project (relative to the workspace, so often
// ../../something) is carried as data for the link step and never appears in
// a file anyone writes.
//
// workspaceDir is the lockfile's directory relative to the repository root.
// Writes nothing, and removes a stale file, when there is no such project.
func WriteProjects(path string, plan *Plan, workspaceDir string, scope Scope) error {
	paths := projectPaths(plan)
	if len(paths) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	boolean := func(b bool) build.Expr {
		if b {
			return &build.Ident{Name: "True"}
		}
		return &build.Ident{Name: "False"}
	}

	projects := &build.DictExpr{ForceMultiLine: true}
	for _, key := range paths {
		repoPath := filepath.ToSlash(filepath.Clean(filepath.Join(workspaceDir, key)))
		if repoPath == ".." || strings.HasPrefix(repoPath, "../") {
			return fmt.Errorf("workspace project %q resolves to %s, outside the repository", key, repoPath)
		}
		entry := &build.DictExpr{ForceMultiLine: true}
		for _, kv := range []struct {
			k string
			v build.Expr
		}{
			{"project", &build.StringExpr{Value: key}},
			{"no_dev", boolean(scope.NoDev)},
			{"no_optional", boolean(scope.NoOptional)},
			// Bare target names: npm_project qualifies them with the tree's
			// label, so the data does not care where the tree lives.
			{"packages", listExpr(plan.Closure[key])},
		} {
			entry.List = append(entry.List, &build.KeyValueExpr{Key: &build.StringExpr{Value: kv.k}, Value: kv.v})
		}
		projects.List = append(projects.List, &build.KeyValueExpr{Key: &build.StringExpr{Value: repoPath}, Value: entry})
	}

	assign := &build.AssignExpr{LHS: &build.Ident{Name: "NPM_PROJECTS"}, Op: "=", RHS: projects}
	assign.Comments.Before = []build.Comment{
		{Token: "# Generated by please_js update from the workspace lockfile. Do not edit:"},
		{Token: "# regenerate with this tree's npm_update target. Read by npm_project."},
	}
	f := &build.File{Path: path, Type: build.TypeDefault, Stmt: []build.Expr{assign}}
	return os.WriteFile(path, build.Format(f), 0o644)
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

func sortedValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
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

