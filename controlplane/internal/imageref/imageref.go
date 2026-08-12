// Package imageref formats manifest image references for control-plane consumers.
package imageref

import (
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

// Format returns the canonical repository:tag reference. An absent repository
// produces no reference; an absent tag defaults to latest.
func Format(image manifest.Image) string {
	if strings.TrimSpace(image.Repository) == "" {
		return ""
	}
	tag := image.Tag
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	return image.Repository + ":" + tag
}
