package tarball_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tools/please_js/tarball"
)

type entry struct {
	name, body, link string
	mode             int64
	kind             byte
}

func write(t *testing.T, entries []entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: e.mode, Typeflag: e.kind, Size: int64(len(e.body)), Linkname: e.link}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.kind == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	os.WriteFile(path, buf.Bytes(), 0o644)
	return path
}

func TestExtractWritesIntoADirectoryTheTarballRecordsReadOnly(t *testing.T) {
	// GIVEN a tarball, as published, whose dist directory is recorded 0555 and
	// holds a file, plus an executable bin script
	file := write(t, []entry{
		{name: "package/dist/", mode: 0o555, kind: tar.TypeDir},
		{name: "package/dist/index.js", body: "// index", mode: 0o444, kind: tar.TypeReg},
		{name: "package/bin/cli", body: "#!/bin/sh", mode: 0o755, kind: tar.TypeReg},
	})
	dst := filepath.Join(t.TempDir(), "out")

	// WHEN it is extracted
	if err := tarball.Extract(file, dst); err != nil {
		t.Fatal(err)
	}

	// THEN the file is there, the directory is writable, and the script stays
	// executable
	if data, _ := os.ReadFile(filepath.Join(dst, "package/dist/index.js")); string(data) != "// index" {
		t.Errorf("dist/index.js = %q", data)
	}
	if fi, _ := os.Stat(filepath.Join(dst, "package/dist")); fi.Mode().Perm() != 0o755 {
		t.Errorf("dist mode = %v, want 0755", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Join(dst, "package/bin/cli")); fi.Mode().Perm()&0o111 == 0 {
		t.Error("bin/cli lost its execute bit")
	}
}

func TestExtractRefusesEntriesThatEscapeThePackage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry entry
	}{
		{"parent directory", entry{name: "package/../../evil", body: "x", mode: 0o644, kind: tar.TypeReg}},
		{"absolute path", entry{name: "/tmp/evil", body: "x", mode: 0o644, kind: tar.TypeReg}},
		{"link out of the tree", entry{name: "package/link", link: "../../../etc/passwd", kind: tar.TypeSymlink}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN a tarball with an entry pointing outside the package
			file := write(t, []entry{tc.entry})

			// WHEN it is extracted
			err := tarball.Extract(file, filepath.Join(t.TempDir(), "out"))

			// THEN it is refused, naming the entry
			if err == nil || !strings.Contains(err.Error(), "outside the package") {
				t.Errorf("expected a refusal, got %v", err)
			}
		})
	}
}
