package app

import (
	"fmt"
	"os/exec"
	"strings"
)

// gitReleasePreflight validates git state before any file or registry mutations:
// ensures HEAD is on the main branch and that the version tag does not exist.
func gitReleasePreflight(version string) error {
	branchOut, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("release: determine current branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch != "main" {
		return fmt.Errorf("release must be run from the main branch (current branch: %s)", branch)
	}

	tagOut, err := exec.Command("git", "tag", "-l", version).Output()
	if err != nil {
		return fmt.Errorf("release: check existing tags: %w", err)
	}
	if strings.TrimSpace(string(tagOut)) != "" {
		return fmt.Errorf("release: tag %s already exists; bump --version or delete the existing tag first", version)
	}
	return nil
}

// runGitRelease stages, commits, tags, and pushes the release.
// Call gitReleasePreflight before any file mutations, then call this.
func runGitRelease(manifestPath, version string) error {
	// Stage manifest.
	if out, err := exec.Command("git", "add", manifestPath).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git add %s: %w\n%s", manifestPath, err, strings.TrimSpace(string(out)))
	}

	// 4. Commit.
	msg := fmt.Sprintf("release: pin images to %s", version)
	if out, err := exec.Command("git", "commit", "-m", msg).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git commit: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	// 5. Lightweight tag.
	if out, err := exec.Command("git", "tag", version).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git tag %s: %w\n%s", version, err, strings.TrimSpace(string(out)))
	}

	// 6. Push commit.
	if out, err := exec.Command("git", "push", "origin", "HEAD").CombinedOutput(); err != nil {
		return fmt.Errorf("release: git push origin HEAD: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	// 7. Push tag.
	if out, err := exec.Command("git", "push", "origin", version).CombinedOutput(); err != nil {
		return fmt.Errorf("release: git push origin %s: %w\n%s", version, err, strings.TrimSpace(string(out)))
	}

	return nil
}
