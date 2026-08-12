// Package servicetemplate implements shared renderer lifecycle mechanics.
// Service hooks retain ownership of their Compose fields, network attachment,
// health, and generated-artifact policy. Per-instance renderers use the
// standard root and instances/<n> artifact placement; interpolated renderers
// validate and return their service-owned artifact layout.
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

// Mode selects how a service renderer handles its resolved instances.
type Mode uint8

const (
	// PerInstance invokes the render hook once for each resolved instance and
	// aggregates its Compose fragments and artifacts.
	PerInstance Mode = iota
	// Interpolated invokes the render hook once with the complete input.
	Interpolated
)

// Hooks supplies service-owned behavior to the common renderer lifecycle.
type Hooks[C any] struct {
	Name           string
	Mode           Mode
	DecodeConfig   func(yaml.Node) (C, error)
	RenderInstance func(context.Context, render.Input, C) (render.Result, error)
}

// New creates a renderer which applies hooks using the requested instance mode.
// Required hook validation happens at render time because render.Renderer has no
// construction error return.
func New[C any](hooks Hooks[C]) render.Renderer {
	return renderer[C]{hooks: hooks}
}

type renderer[C any] struct {
	hooks Hooks[C]
}

func (renderer renderer[C]) Render(ctx context.Context, input render.Input) (render.Result, error) {
	if err := renderer.validateHooks(); err != nil {
		return render.Result{}, err
	}
	if renderer.hooks.Mode == PerInstance && len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("renderer %q service %q has no resolved instances", renderer.hooks.Name, input.Service.Name)
	}

	config, err := renderer.hooks.DecodeConfig(input.Service.Config)
	if err != nil {
		return render.Result{}, fmt.Errorf("renderer %q decode config for service %q: %w", renderer.hooks.Name, input.Service.Name, err)
	}

	if renderer.hooks.Mode == Interpolated {
		result, err := renderer.hooks.RenderInstance(ctx, input, config)
		if err != nil {
			return render.Result{}, fmt.Errorf("renderer %q service %q: %w", renderer.hooks.Name, input.Service.Name, err)
		}
		if err := validateArtifacts(result.Artifacts); err != nil {
			return render.Result{}, fmt.Errorf("renderer %q service %q: %w", renderer.hooks.Name, input.Service.Name, err)
		}
		result.Renderer = renderer.hooks.Name
		return result, nil
	}

	return renderer.renderPerInstance(ctx, input, config)
}

func (renderer renderer[C]) Name() string {
	return renderer.hooks.Name
}

func (renderer renderer[C]) validateHooks() error {
	if renderer.hooks.Name == "" {
		return fmt.Errorf("service renderer name is required")
	}
	if renderer.hooks.DecodeConfig == nil {
		return fmt.Errorf("renderer %q config decoder is required", renderer.hooks.Name)
	}
	if renderer.hooks.RenderInstance == nil {
		return fmt.Errorf("renderer %q render hook is required", renderer.hooks.Name)
	}
	if renderer.hooks.Mode != PerInstance && renderer.hooks.Mode != Interpolated {
		return fmt.Errorf("renderer %q has unsupported mode %d", renderer.hooks.Name, renderer.hooks.Mode)
	}
	return nil
}

func (renderer renderer[C]) renderPerInstance(ctx context.Context, input render.Input, config C) (render.Result, error) {
	aggregate := composeDocument{}
	outputs := newArtifactSet()

	for position, instance := range input.Service.Instances {
		instanceInput := input
		instanceInput.Service.Instances = []plan.Instance{instance}
		result, err := renderer.hooks.RenderInstance(ctx, instanceInput, config)
		if err != nil {
			return render.Result{}, renderer.instanceError(input, instance, err)
		}
		if err := renderer.addInstanceResult(&aggregate, outputs, result.Artifacts, instance, position == 0); err != nil {
			return render.Result{}, renderer.instanceError(input, instance, err)
		}
	}

	compose, err := yaml.Marshal(aggregate)
	if err != nil {
		return render.Result{}, fmt.Errorf("renderer %q marshal Compose document: %w", renderer.hooks.Name, err)
	}
	outputs.addUnchecked(render.Artifact{Key: "compose.yaml", Content: string(compose)})
	return render.Result{Renderer: renderer.hooks.Name, Artifacts: outputs.artifacts}, nil
}

