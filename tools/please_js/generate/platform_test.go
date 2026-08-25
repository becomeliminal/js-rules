package generate_test

import (
	"testing"

	"tools/please_js/generate"
)

func TestSupports(t *testing.T) {
	linux := generate.Platform{OS: "linux", CPU: "x64"}
	darwin := generate.Platform{OS: "darwin", CPU: "arm64"}

	tests := []struct {
		name          string
		os, cpu       []string
		onLinux, onMac bool
	}{
		{"unconstrained", nil, nil, true, true},
		{"os only", []string{"linux"}, nil, true, false},
		{"cpu only", nil, []string{"x64"}, true, false},
		{"both", []string{"linux"}, []string{"x64"}, true, false},
		{"several", []string{"linux", "darwin"}, []string{"x64", "arm64"}, true, true},
		{"mismatched pair", []string{"linux"}, []string{"arm64"}, false, false},
		// npm allows negation, which reads as a denylist.
		{"negated", []string{"!win32"}, nil, true, true},
		{"negated self", []string{"!linux"}, nil, false, true},
	}

	for _, tt := range tests {
		if got := linux.Supports(tt.os, tt.cpu); got != tt.onLinux {
			t.Errorf("%s on linux/x64: got %v, want %v", tt.name, got, tt.onLinux)
		}
		if got := darwin.Supports(tt.os, tt.cpu); got != tt.onMac {
			t.Errorf("%s on darwin/arm64: got %v, want %v", tt.name, got, tt.onMac)
		}
	}
}

// Please spells the architecture the way Go does; npm does not.
func TestArchitectureSpelling(t *testing.T) {
	p, err := generate.ParsePlatform("linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if p.CPU != "x64" {
		t.Errorf("amd64 should be read as npm's x64, got %q", p.CPU)
	}
	if !p.Supports([]string{"linux"}, []string{"x64"}) {
		t.Error("a linux/x64 package should run on linux/amd64")
	}
}

func TestParsePlatformRejectsNonsense(t *testing.T) {
	if _, err := generate.ParsePlatform("linux"); err == nil {
		t.Error("a platform without a cpu should be rejected, not guessed at")
	}
}
