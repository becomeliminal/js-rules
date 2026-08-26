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
