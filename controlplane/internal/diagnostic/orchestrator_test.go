package diagnostic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
			result, err := Diagnose(context.Background(), diagnosticStore(t), resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", ClientService: "config"})
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
	if _, err := Diagnose(context.Background(), diagnosticStore(t), resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", ClientService: "config"}); err == nil || !strings.Contains(err.Error(), "endpoint unavailable") {
		t.Fatalf("transport error = %v", err)
	}

	invalid := fakeDiagnosticClient(t, EndpointResponse{}, nil)
	if _, err := Diagnose(context.Background(), diagnosticStore(t), resolverRegistry(), invalid, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", ClientService: "config"}); err == nil || !strings.Contains(err.Error(), "invalid diagnostic response") {
		t.Fatalf("protocol error = %v", err)
	}
}

func TestDiagnoseArgusWebhooksUsesPersistedLoopbackEndpoint(t *testing.T) {
	store := resolverStore(t, "    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	endpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	registration := validWebhookRegistration(strings.Repeat("a", 64))
	registrations := []WebhookRegistration{registration}
	requests := []string{}
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Port() != fmt.Sprint(endpoint.HostPort) {
			t.Fatalf("diagnostic port = %q", request.URL.Port())
		}
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/diagnostics":
			return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyArgusWebhooks}}), nil
		case "/diagnostics/" + JourneyArgusWebhooks:
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation != (Invocation{}) {
				t.Fatalf("inventory invocation = %+v, %v", invocation, err)
			}
			return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyArgusWebhooks, ObservedAt: now, Observations: []Observation{
				{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
				{EdgeID: "argus-inventory", State: StatePassed, ObservedAt: now},
			}, WebhookRegistrations: &registrations}), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", request.URL.Path)
		}
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "webpa", Target: "webhooks"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if result.Journey != JourneyArgusWebhooks || result.Target.Type != "argus" || result.WebhookRegistrations == nil || len(*result.WebhookRegistrations) != 1 || strings.Join(requests, ",") != "GET /diagnostics,POST /diagnostics/argus-webhooks" {
		t.Fatalf("result = %+v, requests = %v", result, requests)
	}
}

func TestDiagnoseTalariaDevicesUsesPersistedWebPALoopbackEndpoint(t *testing.T) {
	store := resolverStore(t, "    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	endpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	devices := []TalariaDevice{validTalariaDevice("mac:001122334455")}
	requests := []string{}
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Port() != fmt.Sprint(endpoint.HostPort) {
			t.Fatalf("diagnostic port = %q", request.URL.Port())
		}
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/diagnostics":
			return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyTalariaDevices}}), nil
		case "/diagnostics/" + JourneyTalariaDevices:
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation != (Invocation{}) {
				t.Fatalf("inventory invocation = %+v, %v", invocation, err)
			}
			return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyTalariaDevices, ObservedAt: now, Observations: []Observation{
				{EdgeID: "talaria-reachability", State: StatePassed, ObservedAt: now},
				{EdgeID: "talaria-device-inventory", State: StatePassed, ObservedAt: now},
			}, TalariaDevices: &devices}), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", request.URL.Path)
		}
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "webpa", Target: "devices"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if result.Journey != JourneyTalariaDevices || result.Target.Type != "talaria" || result.TalariaDevices == nil || len(*result.TalariaDevices) != 1 || strings.Join(requests, ",") != "GET /diagnostics,POST /diagnostics/talaria-devices" {
		t.Fatalf("result = %+v, requests = %v", result, requests)
	}
}

func TestDiagnoseXB10ParodusClientsUsesPersistedLoopbackEndpoint(t *testing.T) {
	store := resolverStore(t, "    - name: xb10\n      type: xb10\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: xb10}\n")
	endpoint, err := store.ReserveHealthEndpoint("edge", "xb10", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	clients := []string{"apparmor-simulator", "config"}
	truncated := false
	requests := []string{}
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Port() != fmt.Sprint(endpoint.HostPort) {
			t.Fatalf("diagnostic port = %q", request.URL.Port())
		}
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/diagnostics":
			return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyParodusClients}}), nil
		case "/diagnostics/" + JourneyParodusClients:
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation != (Invocation{}) {
				t.Fatalf("Parodus invocation = %+v, %v", invocation, err)
			}
			return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyParodusClients, ObservedAt: now, Observations: []Observation{{EdgeID: "parodus-client-list", State: StatePassed, ObservedAt: now}}, ParodusClients: &clients, ParodusClientsTruncated: &truncated}), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", request.URL.Path)
		}
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "xb10", Target: "parodus"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if result.Journey != JourneyParodusClients || result.Source.Type != "xb10" || result.Target.Type != "parodus" || len(result.Nodes) != 2 || result.Nodes[0].ID != "xb10" || len(result.Edges) != 1 || result.Edges[0].From != "xb10" || result.ParodusClients == nil || strings.Join(*result.ParodusClients, ",") != "apparmor-simulator,config" || result.ParodusClientsTruncated == nil || *result.ParodusClientsTruncated || strings.Join(requests, ",") != "GET /diagnostics,POST /diagnostics/parodus-clients" {
		t.Fatalf("result = %+v, requests = %v", result, requests)
	}
}

