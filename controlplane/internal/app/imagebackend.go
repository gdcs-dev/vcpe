package app

import (
	"context"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/docker"
	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
)

// newImageBackend returns the image.Backend for the given backend name.
// backend must be "podman" (default) or "docker".
func newImageBackend(backend string) image.Backend {
	if os.Getenv("VCPE_SKIP_IMAGE") == "1" {
		return noopImageBackend{}
	}
	if backend == "docker" {
		return docker.New()
	}
	return podman.New()
}

// noopImageBackend is a test-only image backend that satisfies image.Backend
// without contacting a container runtime. ImageExists always returns true so
// the default build-if-missing policy produces action:"noop". All mutating
// methods succeed immediately without side effects.
// Activated by setting VCPE_SKIP_IMAGE=1.
type noopImageBackend struct{}

var _ image.Backend = noopImageBackend{}

func (noopImageBackend) ImageExists(_ context.Context, _ string) (bool, error)    { return true, nil }
func (noopImageBackend) BuildImage(_ context.Context, _ image.BuildRequest) error { return nil }
func (noopImageBackend) PullImage(_ context.Context, _ image.PullRequest) error   { return nil }
func (noopImageBackend) PushImage(_ context.Context, _ image.PushRequest) error   { return nil }
func (noopImageBackend) TagImage(_ context.Context, _ image.TagRequest) error     { return nil }
