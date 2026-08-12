package servicetemplate_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"gopkg.in/yaml.v3"
)

func TestConstructionValidation(t *testing.T) {
	tests := []struct {
		name  string
		hooks servicetemplate.Hooks[string]
		want  string
	}{
		{name: "missing name", hooks: servicetemplate.Hooks[string]{DecodeConfig: decodeString, RenderInstance: renderCompose}, want: "name is required"},
		{name: "missing decoder", hooks: servicetemplate.Hooks[string]{Name: "test", RenderInstance: renderCompose}, want: "config decoder is required"},
		{name: "missing hook", hooks: servicetemplate.Hooks[string]{Name: "test", DecodeConfig: decodeString}, want: "render hook is required"},
		{name: "unsupported mode", hooks: servicetemplate.Hooks[string]{Name: "test", Mode: servicetemplate.Mode(99), DecodeConfig: decodeString, RenderInstance: renderCompose}, want: "unsupported mode"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := servicetemplate.New(testCase.hooks).Render(context.Background(), oneInstanceInput())
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Render() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestPerInstanceLifecycle(t *testing.T) {
	t.Run("no instances", func(t *testing.T) {
		calls := 0
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name:         "test",
			DecodeConfig: decodeString,
			RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
				calls++
				return render.Result{}, nil
			},
		})
		_, err := renderer.Render(context.Background(), render.Input{Service: plan.Service{Name: "webpa"}})
		if err == nil || !strings.Contains(err.Error(), "test") || !strings.Contains(err.Error(), "webpa") || !strings.Contains(err.Error(), "no resolved instances") {
			t.Fatalf("Render() error = %v, want renderer, service, and no-instance context", err)
		}
		if calls != 0 {
			t.Fatalf("hook calls = %d, want 0", calls)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		decodeErr := errors.New("invalid config")
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name:           "test",
			DecodeConfig:   func(yaml.Node) (string, error) { return "", decodeErr },
			RenderInstance: renderCompose,
		})
		_, err := renderer.Render(context.Background(), oneInstanceInput())
		if !errors.Is(err, decodeErr) {
			t.Fatalf("Render() error = %v, want wrapped %v", err, decodeErr)
		}
	})

	t.Run("hook error", func(t *testing.T) {
		hookErr := errors.New("render failed")
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name:         "test",
			DecodeConfig: decodeString,
			RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
				return render.Result{}, hookErr
			},
		})
		_, err := renderer.Render(context.Background(), oneInstanceInput())
		if !errors.Is(err, hookErr) || !strings.Contains(err.Error(), "instance 1") {
			t.Fatalf("Render() error = %v, want wrapped hook error with instance context", err)
		}
	})

	t.Run("decodes once and invokes once per instance", func(t *testing.T) {
		input := render.Input{Service: plan.Service{Name: "webpa", Instances: []plan.Instance{{Index: 3}, {Index: 1}}}}
		decodeCalls := 0
		var received [][]plan.Instance
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name: "test",
			DecodeConfig: func(yaml.Node) (string, error) {
				decodeCalls++
				return "decoded", nil
			},
			RenderInstance: func(_ context.Context, input render.Input, config string) (render.Result, error) {
				if config != "decoded" {
					t.Fatalf("hook config = %q, want decoded", config)
				}
				received = append(received, input.Service.Instances)
				return composeResult("svc" + string(rune('0'+input.Service.Instances[0].Index))), nil
			},
		})
		result, err := renderer.Render(context.Background(), input)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if decodeCalls != 1 {
			t.Fatalf("decoder calls = %d, want 1", decodeCalls)
		}
		wantInstances := [][]plan.Instance{{{Index: 3}}, {{Index: 1}}}
		if !reflect.DeepEqual(received, wantInstances) {
			t.Fatalf("hook instances = %#v, want %#v", received, wantInstances)
		}
		if result.Renderer != "test" {
			t.Fatalf("renderer = %q, want test", result.Renderer)
		}
		if len(result.Artifacts) != 1 || result.Artifacts[0].Key != "compose.yaml" {
			t.Fatalf("artifacts = %#v, want aggregated compose.yaml", result.Artifacts)
		}
		assertComposeSections(t, result.Artifacts[0].Content, map[string]any{"svc3": map[string]any{"image": "example"}, "svc1": map[string]any{"image": "example"}}, map[string]any{"shared": map[string]any{"external": true}})
	})
}

