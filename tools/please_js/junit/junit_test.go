package junit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/junit"
)

// Exactly what node writes for a file whose tests are not inside a describe:
// the cases sit directly under <testsuites>, where Please does not look.
const nodeFlat = `<?xml version="1.0" encoding="utf-8"?>
<testsuites>
	<testcase name="one" time="0.001" classname="test"/>
	<testcase name="two" time="0.002" classname="test" failure="nope">
		<failure type="testCodeFailure" message="nope">stack goes here</failure>
	</testcase>
</testsuites>`

const nodeMixed = `<?xml version="1.0" encoding="utf-8"?>
<testsuites>
	<testsuite name="greet" time="0.01" tests="1" failures="0">
		<testcase name="greets" time="0.001" classname="test"/>
	</testsuite>
	<testcase name="loose" time="0.002" classname="test"/>
</testsuites>`

func convert(t *testing.T, in string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "in.xml")
	dst := filepath.Join(dir, "out.xml")
	if err := os.WriteFile(src, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := junit.Convert(src, dst, "my_test"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The whole point: a file without describe reports nothing before this, because
// Please reads only cases inside a testsuite.
func TestLooseCasesGetASuite(t *testing.T) {
	got := convert(t, nodeFlat)
	if strings.Count(got, "<testsuite ") != 1 {
		t.Errorf("expected one suite, got:\n%s", got)
	}
	for _, name := range []string{`name="one"`, `name="two"`, `name="my_test"`} {
		if !strings.Contains(got, name) {
			t.Errorf("%s missing from:\n%s", name, got)
		}
	}
	// A case outside a suite is what Please cannot read, so none may remain.
	if strings.Contains(got, "</testsuite>\n\t<testcase") {
		t.Errorf("a case was left outside a suite:\n%s", got)
	}
}

// The failure has to survive with its message, or a red test reports as red
// with nothing to read.
func TestFailuresSurviveTheRewrite(t *testing.T) {
	got := convert(t, nodeFlat)
	if !strings.Contains(got, `message="nope"`) || !strings.Contains(got, "stack goes here") {
		t.Errorf("the failure lost its detail:\n%s", got)
	}
	if !strings.Contains(got, `failures="1"`) {
		t.Errorf("the suite should count its failures:\n%s", got)
	}
}

// A describe block already produces a suite, and rewriting must not disturb it.
func TestExistingSuitesAreLeftAlone(t *testing.T) {
	got := convert(t, nodeMixed)
	if strings.Count(got, "<testsuite ") != 2 {
		t.Errorf("expected the original suite plus one for the loose case:\n%s", got)
	}
	if !strings.Contains(got, `name="greet"`) || !strings.Contains(got, `name="greets"`) {
		t.Errorf("the original suite was damaged:\n%s", got)
	}
}

// Nothing loose means nothing to do, and a document that already suits Please
// must come back unchanged in the ways that matter.
func TestADocumentWithNoLooseCasesIsUntouched(t *testing.T) {
	doc := junit.Document{Suites: []junit.Suite{{Name: "a", Cases: []junit.Case{{Name: "x"}}}}}
	junit.Normalise(&doc, "my_test")
	if len(doc.Suites) != 1 || doc.Suites[0].Name != "a" {
		t.Errorf("got %+v", doc.Suites)
	}
}

// Exactly what node writes for nested describes holding tests that share a
// name: a <testsuite> inside a <testsuite>, and every case classnamed "test".
const nodeNested = `<?xml version="1.0" encoding="utf-8"?>
<testsuites>
	<testcase name="loose" time="0.000517" classname="test"/>
	<testsuite name="outer" time="0.000648" tests="2" failures="0">
		<testcase name="same" time="0.000117" classname="test"/>
		<testsuite name="inner" time="0.000288" tests="2" failures="1">
			<testcase name="same" time="0.000087" classname="test">
				<failure type="testCodeFailure" message="inner broke">stack</failure>
			</testcase>
			<testcase name="deep" time="0.000106" classname="test"/>
		</testsuite>
	</testsuite>
	<testsuite name="other" time="0.000731" tests="1" failures="0">
		<testcase name="same" time="0.000665" classname="test"/>
	</testsuite>
</testsuites>`

// A test inside a nested describe used to vanish from the report: only one
// level of suite was read.
func TestNestedDescribesAreReported(t *testing.T) {
	got := convert(t, nodeNested)
	if n := strings.Count(got, "<testcase "); n != 5 {
		t.Errorf("expected all 5 tests, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, `name="outer &gt; inner"`) {
		t.Errorf("the nested describe should be a suite named by its path:\n%s", got)
	}
}

// Please tells tests apart by classname and name, and node classnames every
// test "test" -- so "same" in three describe blocks was one test to Please,
// and a failure among them could be merged with the passes as a flake.
func TestSameNamedTestsInDifferentDescribesStayDistinct(t *testing.T) {
	got := convert(t, nodeNested)
	for _, want := range []string{
		`name="loose" time="0.000517" classname="my_test"`,
		`name="same" time="0.000117" classname="outer"`,
		`name="same" time="0.000087" classname="outer &gt; inner"`,
		`name="same" time="0.000665" classname="other"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	// The failure stays on the test that failed, counted by its own suite.
	if !strings.Contains(got, `message="inner broke"`) {
		t.Errorf("the failure lost its detail:\n%s", got)
	}
	if strings.Count(got, `failures="1"`) != 1 {
		t.Errorf("exactly one suite should count the failure:\n%s", got)
	}
}
