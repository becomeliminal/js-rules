package resolve

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tools/please_js/common"
)

// Args holds the arguments for the resolve subcommand.
type Args struct {
	Lockfile       string
	Out            string
	NoDev          bool
	SubincludePath string
}

// packageLock represents the top-level structure of package-lock.json (v3).
type packageLock struct {
	LockfileVersion int                    `json:"lockfileVersion"`
	Packages        map[string]packageInfo `json:"packages"`
}

// peerDepMeta holds metadata for a single peer dependency entry.
type peerDepMeta struct {
	Optional bool `json:"optional"`
}

// packageInfo represents a single package entry in the lockfile.
type packageInfo struct {
	Version              string                    `json:"version"`
	Resolved             string                    `json:"resolved"`
	Integrity            string                    `json:"integrity"`
	Dependencies         map[string]string         `json:"dependencies"`
	PeerDependencies     map[string]string         `json:"peerDependencies"`
	PeerDependenciesMeta map[string]peerDepMeta    `json:"peerDependenciesMeta"`
	Dev                  bool                      `json:"dev"`
	Optional             bool                      `json:"optional"`
	OS                   []string                  `json:"os"`
	CPU                  []string                  `json:"cpu"`
}

// resolvedPackage is the processed form we use for BUILD file generation.
type resolvedPackage struct {
	Name       string            // npm package name or alias (e.g., "react", "my-ms")
	RealName   string            // real npm package name if aliased (e.g., "ms"); empty if not aliased
	Version    string
	Resolved   string            // tarball URL
	Deps       []string          // dependency package names (mapped to subrepo targets)
	Dev        bool              // true if this is a dev-only package
	NestedDeps map[string]string // import_name -> subrepo target for version-conflict deps
}

// conflictTarget represents an additional npm_module target for a specific
// version of a package that conflicts with the top-level version.
type conflictTarget struct {
	Dir        string   // subrepo directory path (e.g., "zod", "@types/react")
	TargetName string   // version-qualified name (e.g., "zod_v4_3_6")
	PkgName    string   // real npm package name
	Version    string
	Deps       []string // dependency package names
}

