package servicetemplate

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"gopkg.in/yaml.v3"
)

func TestRenderValidatesHooksAndMode(t *testing.T) {
	decode := func(yaml.Node) (string, error) { return "config", nil }
	hook := func(context.Context, render.Input, string) (render.Result, error) {
		return render.Result{Artifacts: []render.Artifact{{Key: "compose.yaml", Content: "services: {}\n"}}}, nil
	}
	input := testInput(0)

	tests := []struct {
		name  string
		hooks Hooks[string]
		input render.Input
		want  string
	}{
		{name: "missing name", hooks: Hooks[string]{DecodeConfig: decode, RenderInstance: hook}, input: input, want: "name is required"},
		{name: "missing decoder", hooks: Hooks[string]{Name: "test", RenderInstance: hook}, input: input, want: "DecodeConfig hook is required"},
		{name: "missing hook", hooks: Hooks[string]{Name: "test", DecodeConfig: decode}, input: input, want: "RenderInstance hook is required"},
		{name: "unsupported mode", hooks: Hooks[string]{Name: "test", Mode: Mode(42), DecodeConfig: decode, RenderInstance: hook}, input: input, want: "unsupported mode 42"},
		{name: "no instances", hooks: Hooks[string]{Name: "test", DecodeConfig: decode, RenderInstance: hook}, input: testInput(), want: "requires at least one resolved instance"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := New(test.hooks).Render(context.Background(), test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want containing %q", err, test.want)
			}
			if !reflect.DeepEqual(result, render.Result{}) {
				t.Fatalf("Render() result = %#v, want zero result", result)
			}
		})
	}
}

func TestPerInstanceDecodesOnceAndInvokesEachInstance(t *testing.T) {
	input := testInput(2, 0, 4)
	decodeCalls := 0
	var received []int
	sharedHealthPorts := input.HealthPorts
	renderer := New(Hooks[string]{
		Name: "typed-test",
		Mode: PerInstance,
		DecodeConfig: func(yaml.Node) (string, error) {
			decodeCalls++
			return "decoded", nil
		},
		RenderInstance: func(_ context.Context, got render.Input, config string) (render.Result, error) {
			if config != "decoded" {
				t.Fatalf("config = %q, want decoded", config)
			}
			if len(got.Service.Instances) != 1 {
				t.Fatalf("hook instances = %#v, want exactly one", got.Service.Instances)
			}
			if reflect.ValueOf(got.HealthPorts).Pointer() != reflect.ValueOf(sharedHealthPorts).Pointer() {
				t.Fatal("hook did not receive shallow-copied input maps")
			}
			index := got.Service.Instances[0].Index
			received = append(received, index)
			return render.Result{Renderer: "ignored", Artifacts: []render.Artifact{
				{Key: "compose.yaml", Content: fmt.Sprintf("services:\n  service-%d:\n    image: test\n", index)},
			}}, nil
		},
	})

	result, err := renderer.Render(context.Background(), input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if decodeCalls != 1 {
		t.Fatalf("decode calls = %d, want 1", decodeCalls)
	}
	if !reflect.DeepEqual(received, []int{2, 0, 4}) {
		t.Fatalf("hook instance order = %v, want [2 0 4]", received)
	}
	if len(input.Service.Instances) != 3 {
		t.Fatalf("Render() mutated input instances: %#v", input.Service.Instances)
	}
	if result.Renderer != "typed-test" || renderer.Name() != "typed-test" {
		t.Fatalf("renderer identities = result %q, Name() %q", result.Renderer, renderer.Name())
	}
}

func TestInterpolatedPassThroughPreservesIdentityAndArtifactOrder(t *testing.T) {
	input := testInput(0, 1)
	input.Service.Replicas = 3
	wantArtifacts := []render.Artifact{
		{Key: "z.txt", Content: "last"},
		{Key: "a.txt", Content: "first"},
		{Key: "a.txt", Content: "first"},
	}
	decodeCalls := 0
	hookCalls := 0
	renderer := New(Hooks[int]{
		Name: "interpolated-test",
		Mode: Interpolated,
		DecodeConfig: func(yaml.Node) (int, error) {
			decodeCalls++
			return 7, nil
		},
		RenderInstance: func(_ context.Context, got render.Input, config int) (render.Result, error) {
			hookCalls++
			if config != 7 {
				t.Fatalf("hook config = %d, want 7", config)
			}
			if got.Service.Replicas != input.Service.Replicas {
				t.Fatalf("hook replicas = %d, want %d", got.Service.Replicas, input.Service.Replicas)
			}
			if !reflect.DeepEqual(got.Service.Instances, input.Service.Instances) {
				t.Fatalf("hook instances = %#v, want %#v", got.Service.Instances, input.Service.Instances)
			}
			return render.Result{Renderer: "hook-name", Artifacts: wantArtifacts}, nil
		},
	})

	result, err := renderer.Render(context.Background(), input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if decodeCalls != 1 || hookCalls != 1 {
		t.Fatalf("decode calls = %d, hook calls = %d, want 1 each", decodeCalls, hookCalls)
	}
	if result.Renderer != "interpolated-test" {
		t.Fatalf("result renderer = %q, want interpolated-test", result.Renderer)
	}
	if !reflect.DeepEqual(result.Artifacts, wantArtifacts) {
		t.Fatalf("artifacts = %#v, want %#v", result.Artifacts, wantArtifacts)
	}
}

func TestRenderWrapsDecodeAndHookErrors(t *testing.T) {
	t.Run("decode", func(t *testing.T) {
		renderer := New(Hooks[string]{
			Name:         "decode-test",
			DecodeConfig: func(yaml.Node) (string, error) { return "", errors.New("bad config") },
			RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
				t.Fatal("hook called after decode error")
				return render.Result{}, nil
			},
		})
		_, err := renderer.Render(context.Background(), testInput(0))
		assertErrorContains(t, err, `renderer "decode-test" service "service"`, "decode config: bad config")
	})

	t.Run("per instance hook", func(t *testing.T) {
		renderer := New(Hooks[string]{
			Name:         "hook-test",
			DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
			RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
				return render.Result{}, errors.New("hook failed")
			},
		})
		_, err := renderer.Render(context.Background(), testInput(4))
		assertErrorContains(t, err, `renderer "hook-test" service "service" instance 5`, "render: hook failed")
	})

	t.Run("interpolated hook", func(t *testing.T) {
		renderer := New(Hooks[string]{
			Name:         "hook-test",
			Mode:         Interpolated,
			DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
			RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
				return render.Result{}, errors.New("hook failed")
			},
		})
		_, err := renderer.Render(context.Background(), testInput())
		assertErrorContains(t, err, `renderer "hook-test" service "service"`, "render: hook failed")
	})
}

