// Package render defines the rendering contract shared by all service types.
// It is a leaf package: it depends only on the resolved plan model and never on
// the service-type registry, so type packages may implement Renderer without an
// import cycle. The dispatch that maps a service type to its renderer lives in
// the orchestrator.
package render

import (
	"context"
	"fmt"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// Input is everything a renderer needs to produce artifacts for one service.
type Input struct {
	Deployment plan.Deployment
	Service    plan.Service
	// Secrets maps secret ref name to its resolved value. File-provider secrets
	// carry the on-host path; env-provider secrets carry the host env var name.
	Secrets map[string]string
}

// Artifact is a single rendered file keyed by a relative path.
type Artifact struct {
	Key     string
	Content string
}

// Result is the set of artifacts a renderer produced.
type Result struct {
	Renderer  string
	Artifacts []Artifact
}

// Contract declares artifacts a renderer must always emit.
type Contract struct {
	RequiredArtifacts []string
}

// Renderer turns a resolved service into deployable artifacts.
type Renderer interface {
	Name() string
	Render(ctx context.Context, input Input) (Result, error)
}

// Validate enforces artifact well-formedness and contract satisfaction.
func Validate(result Result, contract Contract) error {
	if result.Renderer == "" {
		return fmt.Errorf("render result missing renderer name")
	}
	seen := map[string]bool{}
	for _, artifact := range result.Artifacts {
		if artifact.Key == "" {
			return fmt.Errorf("render artifact key is required")
		}
		if seen[artifact.Key] {
			return fmt.Errorf("duplicate render artifact key %s", artifact.Key)
		}
		seen[artifact.Key] = true
	}
	for _, required := range contract.RequiredArtifacts {
		if !seen[required] {
			return fmt.Errorf("missing required render artifact %s", required)
		}
	}
	return nil
}

// RenderWithContract renders and validates against the contract in one step.
func RenderWithContract(ctx context.Context, renderer Renderer, input Input, contract Contract) (Result, error) {
	result, err := renderer.Render(ctx, input)
	if err != nil {
		return Result{}, err
	}
	if err := Validate(result, contract); err != nil {
		return Result{}, err
	}
	return result, nil
}