func TestInterpolatedLifecycle(t *testing.T) {
	input := render.Input{Service: plan.Service{Name: "generic", Instances: []plan.Instance{{Index: 0}, {Index: 1}}}}
	decodeCalls := 0
	hookCalls := 0
	artifacts := []render.Artifact{{Key: "compose.yaml", Content: "services:\n  app:\n    image: example:${REPLICA}"}, {Key: "entrypoint.sh", Content: "#!/bin/sh\n"}}
	renderer := servicetemplate.New(servicetemplate.Hooks[string]{
		Name: "generic-container",
		Mode: servicetemplate.Interpolated,
		DecodeConfig: func(yaml.Node) (string, error) {
			decodeCalls++
			return "decoded", nil
		},
		RenderInstance: func(_ context.Context, got render.Input, config string) (render.Result, error) {
			hookCalls++
			if !reflect.DeepEqual(got, input) {
				t.Fatalf("hook input = %#v, want complete input %#v", got, input)
			}
			if config != "decoded" {
				t.Fatalf("hook config = %q, want decoded", config)
			}
			return render.Result{Renderer: "wrong-name", Artifacts: artifacts}, nil
		},
	})

	result, err := renderer.Render(context.Background(), input)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if decodeCalls != 1 || hookCalls != 1 {
		t.Fatalf("calls = decoder %d hook %d, want one each", decodeCalls, hookCalls)
	}
	if result.Renderer != "generic-container" {
		t.Fatalf("renderer = %q, want normalized hook name", result.Renderer)
	}
	if !reflect.DeepEqual(result.Artifacts, artifacts) {
		t.Fatalf("artifacts = %#v, want pass-through %#v", result.Artifacts, artifacts)
	}
}

func TestInterpolatedArtifactValidation(t *testing.T) {
	renderer := servicetemplate.New(servicetemplate.Hooks[string]{
		Name:         "generic-container",
		Mode:         servicetemplate.Interpolated,
		DecodeConfig: decodeString,
		RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
			return render.Result{Artifacts: []render.Artifact{{Key: "../entrypoint.sh", Content: "#!/bin/sh\n"}}}, nil
		},
	})
	_, err := renderer.Render(context.Background(), render.Input{Service: plan.Service{Name: "generic"}})
	if err == nil || !strings.Contains(err.Error(), "parent traversal") {
		t.Fatalf("Render() error = %v, want invalid artifact error", err)
	}
}

func TestArtifactKeyValidation(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty", key: "", want: "required"},
		{name: "absolute", key: "/etc/passwd", want: "relative"},
		{name: "parent", key: "../secret", want: "parent traversal"},
		{name: "nested parent", key: "configs/../secret", want: "parent traversal"},
		{name: "valid nested", key: "configs/app.conf"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			renderer := servicetemplate.New(servicetemplate.Hooks[string]{
				Name:         "test",
				DecodeConfig: decodeString,
				RenderInstance: func(context.Context, render.Input, string) (render.Result, error) {
					return render.Result{Artifacts: []render.Artifact{{Key: "compose.yaml", Content: "services: {}"}, {Key: testCase.key, Content: "value"}}}, nil
				},
			})
			result, err := renderer.Render(context.Background(), oneInstanceInput())
			if testCase.want == "" {
				if err != nil {
					t.Fatalf("Render() error = %v", err)
				}
				if !hasArtifact(result.Artifacts, "configs/app.conf") || !hasArtifact(result.Artifacts, "instances/1/configs/app.conf") {
					t.Fatalf("artifacts = %#v, want root and instance placement", result.Artifacts)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Render() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestArtifactPlacementAndConflicts(t *testing.T) {
	t.Run("first instance mirror and per-instance paths", func(t *testing.T) {
		input := render.Input{Service: plan.Service{Instances: []plan.Instance{{Index: 2}, {Index: 0}}}}
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name:         "test",
			DecodeConfig: decodeString,
			RenderInstance: func(_ context.Context, input render.Input, _ string) (render.Result, error) {
				index := input.Service.Instances[0].Index
				return render.Result{Artifacts: []render.Artifact{
					{Key: "compose.env", Content: "INSTANCE=" + string(rune('0'+index)) + "\n"},
					{Key: "configs/app.conf", Content: "INSTANCE=" + string(rune('0'+index)) + "\n"},
					{Key: "compose.yaml", Content: "services: {}"},
				}}, nil
			},
		})
		result, err := renderer.Render(context.Background(), input)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		want := []render.Artifact{
			{Key: "compose.env", Content: "INSTANCE=2\n"},
			{Key: "instances/3/compose.env", Content: "INSTANCE=2\n"},
			{Key: "configs/app.conf", Content: "INSTANCE=2\n"},
			{Key: "instances/3/configs/app.conf", Content: "INSTANCE=2\n"},
			{Key: "instances/1/compose.env", Content: "INSTANCE=0\n"},
			{Key: "instances/1/configs/app.conf", Content: "INSTANCE=0\n"},
			{Key: "compose.yaml", Content: "{}\n"},
		}
		if !reflect.DeepEqual(result.Artifacts, want) {
			t.Fatalf("artifacts = %#v, want %#v", result.Artifacts, want)
		}
	})

	t.Run("conflicting output", func(t *testing.T) {
		input := render.Input{Service: plan.Service{Instances: []plan.Instance{{Index: 0}, {Index: 0}}}}
		calls := 0
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name:         "test",
			DecodeConfig: decodeString,
			RenderInstance: func(_ context.Context, _ render.Input, _ string) (render.Result, error) {
				calls++
				return render.Result{Artifacts: []render.Artifact{
					{Key: "config", Content: string(rune('a' + calls))},
					{Key: "compose.yaml", Content: "services: {}"},
				}}, nil
			},
		})
		_, err := renderer.Render(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "conflicting render artifact") {
			t.Fatalf("Render() error = %v, want conflicting output error", err)
		}
	})
}

