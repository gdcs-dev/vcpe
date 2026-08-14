package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
)

// Adapter implements image.Backend directly for Docker image operations.
// It only covers image lifecycle (build, pull, push, tag, exists).
// Networking and compose operations remain Podman-owned.
type Adapter struct{}

var _ image.Backend = (*Adapter)(nil)

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) ImageExists(ctx context.Context, reference string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", reference)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func (a *Adapter) BuildImage(ctx context.Context, req image.BuildRequest) error {
	if !req.ArtifactsPrepared {
		if err := image.PrepareBuild(ctx, req); err != nil {
			return err
		}
	}

	// Auto-detect Containerfile when no explicit file is given.
	// Docker defaults to "Dockerfile" but the vcpe services use "Containerfile".
	if req.File == "" && req.Context != "" {
		candidate := filepath.Join(req.Context, "Containerfile")
		if _, err := os.Stat(candidate); err == nil {
			req.File = candidate
		}
	}

	// Multiple platforms use one buildx invocation. Generated binaries were
	// prepared in platform-qualified context paths before the backend was called.
	if len(req.Platforms) > 1 {
		return a.buildMultiPlatform(ctx, req)
	}
	return a.runBuildImage(ctx, req)
}

func (a *Adapter) runBuildImage(ctx context.Context, req image.BuildRequest) error {
	args, err := buildImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		primary := ""
		if len(req.Tags) > 0 {
			primary = req.Tags[0]
		}
		return fmt.Errorf("build docker image %s: %w", primary, err)
	}
	return nil
}

// buildMultiPlatform builds and pushes the complete manifest list in one
// invocation. TARGETOS/TARGETARCH select platform-qualified generated files.
func (a *Adapter) buildMultiPlatform(ctx context.Context, req image.BuildRequest) error {
	args, err := multiPlatformBuildArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build multi-platform docker image %s: %w", req.Tags[0], err)
	}
	return nil
}

func multiPlatformBuildArgs(req image.BuildRequest) ([]string, error) {
	if len(req.Tags) == 0 {
		return nil, fmt.Errorf("build image tags are required")
	}
	if req.Context == "" {
		return nil, fmt.Errorf("build context is required")
	}
	args := []string{"buildx", "build", "--builder", "multiarch", "--push", "--platform", strings.Join(req.Platforms, ",")}
	for _, tag := range req.Tags {
		args = append(args, "--tag", tag)
	}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	if req.File != "" {
		args = append(args, "-f", req.File)
	}
	return append(args, req.Context), nil
}

func (a *Adapter) PullImage(ctx context.Context, req image.PullRequest) error {
	args, err := pullImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pull docker image %s: %w", req.Reference, err)
	}
	return nil
}

func (a *Adapter) PushImage(ctx context.Context, req image.PushRequest) error {
	args, err := pushImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("push docker image %s: %w", req.Reference, err)
	}
	return nil
}

func (a *Adapter) TagImage(ctx context.Context, req image.TagRequest) error {
	args, err := tagImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tag docker image %s -> %s: %w (%s)", req.Source, req.Target, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// buildImageArgs constructs a plain local Docker build, optionally for one
// explicit platform. Multi-platform requests use multiPlatformBuildArgs.
func buildImageArgs(req image.BuildRequest) ([]string, error) {
	if len(req.Tags) == 0 {
		return nil, fmt.Errorf("build image tags are required")
	}
	if req.Context == "" {
		return nil, fmt.Errorf("build context is required")
	}
	args := []string{"build"}
	if len(req.Platforms) == 1 {
		args = append(args, "--platform", req.Platforms[0])
	}
	for _, t := range req.Tags {
		args = append(args, "--tag", t)
	}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	if req.File != "" {
		args = append(args, "-f", req.File)
	}
	args = append(args, req.Context)
	return args, nil
}

func pullImageArgs(req image.PullRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("pull reference is required")
	}
	return []string{"pull", req.Reference}, nil
}

func pushImageArgs(req image.PushRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("push reference is required")
	}
	return []string{"push", req.Reference}, nil
}

func tagImageArgs(req image.TagRequest) ([]string, error) {
	if req.Source == "" {
		return nil, fmt.Errorf("tag source is required")
	}
	if req.Target == "" {
		return nil, fmt.Errorf("tag target is required")
	}
	return []string{"tag", req.Source, req.Target}, nil
}
