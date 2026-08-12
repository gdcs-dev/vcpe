// Package servicetemplate provides shared lifecycle mechanics for service renderers.
package servicetemplate

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"gopkg.in/yaml.v3"
)

// Mode selects how a service hook receives resolved instances.
type Mode uint8

const (
	// PerInstance invokes the hook once for each resolved instance and aggregates its output.
	PerInstance Mode = iota
	// Interpolated invokes the hook once with the complete render input.
	Interpolated
)

// Hooks defines the service-owned portions of a shared renderer lifecycle.
type Hooks[C any] struct {
	Name           string
	Mode           Mode
	DecodeConfig   func(yaml.Node) (C, error)
	RenderInstance func(context.Context, render.Input, C) (render.Result, error)
}

type renderer[C any] struct {
	hooks Hooks[C]
}

type composeDocument struct {
	Services map[string]any `yaml:"services"`
	Networks map[string]any `yaml:"networks"`
}

// New returns a renderer backed by the supplied typed hooks.
func New[C any](hooks Hooks[C]) render.Renderer {
	return renderer[C]{hooks: hooks}
}

func (r renderer[C]) Name() string {
	return r.hooks.Name
}

func (r renderer[C]) Render(ctx context.Context, input render.Input) (render.Result, error) {
	if err := r.validate(input); err != nil {
		return render.Result{}, err
	}

	config, err := r.hooks.DecodeConfig(input.Service.Config)
	if err != nil {
		return render.Result{}, r.serviceError(input, fmt.Errorf("decode config: %w", err))
	}

	switch r.hooks.Mode {
	case PerInstance:
		return r.renderPerInstance(ctx, input, config)
	case Interpolated:
		return r.renderInterpolated(ctx, input, config)
	default:
		panic("servicetemplate: validated mode became unsupported")
	}
}

func (r renderer[C]) validate(input render.Input) error {
	if r.hooks.Name == "" {
		return fmt.Errorf("renderer contract for service %q: name is required", input.Service.Name)
	}
	if r.hooks.Mode != PerInstance && r.hooks.Mode != Interpolated {
		return r.serviceError(input, fmt.Errorf("unsupported mode %d", r.hooks.Mode))
	}
	if r.hooks.DecodeConfig == nil {
		return r.serviceError(input, fmt.Errorf("DecodeConfig hook is required"))
	}
	if r.hooks.RenderInstance == nil {
		return r.serviceError(input, fmt.Errorf("RenderInstance hook is required"))
	}
	if r.hooks.Mode == PerInstance && len(input.Service.Instances) == 0 {
		return r.serviceError(input, fmt.Errorf("requires at least one resolved instance"))
	}
	return nil
}

func (r renderer[C]) renderInterpolated(ctx context.Context, input render.Input, config C) (render.Result, error) {
	result, err := r.hooks.RenderInstance(ctx, input, config)
	if err != nil {
		return render.Result{}, r.serviceError(input, fmt.Errorf("render: %w", err))
	}
	if err := validateArtifacts(result.Artifacts); err != nil {
		return render.Result{}, r.serviceError(input, err)
	}
	result.Renderer = r.hooks.Name
	return result, nil
}

func (r renderer[C]) renderPerInstance(ctx context.Context, input render.Input, config C) (render.Result, error) {
	artifacts := make([]render.Artifact, 0)
	outputs := make(map[string]string)
	compose := composeDocument{
		Services: make(map[string]any),
		Networks: make(map[string]any),
	}

	for _, instance := range input.Service.Instances {
		instanceInput := input
		instanceInput.Service = input.Service
		instanceInput.Service.Instances = []plan.Instance{instance}

		result, err := r.hooks.RenderInstance(ctx, instanceInput, config)
		if err != nil {
			return render.Result{}, r.instanceError(input, instance.Index, fmt.Errorf("render: %w", err))
		}
		if err := r.collectInstance(result.Artifacts, instance.Index, &artifacts, outputs, &compose); err != nil {
			return render.Result{}, r.instanceError(input, instance.Index, err)
		}
	}

	content, err := yaml.Marshal(compose)
	if err != nil {
		return render.Result{}, r.serviceError(input, fmt.Errorf("marshal compose.yaml: %w", err))
	}
	artifacts = append(artifacts, render.Artifact{Key: "compose.yaml", Content: string(content)})
	return render.Result{Renderer: r.hooks.Name, Artifacts: artifacts}, nil
}

