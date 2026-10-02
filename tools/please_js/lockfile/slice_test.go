package lockfile_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"tools/please_js/lockfile"
)

const twoProjects = `lockfileVersion: '9.0'

settings:
  autoInstallPeers: true

importers:

  ../../apps/web:
    dependencies:
      ms:
        specifier: 2.1.3
        version: 2.1.3
      shared:
        specifier: ^1.0.0
        version: 1.0.0

  ../../apps/admin:
    dependencies:
      shared:
        specifier: ^1.0.0
        version: 1.0.0
      zod:
        specifier: ^3.0.0
        version: 3.25.76

packages:

  ms@2.1.3:
    resolution: {integrity: sha512-ms}

  shared@1.0.0:
    resolution: {integrity: sha512-shared}

  zod@3.25.76:
    resolution: {integrity: sha512-zod}

snapshots:

  ms@2.1.3: {}

  shared@1.0.0: {}

  zod@3.25.76: {}
`

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestSliceKeepsOnlyTheProjectsOwnResolution(t *testing.T) {
	// GIVEN a workspace lockfile with two projects sharing one package
	dir := t.TempDir()
	full := filepath.Join(dir, "pnpm-lock.yaml")
	os.WriteFile(full, []byte(twoProjects), 0o644)
	out := filepath.Join(dir, "web.lock.yaml")

	// WHEN web's slice is written for the snapshots its tree reaches
	if err := lockfile.Slice(full, "../../apps/web", []string{"ms@2.1.3", "shared@1.0.0"}, out); err != nil {
		t.Fatal(err)
	}

	// THEN it parses as a lockfile holding web alone, and admin's zod is gone
	got, err := lockfile.Parse(out)
	if err != nil {
		t.Fatalf("the slice should be a valid lockfile: %v", err)
	}
	if want := []string{"../../apps/web"}; !reflect.DeepEqual(mapKeys(got.Importers), want) {
		t.Errorf("importers = %v, want %v", mapKeys(got.Importers), want)
	}
	if want := []string{"ms@2.1.3", "shared@1.0.0"}; !reflect.DeepEqual(mapKeys(got.Packages), want) {
		t.Errorf("packages = %v, want %v", mapKeys(got.Packages), want)
	}
	if want := []string{"ms@2.1.3", "shared@1.0.0"}; !reflect.DeepEqual(mapKeys(got.Snapshots), want) {
		t.Errorf("snapshots = %v, want %v", mapKeys(got.Snapshots), want)
	}
	// AND the facts pnpm recorded survive
	if got.Packages["ms@2.1.3"].Resolution.Integrity != "sha512-ms" {
		t.Errorf("integrity lost: %+v", got.Packages["ms@2.1.3"])
	}
}

func TestSliceIsUnchangedByAnotherProjectsBump(t *testing.T) {
	// GIVEN web's slice, then the lockfile after admin alone bumps zod
	dir := t.TempDir()
	full := filepath.Join(dir, "pnpm-lock.yaml")
	before, after := filepath.Join(dir, "before.yaml"), filepath.Join(dir, "after.yaml")
	keys := []string{"ms@2.1.3", "shared@1.0.0"}
	os.WriteFile(full, []byte(twoProjects), 0o644)
	if err := lockfile.Slice(full, "../../apps/web", keys, before); err != nil {
		t.Fatal(err)
	}
	bumped := []byte(twoProjects)
	bumped = []byte(strings.ReplaceAll(string(bumped), "3.25.76", "3.25.77"))
	os.WriteFile(full, bumped, 0o644)

	// WHEN web's slice is written again
	if err := lockfile.Slice(full, "../../apps/web", keys, after); err != nil {
		t.Fatal(err)
	}

	// THEN it is byte-identical, so web's tree stays a cache hit
	a, _ := os.ReadFile(before)
	b, _ := os.ReadFile(after)
	if string(a) != string(b) {
		t.Errorf("web's slice changed when only admin did:\n--- before\n%s\n--- after\n%s", a, b)
	}
}

func TestSliceRefusesAProjectTheLockfileDoesNotHave(t *testing.T) {
	// GIVEN a workspace lockfile
	dir := t.TempDir()
	full := filepath.Join(dir, "pnpm-lock.yaml")
	os.WriteFile(full, []byte(twoProjects), 0o644)

	// WHEN a slice is asked for a project it does not list
	err := lockfile.Slice(full, "../../apps/missing", nil, filepath.Join(dir, "x.yaml"))

	// THEN it fails rather than writing an empty slice
	if err == nil {
		t.Error("expected an error for a project the lockfile does not have")
	}
}
