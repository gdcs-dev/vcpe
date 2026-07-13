package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Adapter provides image operations against the Docker CLI.
// It only covers image lifecycle (build, pull, push, tag, exists).
// Networking and compose operations remain Podman-owned.
type Adapter struct{}

type ImageBuildRequest struct {
	Tag       string
	Context   string
	File      string
	NoCache   bool
	Platforms []string
}

type ImagePullRequest struct {
	Reference string
}

type ImagePushRequest struct {
	Reference string
}

type ImageTagRequest struct {
	Source string
	Target string
}

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

func (a *Adapter) BuildImage(ctx context.Context, req ImageBuildRequest) error {
	// Auto-detect Containerfile when no explicit file is given.
	// Docker defaults to "Dockerfile" but the vcpe services use "Containerfile".
	if req.File == "" && req.Context != "" {
		candidate := filepath.Join(req.Context, "Containerfile")
		if _, err := os.Stat(candidate); err == nil {
			req.File = candidate
		}
	}
	args, err := buildImageArgs(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build docker image %s: %w", req.Tag, err)
	}
	return nil
}

func (a *Adapter) PullImage(ctx context.Context, req ImagePullRequest) error {
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

func (a *Adapter) PushImage(ctx context.Context, req ImagePushRequest) error {
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

func (a *Adapter) TagImage(ctx context.Context, req ImageTagRequest) error {
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

// buildImageArgs constructs the docker build argument list.
// Multi-platform (len(Platforms) > 1): uses buildx with --push to produce a
// manifest list in the registry. Uses whatever buildx builder is currently
// active (set with `docker buildx use <builder>`).
// Single platform or no platform: uses plain docker build for a local image.
func buildImageArgs(req ImageBuildRequest) ([]string, error) {
	if req.Tag == "" {
		return nil, fmt.Errorf("build image tag is required")
	}
	if req.Context == "" {
		return nil, fmt.Errorf("build context is required")
	}
	var args []string
	if len(req.Platforms) > 1 {
		// Multi-platform: buildx + --push. Requires an active builder that
		// supports multi-platform (e.g. docker buildx use multiarch).
		args = []string{"buildx", "build",
			"--platform", strings.Join(req.Platforms, ","),
			"--builder", "multiarch",
			"--tag", req.Tag,
			"--push",
		}
	} else {
		// Single-arch or no explicit platform: plain docker build into local store.
		args = []string{"build", "--tag", req.Tag}
		if len(req.Platforms) == 1 {
			args = append(args, "--platform", req.Platforms[0])
		}
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

func pullImageArgs(req ImagePullRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("pull reference is required")
	}
	return []string{"pull", req.Reference}, nil
}

func pushImageArgs(req ImagePushRequest) ([]string, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("push reference is required")
	}
	return []string{"push", req.Reference}, nil
}

func tagImageArgs(req ImageTagRequest) ([]string, error) {
	if req.Source == "" {
		return nil, fmt.Errorf("tag source is required")
	}
	if req.Target == "" {
		return nil, fmt.Errorf("tag target is required")
	}
	return []string{"tag", req.Source, req.Target}, nil
}
