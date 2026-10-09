// Package imageref formats manifest image references for control-plane consumers.
package imageref

import (
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

// Format returns the repository's pinned digest or canonical repository:tag.
// An absent repository produces no reference; an absent tag defaults to latest.
func Format(image manifest.Image) string {
	if strings.TrimSpace(image.Repository) == "" {
		return ""
	}
	if strings.Contains(image.Repository, "@") {
		return image.Repository
	}
	tag := image.Tag
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	return image.Repository + ":" + tag
}
