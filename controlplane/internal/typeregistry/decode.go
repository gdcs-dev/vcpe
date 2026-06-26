package typeregistry

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// StrictDecode decodes an opaque config subtree into a typed struct, rejecting
// unknown fields. An empty/absent config node decodes to the zero value, which
// lets types with no config (or all-optional config) accept an empty config.
func StrictDecode(node yaml.Node, out any) error {
	if node.Kind == 0 {
		return nil
	}
	raw, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Errorf("re-encode config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}
