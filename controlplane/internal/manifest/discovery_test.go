package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeManifest writes a minimal vcpe.dev/v1 manifest file to dir/<name>.yaml.
func writeManifest(t *testing.T, dir, name, description string) string {
	t.Helper()
	content := fmt.Sprintf(`apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: %s
`, name)
	if description != "" {
		content += fmt.Sprintf("  annotations:\n    description: %q\n", description)
	}
	content += "spec:\n  networks: []\n  services: []\n"
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeInvalidManifest writes a YAML file that is NOT a valid vcpe manifest.
func writeInvalidManifest(t *testing.T, dir, name string) {
	t.Helper()
	content := "apiVersion: other/v1\nkind: Something\nmetadata:\n  name: foo\n"
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------- FindAll ----------

func TestFindAll_Empty(t *testing.T) {
	dir := t.TempDir()
	entries, err := FindAll([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestFindAll_OneValid(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "my-deploy", "A test deployment")
	entries, err := FindAll([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "my-deploy" {
		t.Errorf("name: got %q, want %q", entries[0].Name, "my-deploy")
	}
	if entries[0].Description != "A test deployment" {
		t.Errorf("description: got %q", entries[0].Description)
	}
}

func TestFindAll_Multiple(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "alpha", "")
	writeManifest(t, dir, "beta", "")
	entries, err := FindAll([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestFindAll_SkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "valid", "")
	writeInvalidManifest(t, dir, "invalid")
	entries, err := FindAll([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "valid" {
		t.Errorf("expected only 'valid', got %+v", entries)
	}
}

func TestFindAll_SkipsNonExistentDir(t *testing.T) {
	entries, err := FindAll([]string{"/nonexistent/path/that/does/not/exist"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestFindAll_MultipleDirectoriesOrderPreserved(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeManifest(t, dir1, "first", "")
	writeManifest(t, dir2, "second", "")
	entries, err := FindAll([]string{dir1, dir2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
	if entries[0].Name != "first" || entries[1].Name != "second" {
		t.Errorf("order wrong: %v", entries)
	}
}

// ---------- Resolve ----------

func TestResolve_Found(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "my-net", "")
	path, err := Resolve("my-net", []string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestResolve_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Resolve("missing", []string{dir})
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestResolve_FirstDirWins(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeManifest(t, dir1, "duplicate", "from-dir1")
	writeManifest(t, dir2, "duplicate", "from-dir2")
	path, err := Resolve("duplicate", []string{dir1, dir2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to dir1's version
	if filepath.Dir(path) != dir1 {
		t.Errorf("expected dir1, got dir: %s", filepath.Dir(path))
	}
}

// ---------- SearchDirs ----------

func TestSearchDirs_WithVCPEManifestDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VCPE_MANIFEST_DIRS", dir)
	dirs := SearchDirs(func() (string, error) {
		return "/fake/bin/vcpe", nil
	})
	if len(dirs) == 0 || dirs[0] != dir {
		t.Errorf("expected VCPE_MANIFEST_DIRS dir first, got %v", dirs)
	}
}

func TestSearchDirs_WithoutVCPEManifestDirs(t *testing.T) {
	t.Setenv("VCPE_MANIFEST_DIRS", "")
	dirs := SearchDirs(func() (string, error) {
		return "/fake/bin/vcpe", nil
	})
	if len(dirs) == 0 {
		t.Error("expected at least one search dir")
	}
}

func TestSearchDirs_ExecutableFnError(t *testing.T) {
	t.Setenv("VCPE_MANIFEST_DIRS", "")
	dirs := SearchDirs(func() (string, error) {
		return "", fmt.Errorf("exec failed")
	})
	// Should still return ~/.vcpe/manifests and ./manifests even when executable fails
	if len(dirs) < 2 {
		t.Errorf("expected at least 2 dirs even on exec error, got %v", dirs)
	}
}

func TestSearchDirs_TildeExpansion(t *testing.T) {
	t.Setenv("VCPE_MANIFEST_DIRS", "~/my-manifests:/other")
	dirs := SearchDirs(nil)
	if len(dirs) == 0 {
		t.Fatal("no dirs returned")
	}
	// First dir should not start with ~ after expansion
	if dirs[0] == "~/my-manifests" {
		t.Error("tilde was not expanded")
	}
}

func TestSearchDirs_EmptySegmentsSkipped(t *testing.T) {
	t.Setenv("VCPE_MANIFEST_DIRS", ":::")
	dirs := SearchDirs(func() (string, error) {
		return "/fake/bin/vcpe", nil
	})
	// None of the dirs should be empty
	for _, d := range dirs {
		if d == "" {
			t.Error("empty dir in search list")
		}
	}
}

func TestSearchDirs_PkgsharePath(t *testing.T) {
	t.Setenv("VCPE_MANIFEST_DIRS", "")
	dirs := SearchDirs(func() (string, error) {
		return "/opt/homebrew/bin/vcpe", nil
	})
	expected := "/opt/homebrew/share/vcpe/manifests"
	found := false
	for _, d := range dirs {
		if filepath.Clean(d) == filepath.Clean(expected) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pkgshare path %q not in dirs: %v", expected, dirs)
	}
}