func TestValidateArtifactKey(t *testing.T) {
	for _, key := range []string{"compose.env", "nested/config/file.conf", ".hidden"} {
		t.Run("valid "+key, func(t *testing.T) {
			if err := validateArtifactKey(key); err != nil {
				t.Fatalf("validateArtifactKey(%q) error = %v", key, err)
			}
		})
	}

	for _, key := range []string{"", "/absolute", "../escape", "dir/../escape", "dir//file", "./file", "dir/", `dir\..\escape`, `C:\absolute`, `\\server\share`} {
		t.Run("invalid "+key, func(t *testing.T) {
			if err := validateArtifactKey(key); err == nil {
				t.Fatalf("validateArtifactKey(%q) succeeded, want error", key)
			}
		})
	}
}

func TestPerInstanceArtifactPlacement(t *testing.T) {
	renderer := New(Hooks[string]{
		Name:         "placement-test",
		DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
		RenderInstance: func(_ context.Context, input render.Input, _ string) (render.Result, error) {
			index := input.Service.Instances[0].Index
			return render.Result{Artifacts: []render.Artifact{
				{Key: "compose.yaml", Content: fmt.Sprintf("services:\n  service-%d: {}\n", index)},
				{Key: "compose.env", Content: fmt.Sprintf("INDEX=%d\n", index)},
				{Key: "config/app.conf", Content: fmt.Sprintf("instance %d", index)},
			}}, nil
		},
	})

	result, err := renderer.Render(context.Background(), testInput(0, 2))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(result.Artifacts) != 7 {
		t.Fatalf("artifact count = %d, want 7: %#v", len(result.Artifacts), result.Artifacts)
	}
	want := []render.Artifact{
		{Key: "compose.env", Content: "INDEX=0\n"},
		{Key: "instances/1/compose.env", Content: "INDEX=0\n"},
		{Key: "config/app.conf", Content: "instance 0"},
		{Key: "instances/1/config/app.conf", Content: "instance 0"},
		{Key: "instances/3/compose.env", Content: "INDEX=2\n"},
		{Key: "instances/3/config/app.conf", Content: "instance 2"},
	}
	if !reflect.DeepEqual(result.Artifacts[:6], want) {
		t.Fatalf("placed artifacts = %#v, want %#v", result.Artifacts[:6], want)
	}
	if result.Artifacts[6].Key != "compose.yaml" {
		t.Fatalf("final artifact = %#v, want compose.yaml", result.Artifacts[6])
	}
}

func TestArtifactDuplicateHandling(t *testing.T) {
	t.Run("identical per-instance output is retained once", func(t *testing.T) {
		renderer := New(Hooks[string]{
			Name:         "duplicate-test",
			DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
			RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
				return render.Result{Artifacts: []render.Artifact{
					{Key: "same.txt", Content: "same"},
					{Key: "same.txt", Content: "same"},
					{Key: "compose.yaml", Content: "services: {}\n"},
				}}, nil
			},
		})
		result, err := renderer.Render(context.Background(), testInput(0))
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if len(result.Artifacts) != 3 {
			t.Fatalf("artifacts = %#v, want root, instance, and compose", result.Artifacts)
		}
	})

	for _, mode := range []Mode{PerInstance, Interpolated} {
		t.Run(fmt.Sprintf("conflicting mode %d", mode), func(t *testing.T) {
			renderer := New(Hooks[string]{
				Name:         "duplicate-test",
				Mode:         mode,
				DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
				RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
					artifacts := []render.Artifact{{Key: "same.txt", Content: "one"}, {Key: "same.txt", Content: "two"}}
					if mode == PerInstance {
						artifacts = append(artifacts, render.Artifact{Key: "compose.yaml", Content: "services: {}\n"})
					}
					return render.Result{Artifacts: artifacts}, nil
				},
			})
			_, err := renderer.Render(context.Background(), testInput(0))
			assertErrorContains(t, err, "conflicting duplicate")
		})
	}
}

