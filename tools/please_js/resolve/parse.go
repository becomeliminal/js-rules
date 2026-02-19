package resolve

import (
	"encoding/json"
	"fmt"
	"os"
)

// packageLock represents the top-level structure of package-lock.json (v3).
type packageLock struct {
	LockfileVersion int                    `json:"lockfileVersion"`
	Packages        map[string]packageInfo `json:"packages"`
}

// peerDepMeta holds metadata for a single peer dependency entry.
type peerDepMeta struct {
	Optional bool `json:"optional"`
}

// packageInfo represents a single package entry in the lockfile.
type packageInfo struct {
	Version              string                 `json:"version"`
	Resolved             string                 `json:"resolved"`
	Integrity            string                 `json:"integrity"`
	Dependencies         map[string]string      `json:"dependencies"`
	PeerDependencies     map[string]string      `json:"peerDependencies"`
	PeerDependenciesMeta map[string]peerDepMeta `json:"peerDependenciesMeta"`
	Dev                  bool                   `json:"dev"`
	Optional             bool                   `json:"optional"`
	OS                   []string               `json:"os"`
	CPU                  []string               `json:"cpu"`
}

// parseLockfile reads and parses a package-lock.json file.
func parseLockfile(path string) (*packageLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lockfile: %w", err)
	}

	var lock packageLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse lockfile: %w", err)
	}

	if lock.LockfileVersion != 3 && lock.LockfileVersion != 2 {
		return nil, fmt.Errorf("unsupported lockfile version %d (expected 2 or 3)", lock.LockfileVersion)
	}

	return &lock, nil
}