func TestDiagnoseWebhookPassivelyCollectsBothParticipants(t *testing.T) {
	store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 21, 0, 0, 0, time.UTC)
	intentObservedAt := now.Add(-time.Second)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: intentObservedAt, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Port() {
		case fmt.Sprint(subscriberEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}), nil
			case "/diagnostics/webhook-subscriber/intent":
				return jsonResponse(intent), nil
			}
		case fmt.Sprint(webpaEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
			case "/diagnostics/webhook":
				var invocation Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.SubscriberIntent == nil || invocation.AllowActiveCallback {
					t.Fatalf("passive webhook invocation = %+v, %v", invocation, err)
				}
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{
					{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
					{EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now},
					{EdgeID: "registration-present", State: StatePassed, ObservedAt: now},
					{EdgeID: "registration-fresh", State: StatePassed, ObservedAt: now},
					{EdgeID: "registration-conformant", State: StatePassed, ObservedAt: now},
				}}), nil
			}
		}
		return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(result.Observations) != 11 || result.Observations[0].State != StatePassed || result.Observations[5].State != StatePassed || result.Observations[6].State != StateUnknown || result.Observations[6].ReasonID != ReasonActiveCallbackNotRequested {
		t.Fatalf("observations = %+v", result.Observations)
	}
	if result.Observations[0].ObservedAt != intentObservedAt || result.ObservedAt != now {
		t.Fatalf("participant timestamps = %+v, result = %s", result.Observations[0], result.ObservedAt)
	}
	if outcome, err := Classify(result); err != nil || outcome != OutcomeInconclusive {
		t.Fatalf("Classify = %q, %v", outcome, err)
	}
}

func TestDiagnoseCPECallbackExecutesOneCorrelatedEventAfterPrerequisites(t *testing.T) {
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	gatewayEndpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 13, 0, 0, 0, time.UTC)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: JourneyWebhookSubscriber, ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	requests := []string{}
	correlationID := ""
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.Port()+" "+request.Method+" "+request.URL.Path)
		switch request.URL.Port() {
		case fmt.Sprint(gatewayEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA, JourneyCPEWebPACallback}}), nil
			case "/diagnostics/cpe-webpa":
				var invocation Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.ClientService != "apparmor-simulator" || invocation.AllowActiveEvent || invocation.Event != "" || invocation.CorrelationID != "" {
					t.Fatalf("passive CPE invocation = %+v, %v", invocation, err)
				}
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed})}), nil
			case "/diagnostics/cpe-webpa-callback":
				var invocation Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || !invocation.AllowActiveEvent || invocation.ClientService != "apparmor-simulator" || invocation.Subscriber != "event-sink" || len(invocation.CorrelationID) != MaxCorrelationIDLength {
					t.Fatalf("active CPE invocation = %+v, %v", invocation, err)
				}
				correlationID = invocation.CorrelationID
				observations := append(endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed}), Observation{EdgeID: "active-event-acceptance", State: StatePassed, ObservedAt: now})
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPACallback, ObservedAt: now, Observations: observations, ActiveEvent: &CPEActiveEventResult{Accepted: true}}), nil
			}
		case fmt.Sprint(subscriberEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhookSubscriber}}), nil
			case "/diagnostics/webhook-subscriber/intent":
				return jsonResponse(intent), nil
			case "/diagnostics/webhook-subscriber/receipts/" + correlationID:
				return jsonResponse(Receipt{SchemaVersion: SchemaVersion, CorrelationID: correlationID, Source: "caduceus", AcceptedAt: now.Add(time.Second), HTTPStatus: http.StatusNoContent}), nil
			}
		case fmt.Sprint(webpaEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
			case "/diagnostics/webhook":
				var invocation Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.SubscriberIntent == nil || invocation.AllowActiveCallback || invocation.Event != "" || invocation.DeviceID != "" {
					t.Fatalf("passive WebPA invocation = %+v, %v", invocation, err)
				}
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now)}), nil
			case "/diagnostics/cpe-webpa-callback/routing":
				var input map[string]string
				if err := json.NewDecoder(request.Body).Decode(&input); err != nil || len(input) != 1 || input["correlationId"] != correlationID {
					t.Fatalf("routing input = %#v, %v", input, err)
				}
				return jsonResponse(RoutingObservation{SchemaVersion: SchemaVersion, CorrelationID: correlationID, State: "selected", ObservedAt: now}), nil
			}
		}
		return nil, fmt.Errorf("unexpected prerequisite request %s %s", request.URL, request.URL.Path)
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if result.FirstFailure != "" || correlationID == "" {
		t.Fatalf("callback result = %+v; correlation ID = %q", result, correlationID)
	}
	for _, observation := range result.Observations {
		if observation.State != StatePassed {
			t.Fatalf("callback observation = %+v", observation)
		}
	}
	want := []string{
		fmt.Sprint(gatewayEndpoint.HostPort) + " GET /diagnostics",
		fmt.Sprint(gatewayEndpoint.HostPort) + " POST /diagnostics/cpe-webpa",
		fmt.Sprint(subscriberEndpoint.HostPort) + " GET /diagnostics",
		fmt.Sprint(subscriberEndpoint.HostPort) + " GET /diagnostics/webhook-subscriber/intent",
		fmt.Sprint(webpaEndpoint.HostPort) + " GET /diagnostics",
		fmt.Sprint(webpaEndpoint.HostPort) + " POST /diagnostics/webhook",
		fmt.Sprint(gatewayEndpoint.HostPort) + " POST /diagnostics/cpe-webpa-callback",
		fmt.Sprint(webpaEndpoint.HostPort) + " POST /diagnostics/cpe-webpa-callback/routing",
		fmt.Sprint(subscriberEndpoint.HostPort) + " GET /diagnostics/webhook-subscriber/receipts/" + correlationID,
	}
	if strings.Join(requests, ",") != strings.Join(want, ",") {
		t.Fatalf("prerequisite requests = %v, want %v", requests, want)
	}
}

