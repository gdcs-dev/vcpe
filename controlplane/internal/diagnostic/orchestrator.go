package diagnostic

import (
	"context"
	"fmt"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

// Diagnose resolves one expected path, verifies source capability, collects
// active observations over loopback HTTP, and returns one validated safe graph.
func Diagnose(ctx context.Context, store *persist.Store, registry *Registry, client *Client, request ResolveRequest) (Result, error) {
	selection, err := Resolve(store, registry, request)
	if err != nil {
		return Result{}, err
	}
	expected, err := selection.Provider.Expected(ExpectedInput{Deployment: selection.Deployment, Source: selection.Source, Instance: selection.Instance, Target: selection.Target})
	if err != nil {
		return Result{}, err
	}
	target := Target{Host: "127.0.0.1", Port: selection.Endpoint.HostPort}
	capabilities, err := client.Discover(ctx, target)
	if err != nil {
		return Result{}, err
	}
	if !supportsJourney(capabilities, expected.Journey) {
		return Result{}, fmt.Errorf("source service %q does not advertise diagnostic journey %q", selection.Source.Name, expected.Journey)
	}
	endpointResponse, err := client.Run(ctx, target, expected.Journey, Invocation{ClientService: request.ClientService})
	if err != nil {
		return Result{}, err
	}
	if endpointResponse.Journey != expected.Journey {
		return Result{}, fmt.Errorf("diagnostic endpoint returned journey %q: expected %q", endpointResponse.Journey, expected.Journey)
	}
	observations, firstFailure, err := ApplyCausality(expected.Edges, endpointResponse.Observations, endpointResponse.ObservedAt)
	if err != nil {
		return Result{}, fmt.Errorf("evaluate diagnostic response: %w", err)
	}
	observedAt := endpointResponse.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	result := Result{
		SchemaVersion: SchemaVersion,
		Journey:       expected.Journey,
		Source:        expected.Source,
		Target:        expected.Target,
		Metadata:      expected.Metadata,
		Nodes:         expected.Nodes,
		Edges:         expected.Edges,
		Observations:  observations,
		FirstFailure:  firstFailure,
		ObservedAt:    observedAt,
	}
	return Sanitize(result)
}

func supportsJourney(capabilities Capabilities, journey string) bool {
	for _, candidate := range capabilities.Journeys {
		if candidate == journey {
			return true
		}
	}
	return false
}
