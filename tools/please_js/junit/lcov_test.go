package junit_test

import (
	"strings"
	"testing"

	"tools/please_js/junit"
)

// The shape of node's own lcov output, cut down: a repo file, a third-party
// file, and something outside the repo entirely.
const nodeLcov = `TN:
SF:/run/dir/test/suite/greeter.test.js
DA:1,1
DA:2,0
end_of_record
SF:/run/dir/node_modules/@test/greeter/index.js
DA:1,5
end_of_record
SF:/usr/lib/node/internal/whatever.js
DA:1,9
end_of_record
`

func TestLcovBecomesGoCover(t *testing.T) {
	got := junit.LcovToGoCover(nodeLcov, []string{"/run/dir"}, nil)
	if !strings.HasPrefix(got, "mode: count\n") {
		t.Errorf("go-cover starts with a mode line:\n%s", got)
	}
	// The repo file, by the path Please knows it by, hit and unhit lines both.
	for _, want := range []string{
		"test/suite/greeter.test.js:1.1,1.999 1 1",
		"test/suite/greeter.test.js:2.1,2.999 1 0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Third-party coverage is noise, and node internals are not the repo's.
	for _, absent := range []string{"node_modules", "internal/whatever"} {
		if strings.Contains(got, absent) {
			t.Errorf("%q should have been dropped:\n%s", absent, got)
		}
	}
}

// A staged first-party library's hits map back to its source file -- the file
// Please knows -- via the overlay's package-to-srcDir record. Please excludes
// a test's own srcs from reports, so the mapped libraries are most of what
// coverage is for.
func TestLibraryCoverageMapsToItsSource(t *testing.T) {
	got := junit.LcovToGoCover(nodeLcov, []string{"/run/dir"}, map[string]string{
		"@test/greeter": "test/library/greeter",
	})
	if !strings.Contains(got, "test/library/greeter/index.js:1.1,1.999 1 5") {
		t.Errorf("the library's hit line should map to its source:\n%s", got)
	}
	if strings.Contains(got, "node_modules") {
		t.Errorf("no staged path should survive:\n%s", got)
	}
}

// The editor fragment: first-party packages map to their sources, everything
// else falls through to the built trees, every entry ./-relative because
// TypeScript 7 removed baseUrl -- found by running the output through the
// real compiler, which also rejects bare relative paths.
func TestEditorConfigMapsSourcesAndTrees(t *testing.T) {
	got, err := junit.EditorConfig([]string{
		"lib\t@test/greeter\ttest/lib/greeter",
		"tree\tplz-out/gen/third_party/js/node_modules",
		"",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"./test/lib/greeter/index.ts"`,
		`"@test/greeter/*"`,
		`"./plz-out/gen/third_party/js/node_modules/*"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "baseUrl") {
		t.Error("TypeScript 7 removed baseUrl; emitting it makes the whole file an error")
	}
	if strings.Contains(got, "node_modules/node_modules") {
		t.Error("the npm_link output IS the tree root; doubling the segment was the first real run's bug")
	}
}

func TestEditorConfigRefusesWhatItCannotRead(t *testing.T) {
	if _, err := junit.EditorConfig([]string{"garbage line"}); err == nil {
		t.Error("an unrecognised line should be refused, not skipped -- it means the generating script broke")
	}
}
