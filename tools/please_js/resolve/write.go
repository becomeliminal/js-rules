package resolve

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/please-build/buildtools/build"

	"tools/please_js/common"
)

// writePlzConfig writes the .plzconfig for the subrepo.
func writePlzConfig(outDir string) error {
	content := "[Plugin \"js\"]\nTarget=@//plugins:js\n"
	return os.WriteFile(filepath.Join(outDir, ".plzconfig"), []byte(content), 0644)
}

// writeBuildFile generates a BUILD file for a single npm package using
// the buildtools AST for correct formatting.
func writeBuildFile(outDir string, pkg resolvedPackage, subincludePath string) error {
	pkgDir := filepath.Join(outDir, pkg.Name)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return err
	}

	f := &build.File{
		Path: filepath.Join(pkgDir, "BUILD"),
		Type: build.TypeBuild,
	}

	// subinclude(...)
	f.Stmt = append(f.Stmt, &build.CallExpr{
		X:    &build.Ident{Name: "subinclude"},
		List: []build.Expr{&build.StringExpr{Value: subincludePath}},
	})

	// npm_module(...)
	call := &build.CallExpr{
		X:              &build.Ident{Name: "npm_module"},
		ForceMultiLine: true,
	}

	targetName := pkg.targetName()
	addStringArg(call, "name", targetName)

	pkgName := pkg.effectivePkgName()
	if targetName != pkgName || pkg.RealName != "" {
		addStringArg(call, "pkg_name", pkgName)
	}
	addStringArg(call, "version", pkg.Version)

	if len(pkg.Deps) > 0 {
		depTargets := make([]string, len(pkg.Deps))
		for i, dep := range pkg.Deps {
			depTargets[i] = common.DepTarget(dep)
		}
		addListArg(call, "deps", depTargets)
	}

	if len(pkg.NestedDeps) > 0 {
		addDictArg(call, "nested_deps", pkg.NestedDeps)
	}

	if pkg.Dev {
		addListArg(call, "labels", []string{"npm:dev"})
	}

	addListArg(call, "visibility", []string{"PUBLIC"})

	f.Stmt = append(f.Stmt, call)

	return os.WriteFile(f.Path, build.Format(f), 0644)
}

// appendConflictTarget appends a version-conflict npm_module target to an
// existing BUILD file using the buildtools AST.
func appendConflictTarget(outDir string, ct conflictTarget) error {
	buildPath := filepath.Join(outDir, ct.Dir, "BUILD")
	data, err := os.ReadFile(buildPath)
	if err != nil {
		return err
	}

	f, err := build.ParseBuild(buildPath, data)
	if err != nil {
		return err
	}

	call := &build.CallExpr{
		X:              &build.Ident{Name: "npm_module"},
		ForceMultiLine: true,
	}
	addStringArg(call, "name", ct.TargetName)
	addStringArg(call, "pkg_name", ct.PkgName)
	addStringArg(call, "version", ct.Version)

	if len(ct.Deps) > 0 {
		depTargets := make([]string, len(ct.Deps))
		for i, dep := range ct.Deps {
			depTargets[i] = common.DepTarget(dep)
		}
		addListArg(call, "deps", depTargets)
	}

	addListArg(call, "visibility", []string{"PUBLIC"})

	f.Stmt = append(f.Stmt, call)

	return os.WriteFile(buildPath, build.Format(f), 0644)
}

// addStringArg appends a named string argument to a CallExpr.
func addStringArg(call *build.CallExpr, name, value string) {
	call.List = append(call.List, &build.AssignExpr{
		LHS: &build.Ident{Name: name},
		Op:  "=",
		RHS: &build.StringExpr{Value: value},
	})
}

// addListArg appends a named string list argument to a CallExpr.
func addListArg(call *build.CallExpr, name string, values []string) {
	if len(values) == 0 {
		return
	}
	exprs := make([]build.Expr, len(values))
	for i, v := range values {
		exprs[i] = &build.StringExpr{Value: v}
	}
	call.List = append(call.List, &build.AssignExpr{
		LHS: &build.Ident{Name: name},
		Op:  "=",
		RHS: &build.ListExpr{
			List:           exprs,
			ForceMultiLine: len(values) > 1,
		},
	})
}

// addDictArg appends a named string dict argument to a CallExpr.
func addDictArg(call *build.CallExpr, name string, m map[string]string) {
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
		LHS: &build.Ident{Name: name},
		Op:  "=",
		RHS: &build.DictExpr{
			List:           entries,
			ForceMultiLine: len(entries) > 1,
		},
	})
}