func TestComposeAggregation(t *testing.T) {
	fragments := map[int]string{
		0: "services:\n  alpha:\n    image: alpha:1\nnetworks:\n  shared:\n    external: true\n",
		1: "services:\n  beta:\n    image: beta:1\nnetworks:\n  shared: {external: true}\n  private: {}\n",
	}
	renderer := composeRenderer(func(index int) []render.Artifact {
		return []render.Artifact{{Key: "compose.yaml", Content: fragments[index]}}
	})
	result, err := renderer.Render(context.Background(), testInput(0, 1))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var compose composeDocument
	if err := yaml.Unmarshal([]byte(result.Artifacts[0].Content), &compose); err != nil {
		t.Fatalf("parse aggregated compose: %v", err)
	}
	if len(compose.Services) != 2 || compose.Services["alpha"] == nil || compose.Services["beta"] == nil {
		t.Fatalf("services = %#v, want alpha and beta", compose.Services)
	}
	if len(compose.Networks) != 2 || compose.Networks["shared"] == nil || compose.Networks["private"] == nil {
		t.Fatalf("networks = %#v, want shared and private", compose.Networks)
	}
}

func TestComposeFailures(t *testing.T) {
	tests := []struct {
		name      string
		artifacts func(int) []render.Artifact
		want      string
	}{
		{name: "missing", artifacts: func(int) []render.Artifact { return []render.Artifact{{Key: "compose.env"}} }, want: "missing compose.yaml"},
		{name: "multiple", artifacts: func(int) []render.Artifact {
			return []render.Artifact{{Key: "compose.yaml", Content: "services: {}"}, {Key: "compose.yaml", Content: "services: {}"}}
		}, want: "multiple compose.yaml"},
		{name: "malformed", artifacts: func(int) []render.Artifact {
			return []render.Artifact{{Key: "compose.yaml", Content: "services: ["}}
		}, want: "parse compose.yaml"},
		{name: "malformed services map", artifacts: func(int) []render.Artifact {
			return []render.Artifact{{Key: "compose.yaml", Content: "services: invalid"}}
		}, want: "parse compose.yaml"},
		{name: "conflicting service", artifacts: func(index int) []render.Artifact {
			return []render.Artifact{{Key: "compose.yaml", Content: fmt.Sprintf("services:\n  shared:\n    image: image:%d\n", index)}}
		}, want: `conflicting duplicate compose service key "shared"`},
		{name: "conflicting network", artifacts: func(index int) []render.Artifact {
			return []render.Artifact{{Key: "compose.yaml", Content: fmt.Sprintf("services:\n  service-%d: {}\nnetworks:\n  shared:\n    external: %t\n", index, index == 0)}}
		}, want: `conflicting duplicate compose network key "shared"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instances := []int{0}
			if strings.HasPrefix(test.name, "conflicting") {
				instances = []int{0, 1}
			}
			_, err := composeRenderer(test.artifacts).Render(context.Background(), testInput(instances...))
			assertErrorContains(t, err, `renderer "compose-test" service "service" instance`, test.want)
		})
	}
}

func TestInterpolatedArtifactValidationDoesNotRequireCompose(t *testing.T) {
	for _, key := range []string{"", "/absolute", "../escape"} {
		t.Run(key, func(t *testing.T) {
			renderer := New(Hooks[string]{
				Name:         "interpolated-test",
				Mode:         Interpolated,
				DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
				RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
					return render.Result{Artifacts: []render.Artifact{{Key: key, Content: "content"}}}, nil
				},
			})
			_, err := renderer.Render(context.Background(), testInput())
			if err == nil {
				t.Fatalf("Render() with key %q succeeded, want error", key)
			}
		})
	}
}

func composeRenderer(artifacts func(int) []render.Artifact) render.Renderer {
	return New(Hooks[string]{
		Name:         "compose-test",
		DecodeConfig: func(yaml.Node) (string, error) { return "", nil },
		RenderInstance: func(_ context.Context, input render.Input, _ string) (render.Result, error) {
			return render.Result{Artifacts: artifacts(input.Service.Instances[0].Index)}, nil
		},
	})
}

func testInput(indices ...int) render.Input {
	instances := make([]plan.Instance, len(indices))
	for position, index := range indices {
		instances[position] = plan.Instance{Index: index}
	}
	return render.Input{
		Service:     plan.Service{Name: "service", Instances: instances},
		HealthPorts: map[int]int{0: 9878},
		Secrets:     map[string]string{"secret": "value"},
	}
}

func assertErrorContains(t *testing.T, err error, wants ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want containing %q", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want containing %q", err, want)
		}
	}
}