func TestDiagnoseCPECallbackHTTPIntegrationUsesPersistedLoopback(t *testing.T) {
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	gatewayEndpoint, _ := store.ReserveHealthEndpoint("edge", "gateway", 0)
	subscriberEndpoint, _ := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	webpaEndpoint, _ := store.ReserveHealthEndpoint("edge", "webpa", 0)
	now := time.Date(2026, 8, 17, 17, 0, 0, 0, time.UTC)
	correlationID := ""
	activeEvents := 0
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: JourneyWebhookSubscriber, ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	gateway := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics":
			_ = json.NewEncoder(writer).Encode(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA, JourneyCPEWebPACallback}})
		case "/diagnostics/cpe-webpa":
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed})})
		case "/diagnostics/cpe-webpa-callback":
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || !invocation.AllowActiveEvent || invocation.CorrelationID == "" {
				t.Fatalf("active invocation = %+v, %v", invocation, err)
			}
			activeEvents++
			correlationID = invocation.CorrelationID
			observations := append(endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed}), Observation{EdgeID: "active-event-acceptance", State: StatePassed, ObservedAt: now})
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPACallback, ObservedAt: now, Observations: observations, ActiveEvent: &CPEActiveEventResult{Accepted: true}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer gateway.Close()
	subscriber := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics":
			_ = json.NewEncoder(writer).Encode(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhookSubscriber}})
		case "/diagnostics/webhook-subscriber/intent":
			_ = json.NewEncoder(writer).Encode(intent)
		case "/diagnostics/webhook-subscriber/receipts/" + correlationID:
			_ = json.NewEncoder(writer).Encode(Receipt{SchemaVersion: SchemaVersion, CorrelationID: correlationID, Source: "caduceus", AcceptedAt: now, HTTPStatus: http.StatusNoContent})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer subscriber.Close()
	webpa := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics":
			_ = json.NewEncoder(writer).Encode(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}})
		case "/diagnostics/webhook":
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now)})
		case "/diagnostics/cpe-webpa-callback/routing":
			_ = json.NewEncoder(writer).Encode(RoutingObservation{SchemaVersion: SchemaVersion, CorrelationID: correlationID, State: "selected", ObservedAt: now})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer webpa.Close()
	addresses := map[string]string{
		net.JoinHostPort("127.0.0.1", fmt.Sprint(gatewayEndpoint.HostPort)):    strings.TrimPrefix(gateway.URL, "http://"),
		net.JoinHostPort("127.0.0.1", fmt.Sprint(subscriberEndpoint.HostPort)): strings.TrimPrefix(subscriber.URL, "http://"),
		net.JoinHostPort("127.0.0.1", fmt.Sprint(webpaEndpoint.HostPort)):      strings.TrimPrefix(webpa.URL, "http://"),
	}
	dialer := net.Dialer{}
	client := &Client{HTTPClient: &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		participantAddress, ok := addresses[address]
		if !ok {
			return nil, fmt.Errorf("diagnostic HTTP target %q is not persisted loopback state", address)
		}
		return dialer.DialContext(ctx, network, participantAddress)
	}}}}
	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
	if err != nil || activeEvents != 1 || correlationID == "" || result.FirstFailure != "" {
		t.Fatalf("result = %+v, error = %v, active events = %d, correlation ID = %q", result, err, activeEvents, correlationID)
	}
}

func TestDiagnoseCPECallbackRejectsUnmatchedSelectionBeforeActiveEvent(t *testing.T) {
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	gatewayEndpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: JourneyWebhookSubscriber, ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "other/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	activeCalls := 0
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Port() {
		case fmt.Sprint(gatewayEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA, JourneyCPEWebPACallback}}), nil
			case "/diagnostics/cpe-webpa":
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed})}), nil
			case "/diagnostics/cpe-webpa-callback":
				activeCalls++
				return nil, fmt.Errorf("unexpected active CPE request")
			}
		case fmt.Sprint(subscriberEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhookSubscriber}}), nil
			case "/diagnostics/webhook-subscriber/intent":
				return jsonResponse(intent), nil
			}
		case fmt.Sprint(webpaEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
			case "/diagnostics/webhook":
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now)}), nil
			}
		}
		return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if activeCalls != 0 || result.FirstFailure != "registration-conformant" {
		t.Fatalf("active calls = %d; result = %+v", activeCalls, result)
	}
	for _, observation := range result.Observations {
		if observation.EdgeID == "registration-conformant" && (observation.State != StateFailed || observation.ReasonID != ReasonRegistrationMismatch) {
			t.Fatalf("selection observation = %+v", observation)
		}
	}
}