// Run executes the resolve subcommand.
func Run(args Args) error {
	data, err := os.ReadFile(args.Lockfile)
	if err != nil {
		return fmt.Errorf("failed to read lockfile: %w", err)
	}

	var lock packageLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("failed to parse lockfile: %w", err)
	}

	if lock.LockfileVersion != 3 && lock.LockfileVersion != 2 {
		return fmt.Errorf("unsupported lockfile version %d (expected 2 or 3)", lock.LockfileVersion)
	}

	// Collect all top-level packages (skip root, nested, dev) and version-conflict targets
	packages, conflictTargets := collectPackages(lock.Packages, args.NoDev)
	breakCycles(packages)

	// Generate output directory
	if err := os.MkdirAll(args.Out, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	subincludePath := args.SubincludePath

	// Write .plzconfig with plugin declaration
	if err := writePlzConfig(args.Out); err != nil {
		return fmt.Errorf("failed to write .plzconfig: %w", err)
	}

	// Generate BUILD files with explicit subinclude
	for _, pkg := range packages {
		if err := writeBuildFile(args.Out, pkg, subincludePath); err != nil {
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

// breakCycles detects and removes back-edges in the dependency graph via DFS,
// ensuring the resulting graph is a DAG suitable for Please's build system.
func breakCycles(packages []resolvedPackage) {
	idx := make(map[string]int, len(packages))
	for i, pkg := range packages {
		idx[pkg.Name] = i
	}

	// DFS coloring: 0=white, 1=gray (in stack), 2=black (done)
	color := make(map[string]int, len(packages))

	var dfs func(name string)
	dfs = func(name string) {
		color[name] = 1
		i := idx[name]
		var kept []string
		for _, dep := range packages[i].Deps {
			if color[dep] == 1 {
				log.Printf("warning: breaking circular dependency: %s -> %s", name, dep)
				continue
			}
			kept = append(kept, dep)
			if color[dep] == 0 {
				dfs(dep)
			}
		}
		packages[i].Deps = kept
		color[name] = 2
	}

	for _, pkg := range packages {
		if color[pkg.Name] == 0 {
			dfs(pkg.Name)
		}
	}
}

// collectPackages extracts top-level packages from the lockfile and detects
// version conflicts. It returns regular packages (including promoted nested-only
// packages) and version-conflict targets for packages that exist at multiple versions.
func collectPackages(pkgs map[string]packageInfo, noDev bool) ([]resolvedPackage, []conflictTarget) {
	// Phase 1: Build set of top-level package names and their versions
	topLevel := make(map[string]bool)
	topLevelVersions := make(map[string]string)
	for path, info := range pkgs {
		if path == "" || common.IsNestedPackage(path) {
			continue
		}
		name := common.ExtractPackageName(path)
		if name == "" {
			continue
		}
		topLevel[name] = true
		topLevelVersions[name] = info.Version
	}

	// Phase 2: Promote nested-only packages (exist ONLY as nested, no top-level)
	promoted := make(map[string]string) // name -> lockfile path
	for path := range pkgs {
		if path == "" || !common.IsNestedPackage(path) {
			continue
		}
		name := common.ExtractPackageName(path)
		if name == "" || topLevel[name] {
			continue
		}
		if _, already := promoted[name]; already {
			continue
		}
		promoted[name] = path
		topLevel[name] = true
	}

	// Phase 3: Detect version conflicts (nested package version differs from top-level)
	type parentConflict struct {
		ParentName string
		DepName    string
		Version    string
	}
	var conflicts []parentConflict
	conflictVersionInfos := make(map[string]map[string]packageInfo) // depName -> version -> info

	for path, info := range pkgs {
		if path == "" || !common.IsNestedPackage(path) {
			continue
		}
		name := common.ExtractPackageName(path)
		if name == "" {
			continue
		}
		// Skip promoted packages (they're treated as top-level)
		if promoted[name] == path {
			continue
		}
		// Only detect conflicts where a different top-level version exists
		topVer, exists := topLevelVersions[name]
		if !exists || info.Version == topVer {
			continue
		}
		if info.Resolved == "" {
			continue
		}

		parentPath := common.ExtractParentPackagePath(path)
		parentName := common.ExtractPackageName(parentPath)
		if parentName == "" {
			continue
		}

		conflicts = append(conflicts, parentConflict{
			ParentName: parentName,
			DepName:    name,
			Version:    info.Version,
		})

		if conflictVersionInfos[name] == nil {
			conflictVersionInfos[name] = make(map[string]packageInfo)
		}
		conflictVersionInfos[name][info.Version] = info
	}

	// Build parent -> nested deps mapping
	parentNestedDeps := make(map[string]map[string]string) // parentName -> depName -> target
	for _, c := range conflicts {
		if parentNestedDeps[c.ParentName] == nil {
			parentNestedDeps[c.ParentName] = make(map[string]string)
		}
		targetName := versionedTargetName(c.DepName, c.Version)
		parentNestedDeps[c.ParentName][c.DepName] = fmt.Sprintf("//%s:%s", c.DepName, targetName)
	}

	// Phase 4: Build resolvedPackage entries
	var result []resolvedPackage
	for path, info := range pkgs {
		if path == "" {
			continue
		}
		name := common.ExtractPackageName(path)
		if name == "" {
			continue
		}

		// Handle nested packages: only allow promoted ones through
		if common.IsNestedPackage(path) {
			if promoted[name] != path {
				continue
			}
		}

		if noDev && info.Dev {
			continue
		}

		if info.Resolved == "" {
			continue
		}

		var deps []string
		for dep := range info.Dependencies {
			if topLevel[dep] {
				deps = append(deps, dep)
			}
		}
		for dep := range info.PeerDependencies {
			if meta, ok := info.PeerDependenciesMeta[dep]; ok && meta.Optional {
				continue
			}
			if topLevel[dep] {
				deps = append(deps, dep)
			}
		}
		sort.Strings(deps)

		var realName string
		if rn := common.ExtractRealPackageName(info.Resolved); rn != "" && rn != name {
			realName = rn
		}

		pkg := resolvedPackage{
			Name:     name,
			RealName: realName,
			Version:  info.Version,
			Resolved: info.Resolved,
			Deps:     deps,
			Dev:      info.Dev,
		}

		if nd, ok := parentNestedDeps[name]; ok {
			pkg.NestedDeps = nd
		}

		result = append(result, pkg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	// Phase 5: Build conflict targets
	var ctargets []conflictTarget
	seen := make(map[string]bool)
	for _, c := range conflicts {
		key := c.DepName + "@" + c.Version
		if seen[key] {
			continue
		}
		seen[key] = true

		info := conflictVersionInfos[c.DepName][c.Version]

		var deps []string
		for dep := range info.Dependencies {
			if topLevel[dep] {
				deps = append(deps, dep)
			}
		}
		sort.Strings(deps)

		ctargets = append(ctargets, conflictTarget{
			Dir:        c.DepName,
			TargetName: versionedTargetName(c.DepName, c.Version),
			PkgName:    c.DepName,
			Version:    c.Version,
			Deps:       deps,
		})
	}

	sort.Slice(ctargets, func(i, j int) bool {
		if ctargets[i].Dir != ctargets[j].Dir {
			return ctargets[i].Dir < ctargets[j].Dir
		}
		return ctargets[i].TargetName < ctargets[j].TargetName
	})

	return result, ctargets
}

// versionedTargetName generates a version-qualified target name for a conflict target.
// "zod", "4.3.6" → "zod_v4_3_6"
// "@types/react", "17.0.0" → "react_v17_0_0"
func versionedTargetName(name, version string) string {
	base := name
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		base = parts[len(parts)-1]
	}
	v := strings.NewReplacer(".", "_", "-", "_").Replace(version)
	return fmt.Sprintf("%s_v%s", base, v)
}

// writePlzConfig writes the .plzconfig for the subrepo.
func writePlzConfig(outDir string) error {
	content := "[Plugin \"js\"]\nTarget=@//plugins:js\n"
	return os.WriteFile(filepath.Join(outDir, ".plzconfig"), []byte(content), 0644)
}

// writeBuildFile generates a BUILD file for a single npm package.
func writeBuildFile(outDir string, pkg resolvedPackage, subincludePath string) error {
	// Determine directory path for the package
	pkgDir := filepath.Join(outDir, pkg.Name)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return err
	}

	// Determine target name (last component for scoped packages)
	targetName := pkg.Name
	if strings.Contains(pkg.Name, "/") {
		parts := strings.Split(pkg.Name, "/")
		targetName = parts[len(parts)-1]
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("subinclude(%q)\n\n", subincludePath))
	b.WriteString(fmt.Sprintf("npm_module(\n"))
	b.WriteString(fmt.Sprintf("    name = %q,\n", targetName))
	// Emit pkg_name for scoped packages (target name differs from package name)
	// or for aliases (real npm name differs from alias name)
	pkgName := pkg.Name
	if pkg.RealName != "" {
		pkgName = pkg.RealName
	}
	if targetName != pkgName || pkg.RealName != "" {
		b.WriteString(fmt.Sprintf("    pkg_name = %q,\n", pkgName))
	}
	b.WriteString(fmt.Sprintf("    version = %q,\n", pkg.Version))

	if len(pkg.Deps) > 0 {
		b.WriteString("    deps = [\n")
		for _, dep := range pkg.Deps {
			target := depTarget(dep)
			b.WriteString(fmt.Sprintf("        %q,\n", target))
		}
		b.WriteString("    ],\n")
	}

	if len(pkg.NestedDeps) > 0 {
		b.WriteString("    nested_deps = {\n")
		var keys []string
		for k := range pkg.NestedDeps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("        %q: %q,\n", k, pkg.NestedDeps[k]))
		}
		b.WriteString("    },\n")
	}

	if pkg.Dev {
		b.WriteString("    labels = [\"npm:dev\"],\n")
	}

	b.WriteString("    visibility = [\"PUBLIC\"],\n")
	b.WriteString(")\n")

	return os.WriteFile(filepath.Join(pkgDir, "BUILD"), []byte(b.String()), 0644)
}

// appendConflictTarget appends a version-conflict npm_module target to an existing BUILD file.
func appendConflictTarget(outDir string, ct conflictTarget) error {
	pkgDir := filepath.Join(outDir, ct.Dir)
	buildFile := filepath.Join(pkgDir, "BUILD")

	var b strings.Builder
	b.WriteString("\nnpm_module(\n")
	b.WriteString(fmt.Sprintf("    name = %q,\n", ct.TargetName))
	b.WriteString(fmt.Sprintf("    pkg_name = %q,\n", ct.PkgName))
	b.WriteString(fmt.Sprintf("    version = %q,\n", ct.Version))

	if len(ct.Deps) > 0 {
		b.WriteString("    deps = [\n")
		for _, dep := range ct.Deps {
			target := depTarget(dep)
			b.WriteString(fmt.Sprintf("        %q,\n", target))
		}
		b.WriteString("    ],\n")
	}

	b.WriteString("    visibility = [\"PUBLIC\"],\n")
	b.WriteString(")\n")

	f, err := os.OpenFile(buildFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(b.String())
	return err
}

// depTarget converts a package name to a subrepo target reference.
// "react" → "//react"
// "@types/react" → "//@types/react:react"
func depTarget(name string) string {
	if strings.Contains(name, "/") {
		// Scoped package: @scope/pkg → //@scope/pkg:pkg
		parts := strings.Split(name, "/")
		targetName := parts[len(parts)-1]
		return fmt.Sprintf("//%s:%s", name, targetName)
	}
	return fmt.Sprintf("//%s", name)
}
