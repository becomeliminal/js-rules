package tsconfig

import (
	"strings"
	"testing"
)

func TestAgreementPasses(t *testing.T) {
	// rootDir declared in the chain and identical to the rule's: no complaint.
	js := []byte(`{"compilerOptions": {"rootDir": "./src"}}`)
	if err := Check(js, "pkg/tsconfig.json", "pkg/src"); err != nil {
		t.Fatalf("agreement should pass: %v", err)
	}
}

func TestSilenceIsAgreement(t *testing.T) {
	// A config that says nothing cannot disagree.
	if err := Check([]byte(`{}`), "pkg/tsconfig.json", "pkg"); err != nil {
		t.Fatalf("no mirrored options means nothing to check: %v", err)
	}
}

func TestRootDirMismatchNamesBothPaths(t *testing.T) {
	js := []byte(`{"compilerOptions": {"rootDir": "./src"}}`)
	err := Check(js, "pkg/tsconfig.json", "pkg")
	if err == nil {
		t.Fatal("a rootDir the build ignores must be an error")
	}
	for _, want := range []string{"pkg/src", "--rootDir pkg", "command line wins"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message should contain %q:\n%v", want, err)
		}
	}
}

func TestInheritedRootDirResolvesAgainstConfigDir(t *testing.T) {
	// What showConfig prints for a base two directories away, verified against
	// tsc 5.9: relative to the -p config, not to the base that declared it.
	js := []byte(`{"compilerOptions": {"rootDir": "../base/src"}}`)
	if err := Check(js, "pkg/tsconfig.json", "base/src"); err != nil {
		t.Fatalf("inherited agreement should pass: %v", err)
	}
	if err := Check(js, "pkg/tsconfig.json", "pkg"); err == nil {
		t.Fatal("inherited disagreement must still be an error")
	}
}

func TestOutDirIsAlwaysAnError(t *testing.T) {
	js := []byte(`{"compilerOptions": {"outDir": "./dist"}}`)
	err := Check(js, "pkg/tsconfig.json", "pkg")
	if err == nil {
		t.Fatal("an outDir the build ignores must be an error")
	}
	if !strings.Contains(err.Error(), "rule owns the output directory") {
		t.Errorf("the message should say why: %v", err)
	}
}

func TestRootRuleAtRepoRoot(t *testing.T) {
	// An empty root means the repo root; "." is what config-relative paths
	// resolve to there.
	js := []byte(`{"compilerOptions": {"rootDir": "."}}`)
	if err := Check(js, "tsconfig.json", ""); err != nil {
		t.Fatalf("repo-root agreement should pass: %v", err)
	}
}
