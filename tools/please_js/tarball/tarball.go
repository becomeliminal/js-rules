// Package tarball extracts npm package tarballs the way npm and pnpm do.
package tarball

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extract unpacks a gzipped npm tarball into dst.
//
// Modes are normalised rather than restored: published tarballs record
// directories without the owner's write bit often enough that restoring them
// verbatim makes the extraction fail partway, writing a file into a directory
// it has just created read-only. Directories become 0755 and files 0644, or
// 0755 when any execute bit was set -- which is what npm and pnpm produce.
//
// An entry that would land outside dst -- an absolute path, a .. component,
// or a link pointing out of the tree -- is refused: a package is untrusted
// input, and nothing it contains may write anywhere but its own directory.
func Extract(file, dst string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%s is not a gzipped tarball: %w", file, err)
	}
	defer gz.Close()

	root, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	inside := func(name string) (string, error) {
		clean := filepath.Clean(filepath.FromSlash(name))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("%s: entry %q would be written outside the package", file, name)
		}
		return filepath.Join(root, clean), nil
	}

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}
		path, err := inside(h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.FileInfo().Mode()&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Resolved against the link's own directory, as the OS will.
			if _, err := inside(filepath.Join(filepath.Dir(h.Name), h.Linkname)); err != nil || filepath.IsAbs(h.Linkname) {
				return fmt.Errorf("%s: link %q -> %q points outside the package", file, h.Name, h.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, path); err != nil {
				return err
			}
		default:
			// Hard links, devices, FIFOs: never part of a published package's
			// contents worth installing, and each is a way to escape.
			continue
		}
	}
}
