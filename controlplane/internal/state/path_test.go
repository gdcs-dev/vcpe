package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStateRootMakesRelativeOverrideAbsolute(t *testing.T) {
	workingDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	resolved, err := ResolveStateRoot(filepath.Join("relative", "state"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workingDir, "relative", "state")
	if !filepath.IsAbs(resolved) {
		t.Fatalf("ResolveStateRoot() = %q, want an absolute path", resolved)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(resolvedInfo, wantInfo) {
		t.Fatalf("ResolveStateRoot() = %q, want directory %q", resolved, want)
	}
}
