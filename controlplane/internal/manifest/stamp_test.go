package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStampManifestFile(t *testing.T) {
	const input = `# topology comment preserved
apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: test
spec:
  services:
    - name: bng
      type: bng
      image:
        repository: ghcr.io/gdcs-dev/bng
        tag: dev
        buildContext: services/bng
    - name: client
      type: generic-container
      image:
        repository: docker.io/library/alpine
        tag: "3.19"
`

	tmp := filepath.Join(t.TempDir(), "test.yaml")
	if err := os.WriteFile(tmp, []byte(input), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := StampManifestFile(tmp, "v0.1.0"); err != nil {
		t.Fatalf("StampManifestFile: %v", err)
	}

	got, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(got)

	// First-party (has buildContext) tag must be updated.
	if !strings.Contains(s, "tag: v0.1.0") {
		t.Errorf("expected first-party tag v0.1.0 in output:\n%s", s)
	}

	// First-party pullPolicy must be set to always-pull.
	if !strings.Contains(s, "pullPolicy: always-pull") {
		t.Errorf("expected pullPolicy: always-pull in output:\n%s", s)
	}

	// Third-party (no buildContext) tag must be unchanged.
	if !strings.Contains(s, `tag: "3.19"`) && !strings.Contains(s, "tag: 3.19") {
		t.Errorf("expected third-party tag 3.19 to be unchanged in output:\n%s", s)
	}

	// The leading comment must be preserved.
	if !strings.Contains(s, "topology comment preserved") {
		t.Errorf("expected YAML comment to be preserved in output:\n%s", s)
	}
}
func writeStampTestManifest(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestStampManifestFiles_Empty(t *testing.T) {
	if err := StampManifestFiles(nil, "v1.0.0"); err != nil {
		t.Errorf("StampManifestFiles(nil): unexpected error: %v", err)
	}
}

func TestStampManifestFiles_Multiple(t *testing.T) {
	const tmpl = `apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: %s
spec:
  services:
    - name: bng
      type: bng
      image:
        repository: ghcr.io/gdcs-dev/bng
        tag: dev
        buildContext: services/bng
`
	dir := t.TempDir()
	p1 := writeStampTestManifest(t, dir, "a.yaml", fmt.Sprintf(tmpl, "a"))
	p2 := writeStampTestManifest(t, dir, "b.yaml", fmt.Sprintf(tmpl, "b"))

	if err := StampManifestFiles([]string{p1, p2}, "v2.0.0"); err != nil {
		t.Fatalf("StampManifestFiles: %v", err)
	}
	for _, p := range []string{p1, p2} {
		data, _ := os.ReadFile(p)
		if !strings.Contains(string(data), "tag: v2.0.0") {
			t.Errorf("%s not stamped; got:\n%s", p, data)
		}
	}
}

func TestStampManifestFiles_OneFailStops(t *testing.T) {
	dir := t.TempDir()
	p1 := writeStampTestManifest(t, dir, "good.yaml", `apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: good
spec:
  services:
    - name: bng
      type: bng
      image:
        repository: ghcr.io/gdcs-dev/bng
        tag: dev
        buildContext: services/bng
`)
	missing := filepath.Join(dir, "missing.yaml")
	p3 := writeStampTestManifest(t, dir, "other.yaml", `apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: other
spec:
  services: []
`)

	err := StampManifestFiles([]string{p1, missing, p3}, "v3.0.0")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	// p3 should NOT be stamped because we stopped at missing.
	data, _ := os.ReadFile(p3)
	if strings.Contains(string(data), "v3.0.0") {
		t.Errorf("p3 should not have been stamped after failure")
	}
}
