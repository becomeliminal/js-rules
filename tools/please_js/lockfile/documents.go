package lockfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// workspaceDocument returns the YAML document of a pnpm lockfile that
// describes the workspace's projects.
//
// A lockfile is usually one document. When package.json pins pnpm itself
// (packageManager), pnpm 10+ writes a second one ahead of it: the environment
// lockfile, whose importers record pnpm and its per-platform executables under
// packageManagerDependencies or configDependencies rather than any project's
// dependencies. Reading only the first document would silently see pnpm in
// place of the workspace, so the environment document is skipped -- which
// also keeps pnpm's own binaries out of the packages a build stages.
func workspaceDocument(data []byte, path string) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		docs = append(docs, &doc)
	}
	var workspace []*yaml.Node
	for _, doc := range docs {
		if !isEnvironment(doc) {
			workspace = append(workspace, doc)
		}
	}
	switch len(workspace) {
	case 1:
		return workspace[0], nil
	case 0:
		return nil, fmt.Errorf("%s has no workspace document, only pnpm's own environment", path)
	default:
		return nil, fmt.Errorf("%s has %d workspace documents; expected one", path, len(workspace))
	}
}

// isEnvironment reports whether a document is pnpm's environment lockfile:
// every importer it lists carries only packageManagerDependencies and
// configDependencies.
func isEnvironment(doc *yaml.Node) bool {
	importers := mappingValue(rootMapping(doc), "importers")
	if importers == nil || len(importers.Content) == 0 {
		return false
	}
	for i := 1; i < len(importers.Content); i += 2 {
		project := importers.Content[i]
		if project.Kind != yaml.MappingNode || len(project.Content) == 0 {
			return false
		}
		for j := 0; j < len(project.Content); j += 2 {
			switch project.Content[j].Value {
			case "packageManagerDependencies", "configDependencies":
			default:
				return false
			}
		}
	}
	return true
}

func rootMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		return doc.Content[0]
	}
	return nil
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
