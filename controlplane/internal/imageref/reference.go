// Package imageref formats manifest image specifications without owning image
// lifecycle or rendering policy.
package imageref

import (
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

// Format returns repository:tag for a manifest image. An absent tag defaults
// to latest, while an absent repository produces no reference.
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
