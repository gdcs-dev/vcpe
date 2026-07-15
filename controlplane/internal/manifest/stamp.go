package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StampManifestFile rewrites the manifest at path, updating the image.tag field
// to version for every first-party service (those with a non-empty buildContext).
// Third-party images (no buildContext) are left unchanged.
//
// The file is rewritten using the gopkg.in/yaml.v3 Node API so all comments
// and formatting are preserved.
func StampManifestFile(path, version string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("stamp manifest: read %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("stamp manifest: parse %s: %w", path, err)
	}
	if err := stampServices(&root, version); err != nil {
		return fmt.Errorf("stamp manifest: %w", err)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("stamp manifest: marshal: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("stamp manifest: write %s: %w", path, err)
	}
	return nil
}

// stampServices walks the YAML node tree and sets image.tag = version for each
// service whose image.buildContext is non-empty.
func stampServices(root *yaml.Node, version string) error {
	// root is a DocumentNode; Content[0] is the top-level MappingNode.
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	spec := nodeGet(root.Content[0], "spec")
	if spec == nil {
		return nil
	}
	services := nodeGet(spec, "services")
	if services == nil || services.Kind != yaml.SequenceNode {
		return nil
	}
	for _, svc := range services.Content {
		if svc.Kind != yaml.MappingNode {
			continue
		}
		img := nodeGet(svc, "image")
		if img == nil || img.Kind != yaml.MappingNode {
			continue
		}
		// Only stamp first-party images (those that are built, not just pulled).
		if nodeValue(img, "buildContext") == "" {
			continue
		}
		nodeSet(img, "tag", version)
	}
	return nil
}

// nodeGet returns the value node for key in a MappingNode, or nil.
func nodeGet(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// nodeValue returns the scalar string value of key in a MappingNode, or "".
func nodeValue(m *yaml.Node, key string) string {
	n := nodeGet(m, key)
	if n == nil {
		return ""
	}
	return n.Value
}

// nodeSet updates the value of key in a MappingNode. If the key does not exist
// it is appended.
func nodeSet(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].Value = value
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}
