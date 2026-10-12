package image

import (
	"context"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

type fakeBackend struct {
	existsByRef map[string]bool
	builds      []BuildRequest
	pulls       []PullRequest
}

func TestImageReferenceCurrentBehavior(t *testing.T) {
	tests := []struct {
		name  string
		image manifest.Image
		want  string
	}{
		{name: "empty repository", want: ""},
		{name: "omitted tag", image: manifest.Image{Repository: "example/workload"}, want: "example/workload:latest"},
		{name: "whitespace-only tag", image: manifest.Image{Repository: "example/workload", Tag: "  "}, want: "example/workload:latest"},
		{name: "explicit tag", image: manifest.Image{Repository: "example/workload", Tag: "v2"}, want: "example/workload:v2"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ImageReference(testCase.image); got != testCase.want {
				t.Errorf("ImageReference(%+v) = %q, want %q", testCase.image, got, testCase.want)
			}
		})
	}
}

func (f *fakeBackend) ImageExists(_ context.Context, reference string) (bool, error) {
	return f.existsByRef[reference], nil
}

func (f *fakeBackend) BuildImage(_ context.Context, req BuildRequest) error {
	f.builds = append(f.builds, req)
	return nil
}

func (f *fakeBackend) PullImage(_ context.Context, req PullRequest) error {
	f.pulls = append(f.pulls, req)
	return nil
}

func (f *fakeBackend) PushImage(_ context.Context, _ PushRequest) error { return nil }
func (f *fakeBackend) TagImage(_ context.Context, _ TagRequest) error   { return nil }

func TestEnsureForApplyBuildIfMissingPolicy(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": false}}
	mgr := New(backend)

	summary, err := mgr.EnsureForApply(context.Background(), bngManifest(PolicyBuildIfMissing))
	if err != nil {
		t.Fatalf("ensure for apply: %v", err)
	}
	if len(backend.builds) != 1 {
		t.Fatalf("expected one build, got %#v", backend.builds)
	}
	if len(summary.Actions) != 1 || summary.Actions[0].Action != "build" {
		t.Fatalf("unexpected summary actions: %#v", summary.Actions)
	}
}

func TestEnsureForApplyAlwaysPullPolicy(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": true}}
	mgr := New(backend)

	summary, err := mgr.EnsureForApply(context.Background(), bngManifest(PolicyAlwaysPull))
	if err != nil {
		t.Fatalf("ensure for apply: %v", err)
	}
	if len(backend.pulls) != 1 {
		t.Fatalf("expected one pull, got %#v", backend.pulls)
	}
	if len(summary.Actions) != 1 || summary.Actions[0].Action != "pull" {
		t.Fatalf("unexpected summary actions: %#v", summary.Actions)
	}
}

func TestEnsureForApplyNeverBuildPolicy(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": false}}
	mgr := New(backend)

	if _, err := mgr.EnsureForApply(context.Background(), bngManifest(PolicyNeverBuild)); err == nil {
		t.Fatalf("expected policy failure for missing image")
	}
}

func TestBuildBuildsSelectedImages(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": true}}
	mgr := New(backend)

	summary, err := mgr.Build(context.Background(), bngManifest(PolicyBuildIfMissing))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(backend.builds) != 1 {
		t.Fatalf("expected one build, got %#v", backend.builds)
	}
	if len(summary.Actions) != 1 || summary.Actions[0].Action != "build" {
		t.Fatalf("unexpected build summary: %#v", summary.Actions)
	}
}

func TestBuildAlwaysPullPolicy(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": true}}
	mgr := New(backend)

	summary, err := mgr.Build(context.Background(), bngManifest(PolicyAlwaysPull))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(backend.pulls) != 1 {
		t.Fatalf("expected one pull, got %#v", backend.pulls)
	}
	if len(backend.builds) != 0 {
		t.Fatalf("expected no builds for always-pull policy, got %#v", backend.builds)
	}
	if len(summary.Actions) != 1 || summary.Actions[0].Action != "pull" {
		t.Fatalf("unexpected build summary: %#v", summary.Actions)
	}
}

func TestBuildNeverBuildPolicyRequiresExistingImage(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": false}}
	mgr := New(backend)

	if _, err := mgr.Build(context.Background(), bngManifest(PolicyNeverBuild)); err == nil {
		t.Fatalf("expected policy failure for missing image")
	}
}

func TestBuildWithOptionsNoCacheForcesNoCacheBuildRequest(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": true}}
	mgr := New(backend)

	if _, err := mgr.BuildWithOptions(context.Background(), bngManifest(PolicyBuildIfMissing), BuildOptions{NoCache: true}); err != nil {
		t.Fatalf("build with options: %v", err)
	}
	if len(backend.builds) != 1 {
		t.Fatalf("expected one build, got %#v", backend.builds)
	}
	if !backend.builds[0].NoCache {
		t.Fatalf("expected build request no-cache=true, got %#v", backend.builds[0])
	}
}

func TestBuildWithOptionsPerServicePlatformsOverridesGlobal(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": true}}
	mgr := New(backend)

	doc := bngManifest(PolicyBuildIfMissing)
	doc.Spec.Services[0].Image.Platforms = []string{"linux/arm64"}

	if _, err := mgr.BuildWithOptions(context.Background(), doc, BuildOptions{
		Platforms:  []string{"linux/amd64", "linux/arm64"},
		ForceBuild: true,
	}); err != nil {
		t.Fatalf("build with options: %v", err)
	}
	if len(backend.builds) != 1 {
		t.Fatalf("expected one build, got %#v", backend.builds)
	}
	got := backend.builds[0].Platforms
	if len(got) != 1 || got[0] != "linux/arm64" {
		t.Fatalf("expected platforms [linux/arm64] from service override, got %v", got)
	}
}

func TestBuildWithOptionsNoServicePlatformsUsesGlobal(t *testing.T) {
	backend := &fakeBackend{existsByRef: map[string]bool{"ghcr.io/gdcs-dev/bng:dev": true}}
	mgr := New(backend)

	globalPlatforms := []string{"linux/amd64", "linux/arm64"}
	if _, err := mgr.BuildWithOptions(context.Background(), bngManifest(PolicyBuildIfMissing), BuildOptions{
		Platforms:  globalPlatforms,
		ForceBuild: true,
	}); err != nil {
		t.Fatalf("build with options: %v", err)
	}
	if len(backend.builds) != 1 {
		t.Fatalf("expected one build, got %#v", backend.builds)
	}
	got := backend.builds[0].Platforms
	if len(got) != 2 || got[0] != "linux/amd64" || got[1] != "linux/arm64" {
		t.Fatalf("expected global platforms %v, got %v", globalPlatforms, got)
	}
}

func bngManifest(policy string) manifest.Document {
	return manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Services: []manifest.Service{{
				Name: "bng",
				Type: "bng",
				Image: manifest.Image{
					Repository:   "ghcr.io/gdcs-dev/bng",
					Tag:          "dev",
					BuildContext: "services/bng",
					PullPolicy:   policy,
				},
			}},
		},
	}
}
