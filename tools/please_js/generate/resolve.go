package generate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"tools/please_js/lockfile"
)

// Pins is every package a workspace project's tree stages, as build labels:
// the npm_repo targets the tree's generated BUILD file defines, qualified with
// the tree's package. This is what npm_project attaches to its node_modules at
// build time, so the list never has to exist in any file. With merge, peer
// variants of one package@version are folded to a single pin (see Project).
func Pins(plan *Plan, project, tree string, merge bool) ([]string, error) {
	if _, ok := plan.Closure[project]; !ok {
		known := make([]string, 0, len(plan.Closure))
		for p := range plan.Closure {
			known = append(known, p)
		}
		sort.Strings(known)
		return nil, fmt.Errorf("the lockfile has no project %q; it has %s. "+
			"Add the package to packages in the tree's pnpm-workspace.yaml and regenerate it",
			project, strings.Join(known, ", "))
	}
	closure := plan.Project(project, merge).Closure
	out := make([]string, len(closure))
	for i, target := range closure {
		out[i] = tree + ":" + target
	}
	return out, nil
}

// CheckManifest reports where a project's package.json and the lockfile's
// record of it disagree, section by section: a dependency added, removed or
// re-specified in package.json since the lockfile was generated.
//
// The lockfile records the specifier package.json asked for alongside the
// version it resolved, so this compares the two statements of intent
// directly. A package.json edited without regenerating is the failure this
// exists for: without it, the build would quietly use the old resolution.
func CheckManifest(manifest []byte, imp lockfile.Importer) error {
	var pkg struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(manifest, &pkg); err != nil {
		return fmt.Errorf("reading package.json: %w", err)
	}

	var problems []string
	for _, section := range []struct {
		name     string
		declared map[string]string
		recorded map[string]lockfile.ImporterDep
	}{
		{"dependencies", pkg.Dependencies, imp.Dependencies},
		{"devDependencies", pkg.DevDependencies, imp.DevDependencies},
		{"optionalDependencies", pkg.OptionalDependencies, imp.OptionalDependencies},
	} {
		names := map[string]struct{}{}
		for n := range section.declared {
			names[n] = struct{}{}
		}
		for n := range section.recorded {
			names[n] = struct{}{}
		}
		sorted := make([]string, 0, len(names))
		for n := range names {
			sorted = append(sorted, n)
		}
		sort.Strings(sorted)

		for _, n := range sorted {
			declared, inManifest := section.declared[n]
			recorded, inLock := section.recorded[n]
			switch {
			case !inLock:
				problems = append(problems, fmt.Sprintf("%s.%s: %s in package.json, missing from the lockfile", section.name, n, declared))
			case !inManifest:
				problems = append(problems, fmt.Sprintf("%s.%s: in the lockfile, missing from package.json", section.name, n))
			case declared != recorded.Specifier:
				problems = append(problems, fmt.Sprintf("%s.%s: %s in package.json, %s in the lockfile", section.name, n, declared, recorded.Specifier))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("package.json and the lockfile disagree:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}
