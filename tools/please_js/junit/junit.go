// Package junit normalises the test results node's runner writes.
//
// Node emits a <testcase> directly under <testsuites> for any test not inside a
// describe block, and Please reads only the ones inside a <testsuite>. The
// effect is that a test file written without describe -- which is most of them
// -- reports nothing at all. A failing test still fails the target, because the
// runner exits non-zero, but it is reported as a nameless error rather than as
// the test that broke, and a passing one is invisible.
//
// Wrapping the loose cases makes the document standard, which is all Please
// needs.
//
// Two more things node writes that Please reads differently. A nested describe
// is a <testsuite> inside a <testsuite>, which Please does not read either, so
// its tests vanished from the report. And every case is classnamed "test",
// while Please tells tests apart by classname and name -- so two tests with
// one name in different describe blocks were one test to it, and a failure
// among them could be merged with the passes and reported as a flake. Suites
// are flattened, each named by its describe path, and that path is each
// case's classname. This is the same job please_rust does for libtest output, with less
// work: node emits XML already, it is just shaped for a reader that tolerates
// orphans.
package junit

import (
	"encoding/xml"
	"fmt"
	"os"
)

// Failure is a test that did not pass. Node puts the assertion message in the
// attribute and the whole diff, with its stack, in the body.
type Failure struct {
	Type    string `xml:"type,attr,omitempty"`
	Message string `xml:"message,attr,omitempty"`
	Body    string `xml:",chardata"`
}

// Case is one test.
type Case struct {
	XMLName   xml.Name `xml:"testcase"`
	Name      string   `xml:"name,attr"`
	Time      string   `xml:"time,attr,omitempty"`
	Classname string   `xml:"classname,attr,omitempty"`
	Failure   *Failure `xml:"failure,omitempty"`
	Skipped   *struct{} `xml:"skipped,omitempty"`
}

// Suite is a describe block, or the synthetic one loose tests are put in.
// Suites holds the describe blocks nested in it, as node writes them; after
// Normalise there are none.
type Suite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Name     string   `xml:"name,attr"`
	Time     string   `xml:"time,attr,omitempty"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Cases    []Case   `xml:"testcase"`
	Suites   []Suite  `xml:"testsuite"`
}

// Document is what node writes and what Please reads. The difference between
// the two is Loose.
type Document struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []Suite  `xml:"testsuite"`
	Loose   []Case   `xml:"testcase"`
}

// Normalise moves loose cases into a suite of their own, named after the
// target, flattens nested describe blocks into suites named by their path
// ("outer > inner"), and classnames every case with its suite's name.
//
// The synthetic suite goes first, so a file's own tests are read before the
// describe blocks it also happens to contain -- which is the order they were
// written in often enough to matter when scanning output. Each nested block
// follows its parent, in the order node wrote them.
func Normalise(doc *Document, suiteName string) {
	var flat []Suite
	if len(doc.Loose) > 0 {
		flat = append(flat, suite(suiteName, Suite{Cases: doc.Loose}))
		doc.Loose = nil
	}
	for _, s := range doc.Suites {
		flat = flatten(flat, s.Name, s)
	}
	doc.Suites = flat
}

func flatten(into []Suite, path string, s Suite) []Suite {
	if len(s.Cases) > 0 {
		into = append(into, suite(path, s))
	}
	for _, child := range s.Suites {
		into = flatten(into, path+" > "+child.Name, child)
	}
	return into
}

// suite is s's own cases under name, counted and classnamed by it.
func suite(name string, s Suite) Suite {
	out := Suite{Name: name, Time: s.Time, Tests: len(s.Cases)}
	for _, c := range s.Cases {
		c.Classname = name
		if c.Failure != nil {
			out.Failures++
		}
		out.Cases = append(out.Cases, c)
	}
	return out
}

// Convert reads node's results and writes ones Please can read.
func Convert(in, out, suiteName string) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("reading %s: %w", in, err)
	}
	var doc Document
	if err := xml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", in, err)
	}
	Normalise(&doc, suiteName)

	encoded, err := xml.MarshalIndent(&doc, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append([]byte(xml.Header), append(encoded, '\n')...), 0o644)
}
