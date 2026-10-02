package lockfile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Slice writes the part of a pnpm v9 lockfile one workspace project needs: its
// own importer entry, and the packages and snapshots its tree reaches.
//
// A project's node_modules depends on this rather than on the whole lockfile,
// because the whole lockfile changes whenever any project's dependencies do.
// A slice changes only when this project's own resolution does, so a bump in
// one app leaves every other app's tree a cache hit instead of a relink.
//
// The slice is cut from the document as written rather than re-encoded from
// the parsed form, so every field pnpm recorded survives byte for byte, and
// two runs over the same lockfile write the same slice.
func Slice(path, project string, snapshotKeys []string, out string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading lockfile: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a pnpm lockfile", path)
	}

	snapshots := map[string]struct{}{}
	packages := map[string]struct{}{}
	for _, key := range snapshotKeys {
		snapshots[key] = struct{}{}
		name, version, _ := SplitKey(key)
		packages[name+"@"+version] = struct{}{}
	}
	keep := func(section string) func(string) bool {
		switch section {
		case "importers":
			return func(k string) bool { return k == project }
		case "packages":
			return func(k string) bool { _, ok := packages[k]; return ok }
		case "snapshots":
			return func(k string) bool { _, ok := snapshots[k]; return ok }
		}
		return nil
	}

	root := doc.Content[0]
	foundProject := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		section := root.Content[i].Value
		filter := keep(section)
		value := root.Content[i+1]
		if filter == nil || value.Kind != yaml.MappingNode {
			continue
		}
		kept := value.Content[:0:0]
		for j := 0; j+1 < len(value.Content); j += 2 {
			if filter(value.Content[j].Value) {
				kept = append(kept, value.Content[j], value.Content[j+1])
				if section == "importers" {
					foundProject = true
				}
			}
		}
		value.Content = kept
	}
	if !foundProject {
		return fmt.Errorf("%s has no project %q", path, project)
	}

	sliced, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(out, sliced, 0o644)
}
