package junit

import (
	"fmt"
	"strings"
)

// LcovToGoCover converts node's lcov output to the go-cover format Please
// parses natively -- the same translation please_rust does for llvm profiles,
// because Please reads one coverage format and every toolchain speaks another.
//
// stripPrefix is the run directory: the suite stages files at their
// repo-relative paths inside it, so stripping it turns an absolute SF path
// back into the path Please knows the file by. Anything under node_modules is
// dropped -- third-party coverage is noise -- and so is anything that stays
// absolute after stripping, which is a file outside the repo (node internals,
// the runner itself).
// covmap maps a package name to its source directory, from the overlay's
// covmap.json. A staged first-party library lives at node_modules/<package>/,
// and Please excludes a test's own srcs from reports anyway, so the mapped
// libraries are most of what coverage is for.
func LcovToGoCover(lcov string, stripPrefixes []string, covmap map[string]string) string {
	strip := func(path string) string {
		for _, p := range stripPrefixes {
			if p == "" {
				continue
			}
			if !strings.HasSuffix(p, "/") {
				p += "/"
			}
			path = strings.TrimPrefix(path, p)
		}
		return path
	}
	var b strings.Builder
	b.WriteString("mode: count\n")
	current := ""
	for _, line := range strings.Split(lcov, "\n") {
		if path, ok := strings.CutPrefix(line, "SF:"); ok {
			rel := strip(path)
			if strings.HasPrefix(rel, "/") {
				current = ""
				continue
			}
			if inTree, ok := strings.CutPrefix(rel, "node_modules/"); ok {
				current = ""
				for pkg, srcDir := range covmap {
					if file, ok := strings.CutPrefix(inTree, pkg+"/"); ok {
						current = srcDir + "/" + file
						break
					}
				}
				continue
			}
			current = rel
		} else if da, ok := strings.CutPrefix(line, "DA:"); ok && current != "" {
			if ln, count, ok := strings.Cut(da, ","); ok {
				fmt.Fprintf(&b, "%s:%s.1,%s.999 1 %s\n", current, ln, ln, strings.TrimSpace(count))
			}
		} else if line == "end_of_record" {
			current = ""
		}
	}
	return b.String()
}
