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
