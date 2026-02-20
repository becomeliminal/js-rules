package resolve

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"tools/please_js/common"
)

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

// targetName returns the Please target name for this package.
// For scoped packages (@scope/pkg), returns just the last component.
func (p resolvedPackage) targetName() string {
	if strings.Contains(p.Name, "/") {
		parts := strings.Split(p.Name, "/")
		return parts[len(parts)-1]
	}
	return p.Name
}

// effectivePkgName returns the real npm package name if aliased, otherwise the name.
func (p resolvedPackage) effectivePkgName() string {
	if p.RealName != "" {
		return p.RealName
	}
	return p.Name
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

// parentConflict records a version conflict between a nested package
// and the top-level version.
type parentConflict struct {
	ParentName string
	DepName    string
	Version    string
}

// breakCycles detects and removes back-edges in the dependency graph via DFS,
// ensuring the resulting graph is a DAG suitable for Please's build system.
// It operates on a unified graph containing both regular packages and
// version-conflict targets so that cycles spanning both are detected.
func breakCycles(packages []resolvedPackage, ctargets []conflictTarget) {
	// Build unified adjacency list over both regular packages and conflict targets.
	adj := make(map[string][]string)

	// Track which edges from regular packages come from NestedDeps.
	// nestedEdgeKey[pkgName][conflictTargetName] = importName
	nestedEdgeKey := make(map[string]map[string]string)

	// Add regular package nodes.
	for _, pkg := range packages {
		var edges []string
		for _, dep := range pkg.Deps {
			edges = append(edges, dep)
		}
		for importName, label := range pkg.NestedDeps {
			targetName := extractTargetName(label)
			edges = append(edges, targetName)
			if nestedEdgeKey[pkg.Name] == nil {
				nestedEdgeKey[pkg.Name] = make(map[string]string)
			}
			nestedEdgeKey[pkg.Name][targetName] = importName
		}
		adj[pkg.Name] = edges
	}

	// Add conflict target nodes.
	for _, ct := range ctargets {
		var edges []string
		for _, dep := range ct.Deps {
			edges = append(edges, dep)
		}
		adj[ct.TargetName] = edges
	}

	// Sort all node keys for deterministic traversal.
	allNodes := make([]string, 0, len(adj))
	for key := range adj {
		allNodes = append(allNodes, key)
	}
	sort.Strings(allNodes)

	// DFS coloring: 0=white, 1=gray (in stack), 2=black (done)
	color := make(map[string]int, len(allNodes))

	var dfs func(name string)
	dfs = func(name string) {
		color[name] = 1
		var kept []string
		for _, dep := range adj[name] {
			if _, inGraph := adj[dep]; !inGraph {
				// Keep edges to nodes outside the graph (e.g. filtered-out packages).
				kept = append(kept, dep)
				continue
			}
			if color[dep] == 1 {
				log.Printf("warning: breaking circular dependency: %s -> %s", name, dep)
				continue
			}
			kept = append(kept, dep)
			if color[dep] == 0 {
				dfs(dep)
			}
		}
		adj[name] = kept
		color[name] = 2
	}

	for _, node := range allNodes {
		if color[node] == 0 {
			dfs(node)
		}
	}

	// Write back pruned edges to regular packages.
	for i, pkg := range packages {
		var deps []string
		var nestedDeps map[string]string
		for _, edge := range adj[pkg.Name] {
			if importName, ok := nestedEdgeKey[pkg.Name][edge]; ok {
				if nestedDeps == nil {
					nestedDeps = make(map[string]string)
				}
				nestedDeps[importName] = pkg.NestedDeps[importName]
			} else {
				deps = append(deps, edge)
			}
		}
		packages[i].Deps = deps
		packages[i].NestedDeps = nestedDeps
	}

	// Write back pruned edges to conflict targets.
	for i := range ctargets {
		var deps []string
		for _, edge := range adj[ctargets[i].TargetName] {
			deps = append(deps, edge)
		}
		ctargets[i].Deps = deps
	}
}

// extractTargetName extracts the target name from a subrepo target label
// like "//dir:target_name".
func extractTargetName(label string) string {
	if idx := strings.LastIndex(label, ":"); idx >= 0 {
		return label[idx+1:]
	}
	parts := strings.Split(strings.TrimPrefix(label, "//"), "/")
	return parts[len(parts)-1]
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
		targetName := common.VersionedTargetName(c.DepName, c.Version)
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
			TargetName: common.VersionedTargetName(c.DepName, c.Version),
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
