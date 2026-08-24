package generate

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Hasher resolves the download hash for each entry.
//
// A lockfile's integrity field is sha512, and Please verifies sha1 or sha256,
// so the lockfile's hash cannot be handed over as-is. Each tarball is fetched
// once here: its sha512 is checked against the lockfile, which is the actual
// supply-chain guarantee, and its sha256 is recorded for Please to enforce on
// every later fetch. This is the same trade rust-rules makes -- an occasional
// slow update in exchange for verified builds.
type Hasher struct {
	Registry string
	Workers  int

	mu    sync.Mutex
	cache map[string]string
}

// TarballURL is the registry path for a package version. Scoped packages live
// under the scope, but the tarball itself is named without it.
func TarballURL(registry, pkg, version string) string {
	base := pkg
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		base = pkg[i+1:]
	}
	return fmt.Sprintf("%s/%s/-/%s-%s.tgz", registry, pkg, base, version)
}

// Resolve fills in the Sha256 of every entry, concurrently.
func (h *Hasher) Resolve(entries []Entry, progress func(done, total int)) ([]string, error) {
	if h.cache == nil {
		h.cache = map[string]string{}
	}
	workers := h.Workers
	if workers <= 0 {
		workers = 8
	}

	type job struct{ i int }
	jobs := make(chan job)
	sums := make([]string, len(entries))
	errs := make([]error, len(entries))

	var wg sync.WaitGroup
	var done int
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				e := entries[j.i]
				url := TarballURL(h.Registry, e.Package, e.Version)

				h.mu.Lock()
				cached, ok := h.cache[url]
				h.mu.Unlock()
				if ok {
					sums[j.i] = cached
				} else {
					sum, err := fetchAndHash(url, e.Integrity)
					if err != nil {
						errs[j.i] = fmt.Errorf("%s@%s: %w", e.Package, e.Version, err)
					} else {
						sums[j.i] = sum
						h.mu.Lock()
						h.cache[url] = sum
						h.mu.Unlock()
					}
				}

				h.mu.Lock()
				done++
				if progress != nil {
					progress(done, len(entries))
				}
				h.mu.Unlock()
			}
		}()
	}
	for i := range entries {
		jobs <- job{i}
	}
	close(jobs)
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return sums, nil
}

// fetchAndHash downloads a tarball, checks it against the lockfile's integrity
// where there is one, and returns its sha256 in hex.
func fetchAndHash(url, integrity string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	s256 := sha256.New()
	s512 := sha512.New()
	if _, err := io.Copy(io.MultiWriter(s256, s512), resp.Body); err != nil {
		return "", err
	}

	if want, ok := strings.CutPrefix(integrity, "sha512-"); ok {
		got := base64.StdEncoding.EncodeToString(s512.Sum(nil))
		if got != want {
			// The lockfile says one thing and the registry served another.
			// Failing here is the whole point of recording integrity.
			return "", fmt.Errorf(
				"integrity mismatch for %s\n  lockfile: sha512-%s\n  registry: sha512-%s",
				url, want, got)
		}
	}
	return hex.EncodeToString(s256.Sum(nil)), nil
}