func TestDiagnoseCPECallbackStateMatrix(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		stage            string
		wantFailure      string
		wantUnknown      string
		wantActiveCalls  int
		wantRoutingCalls int
		wantReceiptCalls int
	}{
		{name: "CPE application", stage: "application-parodus", wantFailure: "application-parodus"},
		{name: "Talaria DNS", stage: "talaria-dns", wantFailure: "talaria-dns"},
		{name: "Talaria transport", stage: "talaria-transport", wantFailure: "talaria-transport"},
		{name: "Talaria authentication", stage: "talaria-authentication", wantFailure: "talaria-authentication"},
		{name: "device registration", stage: "device-registration", wantFailure: "device-registration"},
		{name: "Argus", stage: "argus-reachability", wantFailure: "argus-reachability"},
		{name: "registration", stage: "registration-present", wantFailure: "registration-present"},
		{name: "subscriber malformed", stage: "subscriber-malformed", wantUnknown: "subscriber-intent"},
		{name: "WebPA malformed", stage: "webpa-malformed", wantUnknown: "argus-reachability"},
		{name: "active CPE malformed", stage: "active-malformed", wantUnknown: "active-event-acceptance", wantActiveCalls: 1},
		{name: "routing missing", stage: "routing-missing", wantUnknown: "routing-observation", wantActiveCalls: 1, wantRoutingCalls: 1},
		{name: "routing malformed", stage: "routing-malformed", wantUnknown: "routing-observation", wantActiveCalls: 1, wantRoutingCalls: 1},
		{name: "receipt missing", stage: "receipt-missing", wantUnknown: "callback-receipt", wantActiveCalls: 1, wantRoutingCalls: 1, wantReceiptCalls: MaxReceiptPollAttempts},
		{name: "receipt malformed", stage: "receipt-malformed", wantUnknown: "callback-receipt", wantActiveCalls: 1, wantRoutingCalls: 1, wantReceiptCalls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, activeCalls, routingCalls, receiptCalls := diagnoseCPECallbackFixture(t, testCase.stage)
			if result.FirstFailure != testCase.wantFailure || activeCalls != testCase.wantActiveCalls || routingCalls != testCase.wantRoutingCalls || receiptCalls != testCase.wantReceiptCalls {
				t.Fatalf("result = %+v; active/routing/receipt calls = %d/%d/%d", result, activeCalls, routingCalls, receiptCalls)
			}
			if testCase.wantUnknown != "" {
				for _, observation := range result.Observations {
					if observation.EdgeID == testCase.wantUnknown {
						if observation.State != StateUnknown {
							t.Fatalf("%s observation = %+v", testCase.wantUnknown, observation)
						}
						return
					}
				}
				t.Fatalf("missing %q observation", testCase.wantUnknown)
			}
		})
	}
}

func diagnoseCPECallbackFixture(t *testing.T, stage string) (Result, int, int, int) {
	t.Helper()
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	gatewayEndpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: JourneyWebhookSubscriber, ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	correlationID := ""
	activeCalls, routingCalls, receiptCalls := 0, 0, 0
	client := &Client{ReceiptPollInterval: time.Millisecond, HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Port() {
		case fmt.Sprint(gatewayEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA, JourneyCPEWebPACallback}}), nil
			case "/diagnostics/cpe-webpa":
				observations := endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed})
				for index := range observations {
					if observations[index].EdgeID == stage {
						observations[index].State = StateFailed
					}
				}
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: observations}), nil
			case "/diagnostics/cpe-webpa-callback":
				activeCalls++
				if stage == "active-malformed" {
					return jsonResponse(EndpointResponse{}), nil
				}
				var invocation Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
					t.Fatal(err)
				}
				correlationID = invocation.CorrelationID
				observations := append(endpointObservations(now, []State{StatePassed, StatePassed, StatePassed, StatePassed, StatePassed}), Observation{EdgeID: "active-event-acceptance", State: StatePassed, ObservedAt: now})
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPACallback, ObservedAt: now, Observations: observations, ActiveEvent: &CPEActiveEventResult{Accepted: true}}), nil
			}
		case fmt.Sprint(subscriberEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhookSubscriber}}), nil
			case "/diagnostics/webhook-subscriber/intent":
				if stage == "subscriber-malformed" {
					return jsonResponse(WebhookSubscriberIntent{}), nil
				}
				return jsonResponse(intent), nil
			case "/diagnostics/webhook-subscriber/receipts/" + correlationID:
				receiptCalls++
				if stage == "receipt-missing" {
					return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
				}
				if stage == "receipt-malformed" {
					return jsonResponse(Receipt{}), nil
				}
				return jsonResponse(Receipt{SchemaVersion: SchemaVersion, CorrelationID: correlationID, Source: "caduceus", AcceptedAt: now, HTTPStatus: http.StatusNoContent}), nil
			}
		case fmt.Sprint(webpaEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
			case "/diagnostics/webhook":
				if stage == "webpa-malformed" {
					return jsonResponse(EndpointResponse{}), nil
				}
				observations := webhookPassedObservations(now)
				for index := range observations {
					if observations[index].EdgeID == stage {
						observations[index].State = StateFailed
					}
				}
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: observations}), nil
			case "/diagnostics/cpe-webpa-callback/routing":
				routingCalls++
				if stage == "routing-missing" {
					return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
				}
				if stage == "routing-malformed" {
					return jsonResponse(RoutingObservation{}), nil
				}
				return jsonResponse(RoutingObservation{SchemaVersion: SchemaVersion, CorrelationID: correlationID, State: "selected", ObservedAt: now}), nil
			}
		}
		return nil, fmt.Errorf("unexpected callback fixture request %s %s", request.URL, request.URL.Path)
	})}}
	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	return result, activeCalls, routingCalls, receiptCalls
}

