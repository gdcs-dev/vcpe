package app

import (
	"fmt"
	"os/exec"
	"strings"
)

// runGitRelease executes the git portion of the release sequence:
//  1. Verify HEAD is on the main branch.
//  2. Verify the version tag does not already exist.
//  3. Stage the manifest file.
//  4. Commit the stamp.
//  5. Create a lightweight tag.
//  6. Push the commit to origin.
//  7. Push the tag to origin.
//
// Each step must succeed before the next begins.
func runGitRelease(manifestPath, version string) error {
	// 1. Must be on main.
	branchOut, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("release: determine current branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch != "main" {
		return fmt.Errorf("release must be run from the main branch (current branch: %s)", branch)
	}

	// 2. Tag must not already exist.
	tagOut, err := exec.Command("git", "tag", "-l", version).Output()
	if err != nil {
		return fmt.Errorf("release: check existing tags: %w", err)
	}
	if strings.TrimSpace(string(tagOut)) != "" {
		return fmt.Errorf("release: tag %s already exists; bump --version or delete the existing tag first", version)
	}

	// 3. Stage manifest.
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
