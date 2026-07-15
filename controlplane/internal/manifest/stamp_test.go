package manifest

import (
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

	// Third-party (no buildContext) tag must be unchanged.
	if !strings.Contains(s, `tag: "3.19"`) && !strings.Contains(s, "tag: 3.19") {
		t.Errorf("expected third-party tag 3.19 to be unchanged in output:\n%s", s)
	}

	// The leading comment must be preserved.
	if !strings.Contains(s, "topology comment preserved") {
		t.Errorf("expected YAML comment to be preserved in output:\n%s", s)
	}
}
