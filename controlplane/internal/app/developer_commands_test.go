//go:build !homebrew

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTempRepo creates a temporary git repository with an initial commit on
// the main branch and returns the repo root path.
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup: %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")

	// Initial commit so the repo has a HEAD.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("git", "add", "README.md")
	run("git", "commit", "-m", "init")

	return dir
}

// runPreflightIn is a helper that sets the working directory for gitReleasePreflight
// by temporarily changing the process working directory.
func runPreflightIn(dir, version string) error {
	orig, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	defer os.Chdir(orig) //nolint:errcheck
	return gitReleasePreflight(version)
}

func TestRunGitRelease_NonMainBranch(t *testing.T) {
	dir := initTempRepo(t)

	// Create and switch to a non-main branch.
	cmd := exec.Command("git", "checkout", "-b", "feature/test")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v\n%s", err, out)
	}

	err := runPreflightIn(dir, "v0.1.0")
	if err == nil {
		t.Fatal("expected error for non-main branch, got nil")
	}
	if !strings.Contains(err.Error(), "main branch") {
		t.Errorf("expected 'main branch' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "feature/test") {
		t.Errorf("expected branch name 'feature/test' in error, got: %v", err)
	}
}

func TestRunGitRelease_TagAlreadyExists(t *testing.T) {
	dir := initTempRepo(t)

	// Pre-create the tag.
	cmd := exec.Command("git", "tag", "v0.1.0")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pre-tag: %v\n%s", err, out)
	}

	err := runPreflightIn(dir, "v0.1.0")
	if err == nil {
		t.Fatal("expected error for existing tag, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}
