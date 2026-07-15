package docker

import (
	"reflect"
	"testing"
)

func TestBuildImageArgs(t *testing.T) {
	// No platforms: plain docker build
	args, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng"})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	if !reflect.DeepEqual(args, []string{"build", "--tag", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected no-platform args: %#v", args)
	}

	// No platforms with file
	withFile, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", File: "services/bng/Containerfile"})
	if err != nil {
		t.Fatalf("build args with file: %v", err)
	}
	if !reflect.DeepEqual(withFile, []string{"build", "--tag", "ghcr.io/gdcs-dev/bng:dev", "-f", "services/bng/Containerfile", "services/bng"}) {
		t.Fatalf("unexpected with-file args: %#v", withFile)
	}

	// Single platform: docker build --platform
	single, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", Platforms: []string{"linux/arm64"}})
	if err != nil {
		t.Fatalf("build args single platform: %v", err)
	}
	if !reflect.DeepEqual(single, []string{"build", "--platform", "linux/arm64", "--tag", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected single-platform args: %#v", single)
	}

	// Multi-platform: buildx --push with single tag
	multi, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", Platforms: []string{"linux/amd64", "linux/arm64"}})
	if err != nil {
		t.Fatalf("build args multi platform: %v", err)
	}
	if !reflect.DeepEqual(multi, []string{"buildx", "build", "--platform", "linux/amd64,linux/arm64", "--builder", "multiarch", "--push", "--tag", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected multi-platform args: %#v", multi)
	}

	// Multi-platform with two tags (release path: versioned + latest)
	multiTags, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:v0.1.0", "ghcr.io/gdcs-dev/bng:latest"}, Context: "services/bng", Platforms: []string{"linux/amd64", "linux/arm64"}})
	if err != nil {
		t.Fatalf("build args multi-tag: %v", err)
	}
	if !reflect.DeepEqual(multiTags, []string{"buildx", "build", "--platform", "linux/amd64,linux/arm64", "--builder", "multiarch", "--push", "--tag", "ghcr.io/gdcs-dev/bng:v0.1.0", "--tag", "ghcr.io/gdcs-dev/bng:latest", "services/bng"}) {
		t.Fatalf("unexpected multi-tag args: %#v", multiTags)
	}

	// No-cache
	noCache, err := buildImageArgs(ImageBuildRequest{Tags: []string{"ghcr.io/gdcs-dev/bng:dev"}, Context: "services/bng", NoCache: true})
	if err != nil {
		t.Fatalf("build args no-cache: %v", err)
	}
	if !reflect.DeepEqual(noCache, []string{"build", "--tag", "ghcr.io/gdcs-dev/bng:dev", "--no-cache", "services/bng"}) {
		t.Fatalf("unexpected no-cache args: %#v", noCache)
	}
}

func TestBuildImageArgsValidation(t *testing.T) {
	if _, err := buildImageArgs(ImageBuildRequest{Tags: []string{"x"}}); err == nil {
		t.Fatalf("expected context validation failure")
	}
	if _, err := buildImageArgs(ImageBuildRequest{Context: "services/bng"}); err == nil {
		t.Fatalf("expected tag validation failure")
	}
	if _, err := pullImageArgs(ImagePullRequest{}); err == nil {
		t.Fatalf("expected pull validation failure")
	}
	if _, err := pushImageArgs(ImagePushRequest{}); err == nil {
		t.Fatalf("expected push validation failure")
	}
	if _, err := tagImageArgs(ImageTagRequest{Source: "x"}); err == nil {
		t.Fatalf("expected tag validation failure")
	}
}
