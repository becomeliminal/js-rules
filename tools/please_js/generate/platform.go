package generate

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

// Platform is the os/cpu pair a tree is being built for.
//
// npm records these on a package as constraint lists, and every fast JavaScript
// tool now ships this way -- TypeScript 7 is a 2.5MB wrapper plus twenty native
// compilers of ~28MB each, and esbuild, swc and rollup's native pieces do the
// same. Fetching the whole set would mean ~560MB per tree of which one binary
// can run, so the constraints have to be honoured rather than recorded.
type Platform struct {
	OS  string
	CPU string
}

// HostPlatform is the platform of the machine running the build, named the way
// npm names them.
//
// Please's own CONFIG.OS and CONFIG.ARCH use Go's spelling, which agrees with
// npm's for the operating system but not for the architecture: Go says amd64
// where npm says x64.
func HostPlatform() Platform {
	return Platform{OS: runtime.GOOS, CPU: npmArch(runtime.GOARCH)}
}

// ParsePlatform reads an "os/cpu" pair, as Please would pass CONFIG.OS and
// CONFIG.ARCH. An empty string means the host.
func ParsePlatform(s string) (Platform, error) {
	if s == "" {
		return HostPlatform(), nil
	}
	os, cpu, ok := strings.Cut(s, "/")
	if !ok {
		return Platform{}, fmt.Errorf("platform %q is not os/cpu", s)
	}
	return Platform{OS: os, CPU: npmArch(cpu)}, nil
}

func npmArch(arch string) string {
	if arch == "amd64" {
		return "x64"
	}
	return arch
}

func (p Platform) String() string { return p.OS + "/" + p.CPU }

// Supports reports whether a package with these constraints can run here.
//
// An empty constraint list means the package places no restriction, which is
// the common case. A leading '!' negates an entry, which npm allows.
func (p Platform) Supports(os, cpu []string) bool {
	return matches(os, p.OS) && matches(cpu, p.CPU)
}

func matches(constraints []string, actual string) bool {
	if len(constraints) == 0 {
		return true
	}
	// A list of negations is a denylist: anything not named is allowed. A list
	// containing any positive entry is an allowlist.
	allowed, denied := false, false
	positive := false
	for _, c := range constraints {
		if negated, ok := strings.CutPrefix(c, "!"); ok {
			if negated == actual {
				denied = true
			}
			continue
		}
		positive = true
		if c == actual {
			allowed = true
		}
	}
	if denied {
		return false
	}
	return allowed || !positive
}

// Unsupported returns the entries a platform cannot run, so a caller can say
// what it dropped rather than silently omitting packages.
func (p Platform) Unsupported(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		if !p.Supports(e.OS, e.CPU) {
			out = append(out, e.Target)
		}
	}
	sort.Strings(out)
	return out
}
