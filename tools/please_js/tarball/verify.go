package tarball

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// Verify checks a file against a subresource integrity string, as a lockfile
// records it: <algorithm>-<base64 digest>, almost always sha512.
func Verify(file, integrity string) error {
	algo, want, ok := strings.Cut(integrity, "-")
	if !ok {
		return fmt.Errorf("integrity %q is not <algorithm>-<base64>", integrity)
	}
	var h hash.Hash
	switch algo {
	case "sha512":
		h = sha512.New()
	case "sha384":
		h = sha512.New384()
	case "sha256":
		h = sha256.New()
	case "sha1":
		h = sha1.New()
	default:
		return fmt.Errorf("integrity %q uses %s, which please_js does not verify", integrity, algo)
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := base64.StdEncoding.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s does not match its lockfile integrity: want %s-%s, got %s-%s", file, algo, want, algo, got)
	}
	return nil
}
