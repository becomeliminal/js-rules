package esmdev

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// tailwindEntry caches compiled Tailwind CSS output keyed by source file path.
type tailwindEntry struct {
	css     string
	modTime time.Time // mtime of the source CSS file at compile time
}

// compileTailwind runs the Tailwind CLI binary on a CSS file and caches the result.
// The cache is invalidated when the source CSS file's mtime changes.
func (s *esmServer) compileTailwind(cssPath string) (string, error) {
	info, err := os.Stat(cssPath)
	if err != nil {
		return "", err
	}

	// Check cache — invalidate if source CSS file changed
	if cached, ok := s.tailwindCache.Load(cssPath); ok {
		entry := cached.(*tailwindEntry)
		if entry.modTime.Equal(info.ModTime()) {
			return entry.css, nil
		}
	}

	cmdArgs := []string{"--input", cssPath}
	if s.tailwindConfig != "" {
		cmdArgs = append(cmdArgs, "--config", filepath.Base(s.tailwindConfig))
	}

	cmd := exec.Command(s.tailwindBin, cmdArgs...)
	if s.tailwindConfig != "" {
		cmd.Dir = filepath.Dir(s.tailwindConfig)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tailwind: %v\n%s", err, stderr.String())
	}

	result := stdout.String()
	s.tailwindCache.Store(cssPath, &tailwindEntry{
		css:     result,
		modTime: info.ModTime(),
	})
	return result, nil
}

// clearTailwindCache removes all entries from the tailwind cache.
// Called on any source file change since new Tailwind classes may have been added.
func (s *esmServer) clearTailwindCache() {
	if s.tailwindBin == "" {
		return
	}
	s.tailwindCache.Range(func(key, _ any) bool {
		s.tailwindCache.Delete(key)
		return true
	})
}
