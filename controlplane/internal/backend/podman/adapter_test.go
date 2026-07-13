package podman

import (
	"reflect"
	"testing"
)

func TestImageCommandArgs(t *testing.T) {
	args, err := buildImageArgs(ImageBuildRequest{Tag: "ghcr.io/gdcs-dev/bng:dev", Context: "services/bng", File: "services/bng/Containerfile"})
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	if !reflect.DeepEqual(args, []string{"build", "-t", "ghcr.io/gdcs-dev/bng:dev", "-f", "services/bng/Containerfile", "services/bng"}) {
		t.Fatalf("unexpected build args: %#v", args)
	}

	noCacheArgs, err := buildImageArgs(ImageBuildRequest{Tag: "ghcr.io/gdcs-dev/bng:dev", Context: "services/bng", NoCache: true})
	if err != nil {
		t.Fatalf("build args no-cache: %v", err)
	}
	if !reflect.DeepEqual(noCacheArgs, []string{"build", "-t", "ghcr.io/gdcs-dev/bng:dev", "--no-cache", "services/bng"}) {
		t.Fatalf("unexpected no-cache build args: %#v", noCacheArgs)
	}

	// Single platform: --manifest mode
	singlePlatform, err := buildImageArgs(ImageBuildRequest{Tag: "ghcr.io/gdcs-dev/bng:dev", Context: "services/bng", Platforms: []string{"linux/amd64"}})
	if err != nil {
		t.Fatalf("build args single platform: %v", err)
	}
	if !reflect.DeepEqual(singlePlatform, []string{"build", "--platform", "linux/amd64", "--manifest", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected single-platform build args: %#v", singlePlatform)
	}

	// Multi-platform: --manifest mode with comma-joined platforms
	multiPlatform, err := buildImageArgs(ImageBuildRequest{Tag: "ghcr.io/gdcs-dev/bng:dev", Context: "services/bng", Platforms: []string{"linux/amd64", "linux/arm64"}})
	if err != nil {
		t.Fatalf("build args multi platform: %v", err)
	}
	if !reflect.DeepEqual(multiPlatform, []string{"build", "--platform", "linux/amd64,linux/arm64", "--manifest", "ghcr.io/gdcs-dev/bng:dev", "services/bng"}) {
		t.Fatalf("unexpected multi-platform build args: %#v", multiPlatform)
	}

	pull, err := pullImageArgs(ImagePullRequest{Reference: "ghcr.io/gdcs-dev/bng:dev"})
	if err != nil {
		t.Fatalf("pull args: %v", err)
	}
	if !reflect.DeepEqual(pull, []string{"pull", "ghcr.io/gdcs-dev/bng:dev"}) {
		t.Fatalf("unexpected pull args: %#v", pull)
	}

	push, err := pushImageArgs(ImagePushRequest{Reference: "ghcr.io/gdcs-dev/bng:dev"})
	if err != nil {
		t.Fatalf("push args: %v", err)
	}
	if !reflect.DeepEqual(push, []string{"push", "ghcr.io/gdcs-dev/bng:dev"}) {
		t.Fatalf("unexpected push args: %#v", push)
	}

	tag, err := tagImageArgs(ImageTagRequest{Source: "ghcr.io/gdcs-dev/bng:dev", Target: "localhost/bng:test"})
	if err != nil {
		t.Fatalf("tag args: %v", err)
	}
	if !reflect.DeepEqual(tag, []string{"tag", "ghcr.io/gdcs-dev/bng:dev", "localhost/bng:test"}) {
		t.Fatalf("unexpected tag args: %#v", tag)
	}
}

func TestImageCommandArgsValidation(t *testing.T) {
	if _, err := buildImageArgs(ImageBuildRequest{Tag: "x"}); err == nil {
		t.Fatalf("expected build context validation failure")
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
