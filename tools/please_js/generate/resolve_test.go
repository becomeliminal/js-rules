package generate_test

import (
	"reflect"
	"strings"
	"testing"

	"tools/please_js/generate"
	"tools/please_js/lockfile"
)

func TestPinsQualifiesTheProjectClosureWithTheTree(t *testing.T) {
	// GIVEN a plan whose project ../../apps/web reaches two packages
	plan := &generate.Plan{Closure: map[string][]string{
		"../../apps/web": {"ms_2.1.3", "react_19.1.1"},
		"../../apps/api": {"zod_3.25.76"},
	}}

	// WHEN the web project's pins are resolved against //third_party/js
	pins, err := generate.Pins(plan, "../../apps/web", "//third_party/js", true)

	// THEN each is that tree's target for the package, and nothing of api's
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"//third_party/js:ms_2.1.3", "//third_party/js:react_19.1.1"}
	if !reflect.DeepEqual(pins, want) {
		t.Errorf("pins = %v, want %v", pins, want)
	}
}

func TestPinsRefusesAProjectTheLockfileDoesNotHave(t *testing.T) {
	// GIVEN a plan with one project
	plan := &generate.Plan{Closure: map[string][]string{"../../apps/web": {"ms_2.1.3"}}}

	// WHEN a project missing from the workspace is resolved
	_, err := generate.Pins(plan, "../../apps/admin", "//third_party/js", true)

	// THEN the error names the projects that exist and the fix
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	for _, want := range []string{`"../../apps/admin"`, "../../apps/web", "pnpm-workspace.yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func importer() lockfile.Importer {
	return lockfile.Importer{
		Dependencies: map[string]lockfile.ImporterDep{
			"react": {Specifier: "19.1.1"},
			"ms":    {Specifier: "^2.1.0"},
		},
		DevDependencies: map[string]lockfile.ImporterDep{
			"vitest": {Specifier: "4.1.11"},
		},
	}
}

func TestCheckManifestAcceptsAPackageJSONTheLockfileMatches(t *testing.T) {
	// GIVEN a package.json declaring exactly what the lockfile recorded
	manifest := []byte(`{
		"dependencies": {"react": "19.1.1", "ms": "^2.1.0"},
		"devDependencies": {"vitest": "4.1.11"}
	}`)

	// WHEN it is checked
	err := generate.CheckManifest(manifest, importer())

	// THEN nothing is reported
	if err != nil {
		t.Errorf("an in-sync package.json should pass: %v", err)
	}
}

func TestCheckManifestNamesEveryDriftSinceTheLockfile(t *testing.T) {
	// GIVEN a package.json edited after the lockfile was generated: react
	// bumped, zod added, ms removed, vitest moved to dependencies
	manifest := []byte(`{
		"dependencies": {"react": "19.2.0", "zod": "^3.0.0", "vitest": "4.1.11"}
	}`)

	// WHEN it is checked
	err := generate.CheckManifest(manifest, importer())

	// THEN every disagreement is named, by section, with both sides
	if err == nil {
		t.Fatal("a drifted package.json should fail")
	}
	for _, want := range []string{
		"dependencies.react: 19.2.0 in package.json, 19.1.1 in the lockfile",
		"dependencies.zod: ^3.0.0 in package.json, missing from the lockfile",
		"dependencies.ms: in the lockfile, missing from package.json",
		"dependencies.vitest: 4.1.11 in package.json, missing from the lockfile",
		"devDependencies.vitest: in the lockfile, missing from package.json",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q:\n%v", want, err)
		}
	}
}
