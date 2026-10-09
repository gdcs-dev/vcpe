package secrets

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWirelessCredentialPathIsStableAndOutsideArtifacts(t *testing.T) {
	stateRoot := t.TempDir()
	first := WirelessCredentialPath(stateRoot, "edge", "home")
	second := WirelessCredentialPath(stateRoot, "edge", "home")
	if first != second {
		t.Fatalf("credential path is not stable: %q != %q", first, second)
	}
	artifactRoot := filepath.Join(stateRoot, "artifacts") + string(filepath.Separator)
	if strings.HasPrefix(first, artifactRoot) {
		t.Fatalf("credential path is inside artifact history: %q", first)
	}
}

func TestWirelessCredentialContainerPathSurvivesSystemdRuntimeMounts(t *testing.T) {
	path := WirelessCredentialContainerPath("home")
	if !strings.HasPrefix(path, "/etc/vcpe/credentials/wireless/") {
		t.Fatalf("container credential path = %q, want /etc/vcpe/credentials/wireless root", path)
	}
}

func TestWirelessCredentialPathCannotTraverseRuntimeRoot(t *testing.T) {
	stateRoot := t.TempDir()
	root := RuntimeCredentialsRoot(stateRoot)
	path := WirelessCredentialPath(stateRoot, "../../operations", "../wireless/../../escape")
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("credential path escaped runtime root: root=%q path=%q", root, path)
	}
	if strings.Contains(relative, "operations") || strings.Contains(relative, "escape") {
		t.Fatalf("credential path retained unsafe raw components: %q", relative)
	}
}