func TestDiagnoseWebhookPassiveUsesPersistedLoopbackHTTPParticipants(t *testing.T) {
	store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 22, 0, 0, 0, time.UTC)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now.Add(-time.Second), CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	var subscriberRequests, webpaRequests []string
	subscriber := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		subscriberRequests = append(subscriberRequests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/diagnostics":
			_ = json.NewEncoder(writer).Encode(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}})
		case "/diagnostics/webhook-subscriber/intent":
			_ = json.NewEncoder(writer).Encode(intent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer subscriber.Close()
	webpa := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		webpaRequests = append(webpaRequests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/diagnostics":
			_ = json.NewEncoder(writer).Encode(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}})
		case "/diagnostics/webhook":
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
				t.Fatalf("decode passive webhook invocation: %v", err)
			}
			if invocation.SubscriberIntent == nil || invocation.AllowActiveCallback || invocation.Event != "" || invocation.DeviceID != "" || invocation.ActivePhase != "" {
				t.Fatalf("passive invocation generated active traffic: %+v", invocation)
			}
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{
				{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
				{EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now},
				{EdgeID: "registration-present", State: StatePassed, ObservedAt: now},
				{EdgeID: "registration-fresh", State: StatePassed, ObservedAt: now},
				{EdgeID: "registration-conformant", State: StatePassed, ObservedAt: now},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer webpa.Close()

	persistedAddresses := map[string]string{
		net.JoinHostPort("127.0.0.1", fmt.Sprint(subscriberEndpoint.HostPort)): strings.TrimPrefix(subscriber.URL, "http://"),
		net.JoinHostPort("127.0.0.1", fmt.Sprint(webpaEndpoint.HostPort)):      strings.TrimPrefix(webpa.URL, "http://"),
	}
	dialer := net.Dialer{}
	client := &Client{HTTPClient: &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		participantAddress, ok := persistedAddresses[address]
		if !ok {
			return nil, fmt.Errorf("diagnostic HTTP target %q is not a persisted loopback endpoint", address)
		}
		return dialer.DialContext(ctx, network, participantAddress)
	}}}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if result.Observations[6].ReasonID != ReasonActiveCallbackNotRequested {
		t.Fatalf("passive result did not retain inactive callback stages: %+v", result.Observations[6])
	}
	if got, want := subscriberRequests, []string{"GET /diagnostics", "GET /diagnostics/webhook-subscriber/intent"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("subscriber requests = %v, want %v", got, want)
	}
	if got, want := webpaRequests, []string{"GET /diagnostics", "POST /diagnostics/webhook"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("WebPA requests = %v, want %v", got, want)
	}
}

func TestDiagnoseWebhookPreservesIncompleteWebPAResults(t *testing.T) {
	now := time.Date(2026, 8, 14, 21, 30, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name          string
		webpaResponse func(*http.Request) (*http.Response, error)
	}{
		{name: "transport", webpaResponse: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}},
		{name: "malformed response", webpaResponse: func(*http.Request) (*http.Response, error) {
			return jsonResponse(Capabilities{}), nil
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
			subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
			if err != nil {
				t.Fatal(err)
			}
			webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
			if err != nil {
				t.Fatal(err)
			}
			intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
			client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Port() {
				case fmt.Sprint(subscriberEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}), nil
					case "/diagnostics/webhook-subscriber/intent":
						return jsonResponse(intent), nil
					}
				case fmt.Sprint(webpaEndpoint.HostPort):
					return testCase.webpaResponse(request)
				}
				return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
			})}}

			result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook"})
			if err != nil {
				t.Fatalf("Diagnose: %v", err)
			}
			if result.Observations[0].State != StatePassed || result.Observations[0].ObservedAt != now || result.Observations[1].State != StateUnknown || result.Observations[1].ReasonID != ReasonParticipantResultIncomplete || result.FirstFailure != "" {
				t.Fatalf("incomplete observations = %+v, first failure = %q", result.Observations, result.FirstFailure)
			}
			if outcome, err := Classify(result); err != nil || outcome != OutcomeInconclusive {
				t.Fatalf("Classify = %q, %v", outcome, err)
			}
		})
	}
}

