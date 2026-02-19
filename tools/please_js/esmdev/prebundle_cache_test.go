package esmdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrebundleCacheKey(t *testing.T) {
	dir := t.TempDir()

	mc1 := filepath.Join(dir, "moduleconfig1")
	if err := os.WriteFile(mc1, []byte("react=/path/to/react\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mc2 := filepath.Join(dir, "moduleconfig2")
	if err := os.WriteFile(mc2, []byte("react=/different/path\n"), 0644); err != nil {
		t.Fatal(err)
	}

	importsA := map[string]bool{"react": true, "lodash": true}
	importsB := map[string]bool{"react": true, "vue": true}

	t.Run("same inputs produce same key", func(t *testing.T) {
		key1 := prebundleCacheKey(mc1, importsA)
		key2 := prebundleCacheKey(mc1, importsA)
		if key1 != key2 {
			t.Errorf("same inputs gave different keys: %q vs %q", key1, key2)
		}
	})

	t.Run("different imports produce different key", func(t *testing.T) {
		key1 := prebundleCacheKey(mc1, importsA)
		key2 := prebundleCacheKey(mc1, importsB)
		if key1 == key2 {
			t.Errorf("different imports gave same key: %q", key1)
		}
	})

	t.Run("different moduleconfig produces different key", func(t *testing.T) {
		key1 := prebundleCacheKey(mc1, importsA)
		key2 := prebundleCacheKey(mc2, importsA)
		if key1 == key2 {
			t.Errorf("different moduleconfigs gave same key: %q", key1)
		}
	})

	t.Run("key is 16 hex characters", func(t *testing.T) {
		key := prebundleCacheKey(mc1, importsA)
		if len(key) != 16 {
			t.Errorf("expected key length 16, got %d (%q)", len(key), key)
		}
		for _, c := range key {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("key contains non-hex character %q in %q", string(c), key)
				break
			}
		}
	})
}

func TestMergeImportmaps(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "importmap1.json")
	data1, _ := json.Marshal(map[string]interface{}{
		"imports": map[string]string{"react": "/@deps/react.js"},
	})
	if err := os.WriteFile(file1, data1, 0644); err != nil {
		t.Fatal(err)
	}

	file2 := filepath.Join(dir, "importmap2.json")
	data2, _ := json.Marshal(map[string]interface{}{
		"imports": map[string]string{"lodash": "/@deps/lodash.js"},
	})
	if err := os.WriteFile(file2, data2, 0644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "out", "merged.json")

	if err := MergeImportmaps([]string{file1, file2}, outPath); err != nil {
		t.Fatalf("MergeImportmaps() error: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	var result struct {
		Imports map[string]string `json:"imports"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if got, want := result.Imports["react"], "/@deps/react.js"; got != want {
		t.Errorf("imports[react] = %q, want %q", got, want)
	}
	if got, want := result.Imports["lodash"], "/@deps/lodash.js"; got != want {
		t.Errorf("imports[lodash] = %q, want %q", got, want)
	}
	// MergeImportmaps now adds prefix entries: "react/" and "lodash/"
	if got, want := result.Imports["react/"], "/@deps/react/"; got != want {
		t.Errorf("imports[react/] = %q, want %q", got, want)
	}
	if got, want := result.Imports["lodash/"], "/@deps/lodash/"; got != want {
		t.Errorf("imports[lodash/] = %q, want %q", got, want)
	}

	t.Run("later file wins on conflicts", func(t *testing.T) {
		// file3 has a conflicting "react" entry
		file3 := filepath.Join(dir, "importmap3.json")
		data3, _ := json.Marshal(map[string]interface{}{
			"imports": map[string]string{"react": "/@deps/react-v2.js"},
		})
		if err := os.WriteFile(file3, data3, 0644); err != nil {
			t.Fatal(err)
		}

		outPath2 := filepath.Join(dir, "out", "merged2.json")
		if err := MergeImportmaps([]string{file1, file3}, outPath2); err != nil {
			t.Fatalf("MergeImportmaps() error: %v", err)
		}

		raw2, err := os.ReadFile(outPath2)
		if err != nil {
			t.Fatalf("failed to read output: %v", err)
		}

		var result2 struct {
			Imports map[string]string `json:"imports"`
		}
		if err := json.Unmarshal(raw2, &result2); err != nil {
			t.Fatalf("failed to parse output: %v", err)
		}

		if got, want := result2.Imports["react"], "/@deps/react-v2.js"; got != want {
			t.Errorf("expected later file to win: imports[react] = %q, want %q", got, want)
		}
	})
}

func TestAddPrefixImportMapEntries(t *testing.T) {
	t.Run("adds prefix entries for each package", func(t *testing.T) {
		importMap := map[string]string{
			"react":            "/@deps/react.js",
			"react-dom":        "/@deps/react-dom.js",
			"react-dom/client": "/@deps/react-dom/client.js",
		}
		addPrefixImportMapEntries(importMap)

		if got, want := importMap["react/"], "/@deps/react/"; got != want {
			t.Errorf("importMap[\"react/\"] = %q, want %q", got, want)
		}
		if got, want := importMap["react-dom/"], "/@deps/react-dom/"; got != want {
			t.Errorf("importMap[\"react-dom/\"] = %q, want %q", got, want)
		}
	})

	t.Run("does not overwrite existing prefix entries", func(t *testing.T) {
		importMap := map[string]string{
			"react":  "/@deps/react.js",
			"react/": "/@deps/custom-react/",
		}
		addPrefixImportMapEntries(importMap)

		if got, want := importMap["react/"], "/@deps/custom-react/"; got != want {
			t.Errorf("existing prefix overwritten: importMap[\"react/\"] = %q, want %q", got, want)
		}
	})

	t.Run("does not overwrite exact entries", func(t *testing.T) {
		importMap := map[string]string{
			"react":            "/@deps/react.js",
			"react-dom/client": "/@deps/react-dom/client.js",
		}
		addPrefixImportMapEntries(importMap)

		if got, want := importMap["react"], "/@deps/react.js"; got != want {
			t.Errorf("exact entry modified: importMap[\"react\"] = %q, want %q", got, want)
		}
		if got, want := importMap["react-dom/client"], "/@deps/react-dom/client.js"; got != want {
			t.Errorf("exact entry modified: importMap[\"react-dom/client\"] = %q, want %q", got, want)
		}
	})

	t.Run("handles scoped packages", func(t *testing.T) {
		importMap := map[string]string{
			"@scope/pkg":      "/@deps/@scope/pkg.js",
			"@scope/pkg/util": "/@deps/@scope/pkg/util.js",
		}
		addPrefixImportMapEntries(importMap)

		if got, want := importMap["@scope/pkg/"], "/@deps/@scope/pkg/"; got != want {
			t.Errorf("importMap[\"@scope/pkg/\"] = %q, want %q", got, want)
		}
	})

	t.Run("empty import map is no-op", func(t *testing.T) {
		importMap := map[string]string{}
		addPrefixImportMapEntries(importMap)
		if len(importMap) != 0 {
			t.Errorf("expected empty map, got %d entries", len(importMap))
		}
	})
}
