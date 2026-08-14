package diagnostic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

type clientRoundTripFunc func(*http.Request) (*http.Response, error)

func (function clientRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func diagnosticStore(t *testing.T) *persist.Store {
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	if _, err := store.ReserveHealthEndpoint("edge", "gateway", 0); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestDiagnoseCompletedOutcomes(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name   string
		states []State
		want   Outcome
	}{
		{name: "healthy downstream", states: []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed}, want: OutcomePassed},
		{name: "application unavailable", states: []State{StateUnknown, StatePassed, StatePassed, StatePassed, StatePassed}, want: OutcomeInconclusive},
		{name: "local Parodus failure", states: []State{StateFailed, StatePassed, StatePassed, StatePassed, StatePassed}, want: OutcomeFailed},
		{name: "DNS failure", states: []State{StateUnknown, StateFailed, StateSkipped, StateSkipped, StateSkipped}, want: OutcomeFailed},
		{name: "transport failure", states: []State{StateUnknown, StatePassed, StateFailed, StateSkipped, StateSkipped}, want: OutcomeFailed},
		{name: "authentication failure", states: []State{StateUnknown, StatePassed, StatePassed, StateFailed, StateSkipped}, want: OutcomeFailed},
		{name: "registration failure", states: []State{StateUnknown, StatePassed, StatePassed, StatePassed, StateFailed}, want: OutcomeFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observations := endpointObservations(now, test.states)
			client := fakeDiagnosticClient(t, EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: observations}, nil)
			result, err := Diagnose(context.Background(), diagnosticStore(t), resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa"})
			if err != nil {
				t.Fatalf("Diagnose: %v", err)
			}
			outcome, err := Classify(result)
			if err != nil || outcome != test.want {
				t.Fatalf("Classify = %q, %v; want %q", outcome, err, test.want)
			}
		})
	}
}

func TestDiagnoseTransportAndProtocolErrors(t *testing.T) {
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("endpoint unavailable")
	})}}
	if _, err := Diagnose(context.Background(), diagnosticStore(t), resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa"}); err == nil || !strings.Contains(err.Error(), "endpoint unavailable") {
		t.Fatalf("transport error = %v", err)
	}

	invalid := fakeDiagnosticClient(t, EndpointResponse{}, nil)
	if _, err := Diagnose(context.Background(), diagnosticStore(t), resolverRegistry(), invalid, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa"}); err == nil || !strings.Contains(err.Error(), "invalid diagnostic response") {
		t.Fatalf("protocol error = %v", err)
	}
}

func fakeDiagnosticClient(t *testing.T, endpoint EndpointResponse, endpointErr error) *Client {
	t.Helper()
	return &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/diagnostics" {
			return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA}}), nil
		}
		if endpointErr != nil {
			return nil, endpointErr
		}
		return jsonResponse(endpoint), nil
	})}}
}

func jsonResponse(value any) *http.Response {
	body, _ := json.Marshal(value)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}
}

func endpointObservations(observedAt time.Time, states []State) []Observation {
	ids := []string{"application-parodus", "talaria-dns", "talaria-transport", "talaria-authentication", "device-registration"}
	observations := make([]Observation, len(ids))
	for index := range ids {
		observations[index] = Observation{EdgeID: ids[index], State: states[index], ObservedAt: observedAt}
	}
	return observations
}