func TestDiagnoseWebhookActivelyFollowsCausalOrderOverLoopback(t *testing.T) {
	store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 22, 0, 0, 0, time.UTC)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	const directCorrelationID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const caduceusCorrelationID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	order := make([]string, 0, 7)
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() != "127.0.0.1" {
			t.Fatalf("diagnostic request escaped loopback: %s", request.URL)
		}
		switch request.URL.Port() {
		case fmt.Sprint(subscriberEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				order = append(order, "subscriber capabilities")
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}), nil
			case "/diagnostics/webhook-subscriber/intent":
				order = append(order, "subscriber intent")
				return jsonResponse(intent), nil
			case "/diagnostics/webhook-subscriber/receipts/" + directCorrelationID:
				order = append(order, "direct receipt")
				return jsonResponse(Receipt{SchemaVersion: SchemaVersion, CorrelationID: directCorrelationID, Source: WebhookActiveDirect, AcceptedAt: now.Add(time.Second), HTTPStatus: http.StatusNoContent}), nil
			case "/diagnostics/webhook-subscriber/receipts/" + caduceusCorrelationID:
				order = append(order, "caduceus receipt")
				return jsonResponse(Receipt{SchemaVersion: SchemaVersion, CorrelationID: caduceusCorrelationID, Source: WebhookActiveCaduceus, AcceptedAt: now.Add(2 * time.Second), HTTPStatus: http.StatusNoContent}), nil
			}
		case fmt.Sprint(webpaEndpoint.HostPort):
			switch request.URL.Path {
			case "/diagnostics":
				order = append(order, "webpa capabilities")
				return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
			case "/diagnostics/webhook":
				var invocation Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
					t.Fatal(err)
				}
				if invocation.SubscriberIntent == nil || !invocation.AllowActiveCallback || invocation.Event != "devices/diagnostic" || invocation.DeviceID != "mac:001122334455" {
					t.Fatalf("active webhook invocation = %+v", invocation)
				}
				correlationID := directCorrelationID
				status := http.StatusNoContent
				phase := "direct callback"
				if invocation.ActivePhase == WebhookActiveCaduceus {
					correlationID = caduceusCorrelationID
					status = http.StatusAccepted
					phase = "caduceus injection"
				}
				order = append(order, phase)
				return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now), Active: &WebhookActiveResult{Phase: invocation.ActivePhase, CorrelationID: correlationID, HTTPStatus: status}}), nil
			}
		}
		return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
	})}}

	result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook", AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	wantOrder := []string{"subscriber capabilities", "subscriber intent", "webpa capabilities", "direct callback", "direct receipt", "caduceus injection", "caduceus receipt"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("request order = %v, want %v", order, wantOrder)
	}
	for _, observation := range result.Observations {
		if observation.State != StatePassed {
			t.Fatalf("observation = %+v", observation)
		}
	}
	if outcome, err := Classify(result); err != nil || outcome != OutcomePassed {
		t.Fatalf("Classify = %q, %v", outcome, err)
	}
}

func TestDiagnoseWebhookActivePreservesWebPAFailureBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 14, 22, 30, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name         string
		observations []Observation
		wantFailure  string
	}{
		{name: "Argus reachability", observations: []Observation{{EdgeID: "argus-reachability", State: StateFailed, ObservedAt: now}}, wantFailure: "argus-reachability"},
		{name: "Argus authentication", observations: []Observation{{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now}, {EdgeID: "argus-authentication", State: StateFailed, ObservedAt: now}}, wantFailure: "argus-authentication"},
		{name: "registration presence", observations: []Observation{{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now}, {EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now}, {EdgeID: "registration-present", State: StateFailed, ObservedAt: now}}, wantFailure: "registration-present"},
		{name: "registration freshness", observations: []Observation{{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now}, {EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now}, {EdgeID: "registration-present", State: StatePassed, ObservedAt: now}, {EdgeID: "registration-fresh", State: StateFailed, ObservedAt: now}}, wantFailure: "registration-fresh"},
		{name: "registration conformance", observations: []Observation{{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now}, {EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now}, {EdgeID: "registration-present", State: StatePassed, ObservedAt: now}, {EdgeID: "registration-fresh", State: StatePassed, ObservedAt: now}, {EdgeID: "registration-conformant", State: StateFailed, ObservedAt: now}}, wantFailure: "registration-conformant"},
		{name: "callback DNS", observations: append(webhookPassedObservations(now), Observation{EdgeID: "callback-dns", State: StateFailed, ObservedAt: now}), wantFailure: "callback-dns"},
		{name: "callback transport", observations: append(webhookPassedObservations(now), Observation{EdgeID: "callback-dns", State: StatePassed, ObservedAt: now}, Observation{EdgeID: "callback-transport", State: StateFailed, ObservedAt: now}), wantFailure: "callback-transport"},
		{name: "callback acceptance", observations: append(webhookPassedObservations(now), Observation{EdgeID: "callback-dns", State: StatePassed, ObservedAt: now}, Observation{EdgeID: "callback-transport", State: StatePassed, ObservedAt: now}, Observation{EdgeID: "callback-acceptance", State: StateFailed, ObservedAt: now}), wantFailure: "callback-acceptance"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
			subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
			if err != nil {
				t.Fatal(err)
			}
			webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
			if err != nil {
				t.Fatal(err)
			}
			intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
			caduceusRequests := 0
			receiptRequests := 0
			client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Port() {
				case fmt.Sprint(subscriberEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}), nil
					case "/diagnostics/webhook-subscriber/intent":
						return jsonResponse(intent), nil
					default:
						receiptRequests++
						return nil, fmt.Errorf("unexpected receipt request %s", request.URL.Path)
					}
				case fmt.Sprint(webpaEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
					case "/diagnostics/webhook":
						var invocation Invocation
						if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
							t.Fatal(err)
						}
						if invocation.ActivePhase != WebhookActiveDirect {
							caduceusRequests++
							return nil, fmt.Errorf("unexpected active phase %q", invocation.ActivePhase)
						}
						return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: testCase.observations}), nil
					}
				}
				return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
			})}}

			result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook", AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
			if err != nil {
				t.Fatalf("Diagnose: %v", err)
			}
			if result.FirstFailure != testCase.wantFailure {
				t.Fatalf("FirstFailure = %q, want %q; observations = %+v", result.FirstFailure, testCase.wantFailure, result.Observations)
			}
			if caduceusRequests != 0 || receiptRequests != 0 {
				t.Fatalf("downstream requests: Caduceus = %d, receipts = %d", caduceusRequests, receiptRequests)
			}
			if outcome, err := Classify(result); err != nil || outcome != OutcomeFailed {
				t.Fatalf("Classify = %q, %v", outcome, err)
			}
		})
	}
}

