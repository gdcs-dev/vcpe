package app

import (
	"context"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
)

// podmanImageBackend adapts the podman backend's request types to the
// image.Backend interface the image.Manager consumes. It is the single seam
// between the image lifecycle policy and the concrete container runtime.
type podmanImageBackend struct {
	adapter *podman.Adapter
}

func newImageBackend() image.Backend {
	if os.Getenv("VCPE_SKIP_IMAGE") == "1" {
		return noopImageBackend{}
	}
	return podmanImageBackend{adapter: podman.New()}
}

// noopImageBackend is a test-only image backend that satisfies image.Backend
// without contacting a container runtime. ImageExists always returns true so
// the default build-if-missing policy produces action:"noop". All mutating
// methods succeed immediately without side effects.
// Activated by setting VCPE_SKIP_IMAGE=1.
type noopImageBackend struct{}

func (noopImageBackend) ImageExists(_ context.Context, _ string) (bool, error)    { return true, nil }
func (noopImageBackend) BuildImage(_ context.Context, _ image.BuildRequest) error { return nil }
func (noopImageBackend) PullImage(_ context.Context, _ image.PullRequest) error   { return nil }
func (noopImageBackend) PushImage(_ context.Context, _ image.PushRequest) error   { return nil }
func (noopImageBackend) TagImage(_ context.Context, _ image.TagRequest) error     { return nil }

func (b podmanImageBackend) ImageExists(ctx context.Context, reference string) (bool, error) {
	return b.adapter.ImageExists(ctx, reference)
}

func (b podmanImageBackend) BuildImage(ctx context.Context, req image.BuildRequest) error {
	return b.adapter.BuildImage(ctx, podman.ImageBuildRequest{
		Tag:       req.Tag,
		Context:   req.Context,
		File:      req.File,
		NoCache:   req.NoCache,
		Platforms: req.Platforms,
	})
}

func (b podmanImageBackend) PullImage(ctx context.Context, req image.PullRequest) error {
	return b.adapter.PullImage(ctx, podman.ImagePullRequest{Reference: req.Reference})
}

func (b podmanImageBackend) PushImage(ctx context.Context, req image.PushRequest) error {
	return b.adapter.PushImage(ctx, podman.ImagePushRequest{Reference: req.Reference})
}

func (b podmanImageBackend) TagImage(ctx context.Context, req image.TagRequest) error {
	return b.adapter.TagImage(ctx, podman.ImageTagRequest{Source: req.Source, Target: req.Target})
}
