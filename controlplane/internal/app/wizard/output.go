package wizard

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"gopkg.in/yaml.v3"
)

// AskOutput prompts for the output file path and writes the manifest.
// Returns the path written.
func AskOutput(r io.Reader, w io.Writer, defaultPath string, doc manifest.Document) (string, error) {
	path := Prompt(w, r, "Write manifest to", defaultPath)
	if err := WriteManifest(doc, path); err != nil {
		return "", err
	}
	fmt.Fprintf(w, "\n✓ Manifest written to %s\n", path)
	fmt.Fprintf(w, "  Run: vcpe plan --manifest %s\n", path)
	return path, nil
}

// WriteManifest serializes doc to YAML and writes it to path.
func WriteManifest(doc manifest.Document, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create manifest %s: %w", path, err)
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	return enc.Encode(doc)
}

// defaultOutputPath derives a default output path from an existing manifest
// path (adds "-updated" before the extension) or returns "manifest.yaml".
func defaultOutputPath(existingPath, deploymentName string) string {
	if existingPath == "" {
		if deploymentName != "" {
			return deploymentName + ".yaml"
		}
		return "manifest.yaml"
	}
	// Strip .yaml/.yml and add -updated.
	for _, ext := range []string{".yaml", ".yml"} {
		if strings.HasSuffix(existingPath, ext) {
			return strings.TrimSuffix(existingPath, ext) + "-updated" + ext
		}
	}
	return existingPath + "-updated.yaml"
}