func TestDiagnoseWebhookActiveCaduceusBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC)
	const directCorrelationID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const caduceusCorrelationID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for _, testCase := range []struct {
		name            string
		caduceusResult  EndpointResponse
		receiptResponse func() (*http.Response, error)
		wantFailure     string
		wantUnknown     string
	}{
		{
			name:           "ingestion failure",
			caduceusResult: EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: append(webhookPassedObservations(now), Observation{EdgeID: "caduceus-ingestion", State: StateFailed, ObservedAt: now})},
			wantFailure:    "caduceus-ingestion",
		},
		{
			name:            "receipt unavailable",
			caduceusResult:  EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now), Active: &WebhookActiveResult{Phase: WebhookActiveCaduceus, CorrelationID: caduceusCorrelationID, HTTPStatus: http.StatusAccepted}},
			receiptResponse: func() (*http.Response, error) { return nil, errors.New("connection refused") },
			wantUnknown:     "caduceus-receipt",
		},
		{
			name:            "receipt malformed",
			caduceusResult:  EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now), Active: &WebhookActiveResult{Phase: WebhookActiveCaduceus, CorrelationID: caduceusCorrelationID, HTTPStatus: http.StatusAccepted}},
			receiptResponse: func() (*http.Response, error) { return jsonResponse(Receipt{}), nil },
			wantUnknown:     "caduceus-receipt",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
			subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
			if err != nil {
				t.Fatal(err)
			}
			webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
			if err != nil {
				t.Fatal(err)
			}
			intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
			client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Port() {
				case fmt.Sprint(subscriberEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}), nil
					case "/diagnostics/webhook-subscriber/intent":
						return jsonResponse(intent), nil
					case "/diagnostics/webhook-subscriber/receipts/" + directCorrelationID:
						return jsonResponse(Receipt{SchemaVersion: SchemaVersion, CorrelationID: directCorrelationID, Source: WebhookActiveDirect, AcceptedAt: now.Add(time.Second), HTTPStatus: http.StatusNoContent}), nil
					case "/diagnostics/webhook-subscriber/receipts/" + caduceusCorrelationID:
						if testCase.receiptResponse == nil {
							t.Fatalf("unexpected Caduceus receipt poll")
						}
						return testCase.receiptResponse()
					}
				case fmt.Sprint(webpaEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
					case "/diagnostics/webhook":
						var invocation Invocation
						if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
							t.Fatal(err)
						}
						switch invocation.ActivePhase {
						case WebhookActiveDirect:
							return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now), Active: &WebhookActiveResult{Phase: WebhookActiveDirect, CorrelationID: directCorrelationID, HTTPStatus: http.StatusNoContent}}), nil
						case WebhookActiveCaduceus:
							return jsonResponse(testCase.caduceusResult), nil
						}
					}
				}
				return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
			})}}

			result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook", AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
			if err != nil {
				t.Fatalf("Diagnose: %v", err)
			}
			if testCase.wantFailure != "" && result.FirstFailure != testCase.wantFailure {
				t.Fatalf("FirstFailure = %q, want %q; observations = %+v", result.FirstFailure, testCase.wantFailure, result.Observations)
			}
			if testCase.wantUnknown != "" {
				observation := result.Observations[len(result.Observations)-1]
				if observation.EdgeID != testCase.wantUnknown || observation.State != StateUnknown || observation.ReasonID != ReasonParticipantResultIncomplete {
					t.Fatalf("receipt observation = %+v", observation)
				}
			}
		})
	}
}

