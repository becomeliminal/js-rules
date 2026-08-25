package hooks_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/hooks"
)

func pkg(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// npm's order is not alphabetical, and a package declaring several relies on
// it: preinstall, then install, then postinstall.
func TestHooksRunInNpmsOrder(t *testing.T) {
	dir := pkg(t, `{"scripts":{
		"postinstall":"echo third >> log",
		"preinstall":"echo first >> log",
		"install":"echo second >> log",
		"test":"echo never >> log"
	}}`)
	var out bytes.Buffer
	if err := hooks.Run(dir, os.Environ(), &out); err != nil {
		t.Fatal(err, out.String())
	}
	log, err := os.ReadFile(filepath.Join(dir, "log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(log)); strings.Join(got, ",") != "first,second,third" {
		t.Errorf("ran %v, want first second third", got)
	}
}

// The common case by a wide margin: most of the registry declares nothing, and
// a package with no manifest at all is not an error either.
func TestNoScriptsIsNotAnError(t *testing.T) {
	for _, manifest := range []string{"", `{}`, `{"scripts":{"test":"jest"}}`} {
		var out bytes.Buffer
		if err := hooks.Run(pkg(t, manifest), os.Environ(), &out); err != nil {
			t.Errorf("manifest %q: %v", manifest, err)
		}
	}
}

// A hook sees what it was given and nothing else, so it cannot depend on the
// machine it built on and then fail somewhere that machine is different.
func TestEnvironmentIsExactlyWhatWasGiven(t *testing.T) {
	t.Setenv("SHOULD_NOT_LEAK", "1")
	dir := pkg(t, `{"scripts":{"postinstall":"printf '%s|%s' \"$GIVEN\" \"$SHOULD_NOT_LEAK\" > seen"}}`)
	var out bytes.Buffer
	if err := hooks.Run(dir, []string{"GIVEN=yes", "PATH=" + os.Getenv("PATH")}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	seen, err := os.ReadFile(filepath.Join(dir, "seen"))
	if err != nil {
		t.Fatal(err)
	}
	if string(seen) != "yes|" {
		t.Errorf("hook saw %q; the build's own environment leaked in", seen)
	}
}

// A package that reports success while half-installed is worse than one that
// fails, so the sequence stops at the first failure.
func TestAFailingHookStopsTheSequence(t *testing.T) {
	dir := pkg(t, `{"scripts":{"install":"exit 3","postinstall":"echo ran > after"}}`)
	var out bytes.Buffer
	err := hooks.Run(dir, os.Environ(), &out)
	if err == nil {
		t.Fatal("expected the failure to propagate")
	}
	if !strings.Contains(err.Error(), "install script failed") {
		t.Errorf("error should name the hook, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "after")); err == nil {
		t.Error("postinstall ran after install failed")
	}
}

// Hooks run in the package, not in whatever directory the tool was invoked
// from. Every install script assumes it.
func TestHooksRunInThePackageDirectory(t *testing.T) {
	dir := pkg(t, `{"scripts":{"postinstall":"pwd > where"}}`)
	var out bytes.Buffer
	if err := hooks.Run(dir, os.Environ(), &out); err != nil {
		t.Fatal(err)
	}
	where, err := os.ReadFile(filepath.Join(dir, "where"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(where)))
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("ran in %s, want %s", got, want)
	}
}

func TestReadReportsWhatAPackageDeclares(t *testing.T) {
	got, err := hooks.Read(pkg(t, `{"scripts":{"postinstall":"a","preinstall":"b"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "preinstall" || got[1].Name != "postinstall" {
		t.Errorf("got %v", got)
	}
}
