package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeWirelessCredentialCreatesPrivateAtomicFile(t *testing.T) {
	stateRoot := t.TempDir()
	first, err := MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("first-passphrase"))
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if !first.Changed {
		t.Fatal("first materialization was not marked changed")
	}
	assertMode(t, filepath.Dir(first.Path), 0o700)
	assertMode(t, first.Path, 0o600)

	unchanged, err := MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("first-passphrase"))
	if err != nil {
		t.Fatalf("unchanged materialize: %v", err)
	}
	if unchanged.Changed || unchanged.Path != first.Path {
		t.Fatalf("unchanged result = %+v", unchanged)
	}

	changed, err := MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("second-passphrase"))
	if err != nil {
		t.Fatalf("changed materialize: %v", err)
	}
	if !changed.Changed || changed.Path != first.Path {
		t.Fatalf("changed result = %+v", changed)
	}
	value, err := os.ReadFile(changed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "second-passphrase" {
		t.Fatalf("credential value = %q", value)
	}
	entries, err := os.ReadDir(filepath.Dir(changed.Path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "passphrase" {
		t.Fatalf("credential directory entries = %+v", entries)
	}
}

func TestMaterializeWirelessCredentialRejectsSymlinks(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		stateRoot := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(stateRoot, "credentials")); err != nil {
			t.Fatal(err)
		}
		if _, err := MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("valid-passphrase")); err == nil {
			t.Fatal("expected symlinked directory rejection")
		}
	})

	t.Run("file", func(t *testing.T) {
		stateRoot := t.TempDir()
		target := WirelessCredentialPath(stateRoot, "edge", "home")
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("do-not-replace"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, target); err != nil {
			t.Fatal(err)
		}
		if _, err := MaterializeWirelessCredential(stateRoot, "edge", "home", []byte("valid-passphrase")); err == nil {
			t.Fatal("expected symlinked file rejection")
		}
	})
}

func TestMaterializeMeshCredentialIsPrivateAndRejectsSymlink(t *testing.T) {
	stateRoot := t.TempDir()
	first, err := MaterializeMeshCredential(stateRoot, "edge", "mesh-home", []byte("private-mesh-key"))
	if err != nil || !first.Changed {
		t.Fatalf("materialize mesh credential = %+v, %v", first, err)
	}
	assertMode(t, filepath.Dir(first.Path), 0o700)
	assertMode(t, first.Path, 0o600)
	if first.Path != MeshCredentialPath(stateRoot, "edge", "mesh-home") {
		t.Fatalf("mesh credential path = %s", first.Path)
	}
	containerPath := MeshCredentialContainerPath("mesh-home")
	if !strings.HasPrefix(containerPath, "/etc/vcpe/credentials/mesh/") || strings.Contains(containerPath, "mesh-home") {
		t.Fatalf("unsafe container path = %s", containerPath)
	}
	if err := os.Remove(first.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), first.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeMeshCredential(stateRoot, "edge", "mesh-home", []byte("private-mesh-key")); err == nil {
		t.Fatal("accepted symlink at mesh credential path")
	}
}

func TestCredentialCleanupAndErrorRedaction(t *testing.T) {
	stateRoot := t.TempDir()
	for _, network := range []string{"home", "obsolete"} {
		if _, err := MaterializeWirelessCredential(stateRoot, "edge", network, []byte("sentinel-passphrase")); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveObsoleteWirelessCredentials(stateRoot, "edge", []string{"home"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(WirelessCredentialPath(stateRoot, "edge", "home")); err != nil {
		t.Fatalf("kept credential missing: %v", err)
	}
	if _, err := os.Stat(WirelessCredentialPath(stateRoot, "edge", "obsolete")); !os.IsNotExist(err) {
		t.Fatalf("obsolete credential remains: %v", err)
	}
	redacted := RedactError(fmt.Errorf("runtime echoed sentinel-passphrase"), map[string]string{"wifi": "sentinel-passphrase"})
	if strings.Contains(redacted.Error(), "sentinel-passphrase") || !strings.Contains(redacted.Error(), "[REDACTED]") {
		t.Fatalf("redacted error = %q", redacted)
	}
	if err := RemoveDeploymentCredentials(stateRoot, "edge"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(DeploymentCredentialsDir(stateRoot, "edge")); !os.IsNotExist(err) {
		t.Fatalf("deployment credentials remain: %v", err)
	}
}

func TestMeshCredentialCleanupKeepsOnlyDeclaredMeshes(t *testing.T) {
	stateRoot := t.TempDir()
	for _, meshID := range []string{"retained", "obsolete"} {
		if _, err := MaterializeMeshCredential(stateRoot, "edge", meshID, []byte("private-mesh-key")); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveObsoleteMeshCredentials(stateRoot, "edge", []string{"retained"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(MeshCredentialPath(stateRoot, "edge", "retained")); err != nil {
		t.Fatalf("retained mesh credential missing: %v", err)
	}
	if _, err := os.Stat(MeshCredentialPath(stateRoot, "edge", "obsolete")); !os.IsNotExist(err) {
		t.Fatalf("obsolete mesh credential remains: %v", err)
	}
	if err := RemoveMeshCredential(stateRoot, "edge", "retained"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(MeshCredentialPath(stateRoot, "edge", "retained")); !os.IsNotExist(err) {
		t.Fatalf("mesh credential remains after rollback: %v", err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %o, want %o", path, got, want)
	}
}