func TestDiagnoseWebhookParticipantUnavailableOrMalformed(t *testing.T) {
	now := time.Date(2026, 8, 14, 23, 30, 0, 0, time.UTC)
	const directCorrelationID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, testCase := range []struct {
		name      string
		failureAt string
		wantEdge  string
	}{
		{name: "subscriber unavailable", failureAt: "subscriber-capabilities-unavailable", wantEdge: "subscriber-intent"},
		{name: "subscriber intent malformed", failureAt: "subscriber-intent-malformed", wantEdge: "subscriber-intent"},
		{name: "WebPA unavailable", failureAt: "webpa-capabilities-unavailable", wantEdge: "argus-reachability"},
		{name: "WebPA capabilities malformed", failureAt: "webpa-capabilities-malformed", wantEdge: "argus-reachability"},
		{name: "direct acknowledgement malformed", failureAt: "direct-response-malformed", wantEdge: "callback-dns"},
		{name: "direct receipt unavailable", failureAt: "direct-receipt-unavailable", wantEdge: "callback-acceptance"},
		{name: "direct receipt malformed", failureAt: "direct-receipt-malformed", wantEdge: "callback-acceptance"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
			subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
			if err != nil {
				t.Fatal(err)
			}
			webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
			if err != nil {
				t.Fatal(err)
			}
			intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
			caduceusRequests := 0
			client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Port() {
				case fmt.Sprint(subscriberEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						if testCase.failureAt == "subscriber-capabilities-unavailable" {
							return nil, errors.New("connection refused")
						}
						return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}), nil
					case "/diagnostics/webhook-subscriber/intent":
						if testCase.failureAt == "subscriber-intent-malformed" {
							return jsonResponse(WebhookSubscriberIntent{}), nil
						}
						return jsonResponse(intent), nil
					case "/diagnostics/webhook-subscriber/receipts/" + directCorrelationID:
						switch testCase.failureAt {
						case "direct-receipt-unavailable":
							return nil, errors.New("connection refused")
						case "direct-receipt-malformed":
							return jsonResponse(Receipt{}), nil
						default:
							return jsonResponse(Receipt{SchemaVersion: SchemaVersion, CorrelationID: directCorrelationID, Source: WebhookActiveDirect, AcceptedAt: now.Add(time.Second), HTTPStatus: http.StatusNoContent}), nil
						}
					}
				case fmt.Sprint(webpaEndpoint.HostPort):
					switch request.URL.Path {
					case "/diagnostics":
						switch testCase.failureAt {
						case "webpa-capabilities-unavailable":
							return nil, errors.New("connection refused")
						case "webpa-capabilities-malformed":
							return jsonResponse(Capabilities{}), nil
						default:
							return jsonResponse(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyWebhook}}), nil
						}
					case "/diagnostics/webhook":
						var invocation Invocation
						if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
							t.Fatal(err)
						}
						if invocation.ActivePhase == WebhookActiveCaduceus {
							caduceusRequests++
							return nil, fmt.Errorf("unexpected Caduceus request")
						}
						if testCase.failureAt == "direct-response-malformed" {
							return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now)}), nil
						}
						return jsonResponse(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: webhookPassedObservations(now), Active: &WebhookActiveResult{Phase: WebhookActiveDirect, CorrelationID: directCorrelationID, HTTPStatus: http.StatusNoContent}}), nil
					}
				}
				return nil, fmt.Errorf("unexpected request %s %s", request.URL, request.URL.Path)
			})}}

			result, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook", AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"})
			if err != nil {
				t.Fatalf("Diagnose: %v", err)
			}
			for _, observation := range result.Observations {
				if observation.EdgeID == testCase.wantEdge && (observation.State != StateUnknown || observation.ReasonID != ReasonParticipantResultIncomplete) {
					t.Fatalf("boundary observation = %+v", observation)
				}
			}
			if caduceusRequests != 0 {
				t.Fatalf("Caduceus requests = %d, want 0", caduceusRequests)
			}
		})
	}
}

func TestDiagnoseWebhookRejectsInvalidActiveInputBeforeParticipantRequests(t *testing.T) {
	store := resolverStore(t, "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	if _, err := store.ReserveHealthEndpoint("edge", "event-sink", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveHealthEndpoint("edge", "webpa", 0); err != nil {
		t.Fatal(err)
	}
	client := &Client{HTTPClient: &http.Client{Transport: clientRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid active input sent a participant request")
		return nil, nil
	})}}
	_, err := Diagnose(context.Background(), store, resolverRegistry(), client, ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook", AllowActiveCallback: true, DeviceID: "mac:001122334455"})
	if err == nil || !strings.Contains(err.Error(), "event") {
		t.Fatalf("Diagnose error = %v", err)
	}
}

func webhookPassedObservations(observedAt time.Time) []Observation {
	return []Observation{
		{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: observedAt},
		{EdgeID: "argus-authentication", State: StatePassed, ObservedAt: observedAt},
		{EdgeID: "registration-present", State: StatePassed, ObservedAt: observedAt},
		{EdgeID: "registration-fresh", State: StatePassed, ObservedAt: observedAt},
		{EdgeID: "registration-conformant", State: StatePassed, ObservedAt: observedAt},
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