func TestComposeAggregation(t *testing.T) {
	t.Run("distinct service and equivalent network", func(t *testing.T) {
		input := render.Input{Service: plan.Service{Instances: []plan.Instance{{Index: 0}, {Index: 1}}}}
		renderer := servicetemplate.New(servicetemplate.Hooks[string]{
			Name:         "test",
			DecodeConfig: decodeString,
			RenderInstance: func(_ context.Context, input render.Input, _ string) (render.Result, error) {
				if input.Service.Instances[0].Index == 0 {
					return render.Result{Artifacts: []render.Artifact{{Key: "compose.yaml", Content: "services:\n  first:\n    image: example\nnetworks:\n  shared:\n    external: true\n    name: shared-net"}}}, nil
				}
				return render.Result{Artifacts: []render.Artifact{{Key: "compose.yaml", Content: "networks:\n  shared:\n    name: shared-net\n    external: true\nservices:\n  second:\n    image: example"}}}, nil
			},
		})
		result, err := renderer.Render(context.Background(), input)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		assertComposeSections(t, result.Artifacts[0].Content, map[string]any{"first": map[string]any{"image": "example"}, "second": map[string]any{"image": "example"}}, map[string]any{"shared": map[string]any{"name": "shared-net", "external": true}})
	})

	for _, testCase := range []struct {
		name      string
		fragments []string
		want      string
	}{
		{name: "conflicting service", fragments: []string{"services:\n  app:\n    image: one", "services:\n  app:\n    image: two"}, want: "conflicting Compose service"},
		{name: "conflicting network", fragments: []string{"networks:\n  shared:\n    external: true", "networks:\n  shared:\n    external: false"}, want: "conflicting Compose network"},
		{name: "missing compose", fragments: []string{""}, want: "missing compose.yaml artifact"},
		{name: "multiple compose", fragments: []string{"services: {}", "services: {}"}, want: "multiple compose.yaml artifacts"},
		{name: "malformed yaml", fragments: []string{"services: ["}, want: "parse compose.yaml"},
		{name: "unstructured yaml", fragments: []string{"- service"}, want: "document must be a mapping"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := render.Input{Service: plan.Service{Instances: []plan.Instance{{Index: 0}, {Index: 1}}}}
			renderer := servicetemplate.New(servicetemplate.Hooks[string]{
				Name:         "test",
				DecodeConfig: decodeString,
				RenderInstance: func(_ context.Context, input render.Input, _ string) (render.Result, error) {
					if testCase.name == "missing compose" {
						return render.Result{Artifacts: []render.Artifact{{Key: "config", Content: "value"}}}, nil
					}
					artifacts := []render.Artifact{{Key: "compose.yaml", Content: testCase.fragments[0]}}
					if testCase.name == "multiple compose" {
						artifacts = append(artifacts, render.Artifact{Key: "compose.yaml", Content: testCase.fragments[1]})
					} else if input.Service.Instances[0].Index == 1 {
						artifacts[0].Content = testCase.fragments[1]
					}
					return render.Result{Artifacts: artifacts}, nil
				},
			})
			_, err := renderer.Render(context.Background(), input)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Render() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func decodeString(yaml.Node) (string, error) {
	return "decoded", nil
}

func renderCompose(context.Context, render.Input, string) (render.Result, error) {
	return composeResult("app"), nil
}

func composeResult(service string) render.Result {
	return render.Result{Artifacts: []render.Artifact{{Key: "compose.yaml", Content: "services:\n  " + service + ":\n    image: example\nnetworks:\n  shared:\n    external: true"}}}
}

func oneInstanceInput() render.Input {
	return render.Input{Service: plan.Service{Name: "webpa", Instances: []plan.Instance{{Index: 0}}}}
}

func hasArtifact(artifacts []render.Artifact, key string) bool {
	for _, artifact := range artifacts {
		if artifact.Key == key {
			return true
		}
	}
	return false
}

func assertComposeSections(t *testing.T, content string, services, networks map[string]any) {
	t.Helper()
	var document struct {
		Services map[string]any `yaml:"services"`
		Networks map[string]any `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("unmarshal aggregated Compose: %v", err)
	}
	if !reflect.DeepEqual(document.Services, services) {
		t.Errorf("Compose services = %#v, want %#v", document.Services, services)
	}
	if !reflect.DeepEqual(document.Networks, networks) {
		t.Errorf("Compose networks = %#v, want %#v", document.Networks, networks)
	}
}