func (renderer renderer[C]) addInstanceResult(aggregate *composeDocument, outputs *artifactSet, artifacts []render.Artifact, instance plan.Instance, first bool) error {
	composeCount := 0
	for _, artifact := range artifacts {
		if err := validateArtifactKey(artifact.Key); err != nil {
			return err
		}
		if artifact.Key == "compose.yaml" {
			composeCount++
			if composeCount > 1 {
				return fmt.Errorf("multiple compose.yaml artifacts")
			}
			fragment, err := parseCompose(artifact.Content)
			if err != nil {
				return err
			}
			if err := aggregate.merge(fragment); err != nil {
				return err
			}
			continue
		}

		if first {
			if err := outputs.add(render.Artifact{Key: artifact.Key, Content: artifact.Content}); err != nil {
				return err
			}
		}
		key := fmt.Sprintf("instances/%d/%s", instance.Index+1, artifact.Key)
		if err := outputs.add(render.Artifact{Key: key, Content: artifact.Content}); err != nil {
			return err
		}
	}
	if composeCount == 0 {
		return fmt.Errorf("missing compose.yaml artifact")
	}
	return nil
}

func (renderer renderer[C]) instanceError(input render.Input, instance plan.Instance, err error) error {
	return fmt.Errorf("renderer %q service %q instance %d: %w", renderer.hooks.Name, input.Service.Name, instance.Index+1, err)
}

type artifactSet struct {
	artifacts []render.Artifact
	content   map[string]string
}

func newArtifactSet() *artifactSet {
	return &artifactSet{content: make(map[string]string)}
}

func (set *artifactSet) add(artifact render.Artifact) error {
	if existing, ok := set.content[artifact.Key]; ok {
		if existing != artifact.Content {
			return fmt.Errorf("conflicting render artifact %q", artifact.Key)
		}
		return nil
	}
	set.addUnchecked(artifact)
	return nil
}

func (set *artifactSet) addUnchecked(artifact render.Artifact) {
	set.content[artifact.Key] = artifact.Content
	set.artifacts = append(set.artifacts, artifact)
}

func validateArtifacts(artifacts []render.Artifact) error {
	seen := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		if err := validateArtifactKey(artifact.Key); err != nil {
			return err
		}
		if existing, ok := seen[artifact.Key]; ok && existing != artifact.Content {
			return fmt.Errorf("conflicting render artifact %q", artifact.Key)
		}
		seen[artifact.Key] = artifact.Content
	}
	return nil
}

func validateArtifactKey(key string) error {
	if key == "" {
		return fmt.Errorf("render artifact key is required")
	}
	if path.IsAbs(key) {
		return fmt.Errorf("render artifact key %q must be relative", key)
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return fmt.Errorf("render artifact key %q must not contain parent traversal", key)
		}
	}
	return nil
}

type composeDocument struct {
	Services map[string]yaml.Node `yaml:"services,omitempty"`
	Networks map[string]yaml.Node `yaml:"networks,omitempty"`
}

func parseCompose(content string) (composeDocument, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return composeDocument{}, fmt.Errorf("parse compose.yaml: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return composeDocument{}, fmt.Errorf("parse compose.yaml: document must be a mapping")
	}
	for position := 0; position < len(root.Content[0].Content); position += 2 {
		key := root.Content[0].Content[position]
		value := root.Content[0].Content[position+1]
		if (key.Value == "services" || key.Value == "networks") && value.Kind != yaml.MappingNode {
			return composeDocument{}, fmt.Errorf("parse compose.yaml: %s must be a mapping", key.Value)
		}
	}

	var document composeDocument
	if err := root.Decode(&document); err != nil {
		return composeDocument{}, fmt.Errorf("parse compose.yaml: %w", err)
	}
	return document, nil
}

func (document *composeDocument) merge(fragment composeDocument) error {
	if err := mergeComposeSection(&document.Services, fragment.Services, "service"); err != nil {
		return err
	}
	if err := mergeComposeSection(&document.Networks, fragment.Networks, "network"); err != nil {
		return err
	}
	return nil
}

func mergeComposeSection(destination *map[string]yaml.Node, source map[string]yaml.Node, kind string) error {
	if len(source) == 0 {
		return nil
	}
	if *destination == nil {
		*destination = make(map[string]yaml.Node, len(source))
	}
	for key, value := range source {
		existing, ok := (*destination)[key]
		if ok {
			equal, err := yamlNodesEqual(existing, value)
			if err != nil {
				return fmt.Errorf("normalize Compose %s %q: %w", kind, key, err)
			}
			if !equal {
				return fmt.Errorf("conflicting Compose %s %q", kind, key)
			}
			continue
		}
		(*destination)[key] = value
	}
	return nil
}

func yamlNodesEqual(left, right yaml.Node) (bool, error) {
	var leftValue any
	if err := left.Decode(&leftValue); err != nil {
		return false, err
	}
	var rightValue any
	if err := right.Decode(&rightValue); err != nil {
		return false, err
	}
	return reflect.DeepEqual(leftValue, rightValue), nil
}
