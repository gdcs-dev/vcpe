package hwsim

import (
	"context"
	"fmt"
)

type ClientFactory func(context.Context) (*Client, error)

func Preflight(ctx context.Context, radioCount int, factory ClientFactory) error {
	return PreflightFeatures(ctx, radioCount, false, factory)
}

func PreflightFeatures(ctx context.Context, radioCount int, requireMesh bool, factory ClientFactory) error {
	return PreflightRequirements(ctx, radioCount, requireMesh, false, factory)
}

func PreflightRequirements(ctx context.Context, radioCount int, requireMesh, requireRF bool, factory ClientFactory) error {
	if radioCount == 0 {
		if requireMesh || requireRF {
			return fmt.Errorf("wireless preflight: mesh and RF scenarios require radios")
		}
		return nil
	}
	if factory == nil {
		return fmt.Errorf("wireless preflight: hwsim client factory is not configured")
	}
	client, err := factory(ctx)
	if err != nil {
		return fmt.Errorf("wireless preflight: %w", err)
	}
	if _, err := client.Version(ctx); err != nil {
		return fmt.Errorf("wireless preflight version: %w", err)
	}
	liveMesh := false
	if requireMesh {
		radios, err := client.List(ctx)
		if err != nil {
			return fmt.Errorf("wireless preflight list: %w", err)
		}
		for _, radio := range radios {
			if radio.Live && radio.Record.RadioType == "mesh" && radio.Record.Lifecycle == "ready" && radio.ResolvedKernel != nil && radio.ResolvedNamespace != nil {
				liveMesh = true
				break
			}
		}
	}
	doctor, err := client.Doctor(ctx, requireMesh && !liveMesh)
	if err != nil {
		return fmt.Errorf("wireless preflight doctor: %w", err)
	}
	if !doctor.Healthy {
		return fmt.Errorf("wireless preflight doctor reported an unhealthy manager")
	}
	if requireMesh && !liveMesh && doctor.MeshCapability != "supported" {
		return fmt.Errorf("wireless preflight: mesh capability %q is not supported", doctor.MeshCapability)
	}
	if requireRF {
		medium, err := client.MediumHealth(ctx)
		if err != nil {
			return fmt.Errorf("wireless preflight medium-health: %w", err)
		}
		if !medium.Healthy {
			return fmt.Errorf("wireless preflight: medium-health is unhealthy")
		}
		checks := map[string]bool{}
		for _, check := range medium.Checks {
			checks[check.Name] = check.OK
		}
		for _, required := range []string{"baseline", "service", "socket", "registration"} {
			if !checks[required] {
				return fmt.Errorf("wireless preflight: medium-health %s check failed or is missing", required)
			}
		}
	}
	return nil
}
