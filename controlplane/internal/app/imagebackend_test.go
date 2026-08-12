package app

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/docker"
	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
)

func TestNewImageBackend(t *testing.T) {
	t.Setenv("VCPE_SKIP_IMAGE", "")

	if _, ok := newImageBackend("docker").(*docker.Adapter); !ok {
		t.Fatalf("docker backend = %T, want *docker.Adapter", newImageBackend("docker"))
	}
	if _, ok := newImageBackend("").(*podman.Adapter); !ok {
		t.Fatalf("default backend = %T, want *podman.Adapter", newImageBackend(""))
	}

	t.Setenv("VCPE_SKIP_IMAGE", "1")
	if _, ok := newImageBackend("docker").(noopImageBackend); !ok {
		t.Fatalf("skip-image backend = %T, want noopImageBackend", newImageBackend("docker"))
	}
}
