package app

import (
	"fmt"
	"os/exec"
	"strings"
)

// DetectGitVersion returns the nearest git tag reachable from HEAD using
// git describe --tags --abbrev=0. Returns the tag as-is (e.g. "v0.1.0").
// Returns an error if no tag exists or git is unavailable.
func DetectGitVersion() (string, error) {
	out, err := exec.Command("git", "describe", "--tags", "--abbrev=0").Output()
	if err != nil {
		return "", fmt.Errorf("no git tag found; create a tag before releasing (git tag vX.Y.Z): %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("git describe returned an empty tag")
	}
	return v, nil
}
