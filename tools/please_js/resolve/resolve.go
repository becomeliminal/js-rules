package resolve

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// packageInfo represents a single package entry in the lockfile.
type packageInfo struct {
	Version          string            `json:"version"`
	Resolved         string            `json:"resolved"`
	Integrity        string            `json:"integrity"`
	Dependencies     map[string]string `json:"dependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
	Dev              bool              `json:"dev"`
	Optional         bool              `json:"optional"`
	OS               []string          `json:"os"`
	CPU              []string          `json:"cpu"`
}

// resolvedPackage is the processed form we use for BUILD file generation.
type resolvedPackage struct {
	Name     string   // npm package name (e.g., "react", "@types/react")
	Version  string
	Resolved string   // tarball URL
	Deps     []string // dependency package names (mapped to subrepo targets)
	Dev      bool     // true if this is a dev-only package
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

	// Collect all top-level packages (skip root, nested, optional, dev)
	packages := collectPackages(lock.Packages, args.NoDev)

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

	fmt.Fprintf(os.Stderr, "Generated %d npm_module rules\n", len(packages))
	return nil
}

// collectPackages extracts top-level packages from the lockfile.
func collectPackages(pkgs map[string]packageInfo, noDev bool) []resolvedPackage {
	// Build set of known top-level package names for dep resolution
	topLevel := make(map[string]bool)
	for path, info := range pkgs {
		if path == "" {
			continue // skip root
		}
		name := extractPackageName(path)
		if name == "" {
			continue
		}
		// Only consider top-level (not nested node_modules)
		if isNestedPackage(path) {
			continue
		}
		// Skip optional packages so they don't appear in other packages' deps
		if info.Optional {
			continue
		}
		topLevel[name] = true
	}

	var result []resolvedPackage
	for path, info := range pkgs {
		if path == "" {
			continue // skip root package
		}

		name := extractPackageName(path)
		if name == "" {
			continue
		}

		// Skip nested node_modules (version conflicts) for v1
		if isNestedPackage(path) {
			log.Printf("warning: skipping nested package %s (version conflict handling not yet supported)", path)
			continue
		}

		// Skip optional packages
		if info.Optional {
			continue
		}

		// Skip dev packages if --no-dev
		if noDev && info.Dev {
			continue
		}

		// Skip packages without a resolved URL (local packages, workspace refs)
		if info.Resolved == "" {
			continue
		}

		// Map dependencies to target names (only include deps that exist at top level)
		var deps []string
		for dep := range info.Dependencies {
			if topLevel[dep] {
				deps = append(deps, dep)
			}
		}
		sort.Strings(deps)

		result = append(result, resolvedPackage{
			Name:     name,
			Version:  info.Version,
			Resolved: info.Resolved,
			Deps:     deps,
			Dev:      info.Dev,
		})
	}

	// Sort for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// extractPackageName extracts the npm package name from a lockfile path.
// "node_modules/react" → "react"
// "node_modules/@types/react" → "@types/react"
func extractPackageName(path string) string {
	const prefix = "node_modules/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	// Find the last occurrence of "node_modules/" to handle nested packages
	idx := strings.LastIndex(path, prefix)
	return path[idx+len(prefix):]
}

// isNestedPackage checks if a package is inside another package's node_modules.
// "node_modules/react" → false
// "node_modules/@types/react" → false
// "node_modules/react-dom/node_modules/scheduler" → true
func isNestedPackage(path string) bool {
	// Count occurrences of "node_modules/"
	return strings.Count(path, "node_modules/") > 1
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
	if targetName != pkg.Name {
		b.WriteString(fmt.Sprintf("    pkg_name = %q,\n", pkg.Name))
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

	if pkg.Dev {
		b.WriteString("    labels = [\"npm:dev\"],\n")
	}

	b.WriteString("    visibility = [\"PUBLIC\"],\n")
	b.WriteString(")\n")

	return os.WriteFile(filepath.Join(pkgDir, "BUILD"), []byte(b.String()), 0644)
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
