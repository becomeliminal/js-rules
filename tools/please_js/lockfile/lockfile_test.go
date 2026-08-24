package lockfile_test

import (
	"testing"

	"tools/please_js/lockfile"
)

func TestSplitKey(t *testing.T) {
	// The cases that make this non-trivial: a scope's '@' is not a version
	// separator, and a peer suffix can nest and carry '@' of its own.
	tests := []struct {
		key                  string
		name, version, peers string
	}{
		{"react@18.3.1", "react", "18.3.1", ""},
		{"@aspect-test/c@1.0.0", "@aspect-test/c", "1.0.0", ""},
		{
			"@aspect-test/d@2.0.0(@aspect-test/c@2.0.2)",
			"@aspect-test/d", "2.0.0", "(@aspect-test/c@2.0.2)",
		},
		{
			"@testing-library/react@15.2.6(react-dom@19.2.4(react@19.2.4))(react@19.2.4)",
			"@testing-library/react", "15.2.6",
			"(react-dom@19.2.4(react@19.2.4))(react@19.2.4)",
		},
		{"react-dom@19.2.4(react@19.2.4)", "react-dom", "19.2.4", "(react@19.2.4)"},
	}

	for _, tt := range tests {
		name, version, peers := lockfile.SplitKey(tt.key)
		if name != tt.name || version != tt.version || peers != tt.peers {
			t.Errorf("SplitKey(%q)\n got  name=%q version=%q peers=%q\n want name=%q version=%q peers=%q",
				tt.key, name, version, peers, tt.name, tt.version, tt.peers)
		}
	}
}

func TestIsLink(t *testing.T) {
	if !lockfile.IsLink("link:../lib") {
		t.Error("link:../lib should be a workspace link")
	}
	if lockfile.IsLink("1.0.0") {
		t.Error("a plain version is not a workspace link")
	}
}