func (r renderer[C]) collectInstance(source []render.Artifact, index int, artifacts *[]render.Artifact, outputs map[string]string, compose *composeDocument) error {
	composeCount := 0
	for _, artifact := range source {
		if err := validateArtifactKey(artifact.Key); err != nil {
			return err
		}
		if artifact.Key == "compose.yaml" {
			composeCount++
			if composeCount > 1 {
				return fmt.Errorf("multiple compose.yaml artifacts")
			}
			if err := mergeCompose(compose, artifact.Content); err != nil {
				return err
			}
			continue
		}

		if index == 0 {
			if err := appendOutput(artifacts, outputs, artifact); err != nil {
				return err
			}
		}
		placed := artifact
		placed.Key = path.Join("instances", fmt.Sprint(index+1), artifact.Key)
		if err := appendOutput(artifacts, outputs, placed); err != nil {
			return err
		}
	}
	if composeCount == 0 {
		return fmt.Errorf("missing compose.yaml artifact")
	}
	return nil
}

func validateArtifacts(artifacts []render.Artifact) error {
	seen := make(map[string]string)
	for _, artifact := range artifacts {
		if err := validateArtifactKey(artifact.Key); err != nil {
			return err
		}
		if content, ok := seen[artifact.Key]; ok && content != artifact.Content {
			return fmt.Errorf("conflicting duplicate artifact path %q", artifact.Key)
		}
		seen[artifact.Key] = artifact.Content
	}
	return nil
}

func validateArtifactKey(key string) error {
	if key == "" {
		return fmt.Errorf("artifact key is required")
	}
	if path.IsAbs(key) || strings.HasPrefix(key, `\`) || hasWindowsVolume(key) {
		return fmt.Errorf("artifact key %q must be relative", key)
	}
	if cleaned := path.Clean(key); cleaned != key {
		return fmt.Errorf("artifact key %q must be canonical", key)
	}
	for _, part := range strings.FieldsFunc(key, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return fmt.Errorf("artifact key %q contains parent traversal", key)
		}
	}
	return nil
}

func hasWindowsVolume(key string) bool {
	return len(key) >= 2 && ((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= 'a' && key[0] <= 'z')) && key[1] == ':'
}

func appendOutput(artifacts *[]render.Artifact, outputs map[string]string, artifact render.Artifact) error {
	if content, ok := outputs[artifact.Key]; ok {
		if content != artifact.Content {
			return fmt.Errorf("conflicting duplicate output path %q", artifact.Key)
		}
		return nil
	}
	outputs[artifact.Key] = artifact.Content
	*artifacts = append(*artifacts, artifact)
	return nil
}

func mergeCompose(destination *composeDocument, content string) error {
	var fragment composeDocument
	if err := yaml.Unmarshal([]byte(content), &fragment); err != nil {
		return fmt.Errorf("parse compose.yaml: %w", err)
	}
	if err := mergeMap("service", destination.Services, fragment.Services); err != nil {
		return err
	}
	if err := mergeMap("network", destination.Networks, fragment.Networks); err != nil {
		return err
	}
	return nil
}

func mergeMap(kind string, destination, source map[string]any) error {
	for key, value := range source {
		if existing, ok := destination[key]; ok {
			if !reflect.DeepEqual(existing, value) {
				return fmt.Errorf("conflicting duplicate compose %s key %q", kind, key)
			}
			continue
		}
		destination[key] = value
	}
	return nil
}

func (r renderer[C]) serviceError(input render.Input, err error) error {
	return fmt.Errorf("renderer %q service %q: %w", r.hooks.Name, input.Service.Name, err)
}

func (r renderer[C]) instanceError(input render.Input, index int, err error) error {
	return fmt.Errorf("renderer %q service %q instance %d: %w", r.hooks.Name, input.Service.Name, index+1, err)
}
