package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/gdcs-dev/vcpe/controlplane/internal/health"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

func TestStatusOutputModes(t *testing.T) {
	stateRoot := t.TempDir()

	human, err := executeLocal(Options{Command: "status", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("human status: %v", err)
	}
	if !strings.HasPrefix(human.Message, "vCPE status") {
		t.Fatalf("expected human status, got %q", human.Message)
	}

	jsonOut, err := executeLocal(Options{Command: "status", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("json status: %v", err)
	}
	for _, key := range []string{"\"metrics\"", "\"desired\"", "\"planned\"", "\"observed\"", "\"runtimeInitDiagnostics\""} {
		if !strings.Contains(jsonOut.Message, key) {
			t.Fatalf("expected %s in json status, got %q", key, jsonOut.Message)
		}
	}
}

func TestNamedStatusCollectsPersistedHealthOverHTTP(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	endpoint, err := store.ReserveHealthEndpoint("edge", "bng", 0)
	if err != nil {
		t.Fatalf("reserve endpoint: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(endpoint.HostPort))
	if err != nil {
		t.Fatalf("listen on reserved port: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/health" {
			t.Errorf("status request = %s %s, want GET /health", request.Method, request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(health.Response{SchemaVersion: health.SchemaVersion, Status: health.StatusHealthy, ObservedAt: time.Now().UTC(), Checks: []health.Check{{Name: "service", Status: health.StatusHealthy}}})
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })

	human, err := executeLocal(Options{Command: "status", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("human status: %v", err)
	}
	if !strings.Contains(human.Message, "health bng/0: healthy") {
		t.Fatalf("human health status missing: %s", human.Message)
	}
	jsonStatus, err := executeLocal(Options{Command: "status", Name: "edge", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("json status: %v", err)
	}
	if !strings.Contains(jsonStatus.Message, `"state": "healthy"`) || !strings.Contains(jsonStatus.Message, `"service": "bng"`) {
		t.Fatalf("json health status missing: %s", jsonStatus.Message)
	}
}

type diagnosticRoundTripFunc func(*http.Request) (*http.Response, error)

func (function diagnosticRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestDiagnoseUsesPersistedLoopbackHTTPAndRendersResult(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := []byte("apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: edge}\nspec:\n  networks:\n    - role: wan\n      ipv4: {cidr: 10.0.0.0/24}\n  services:\n    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	if err := store.SaveDesiredSnapshot("edge", snapshot); err != nil {
		t.Fatal(err)
	}
	endpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	now := time.Now().UTC()
	var requests []string
	previousFactory := newDiagnosticClient
	newDiagnosticClient = func(time.Duration) *diagnostic.Client {
		return &diagnostic.Client{HTTPClient: &http.Client{Transport: diagnosticRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host != "127.0.0.1:"+strconv.Itoa(endpoint.HostPort) {
				t.Errorf("diagnostic host = %q", request.URL.Host)
			}
			requests = append(requests, request.Method+" "+request.URL.Path)
			var payload any
			if request.URL.Path == "/diagnostics" {
				payload = diagnostic.Capabilities{SchemaVersion: diagnostic.CapabilitiesSchema, Journeys: []string{diagnostic.JourneyCPEWebPA}}
			} else {
				var invocation diagnostic.Invocation
				if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.ClientService != "config" {
					t.Errorf("diagnostic invocation = %+v, error = %v", invocation, err)
				}
				states := []diagnostic.State{diagnostic.StateUnknown, diagnostic.StatePassed, diagnostic.StatePassed, diagnostic.StatePassed, diagnostic.StatePassed}
				ids := []string{"application-parodus", "talaria-dns", "talaria-transport", "talaria-authentication", "device-registration"}
				observations := make([]diagnostic.Observation, len(ids))
				for index := range ids {
					observations[index] = diagnostic.Observation{EdgeID: ids[index], State: states[index], ObservedAt: now}
				}
				payload = diagnostic.EndpointResponse{SchemaVersion: diagnostic.SchemaVersion, Journey: diagnostic.JourneyCPEWebPA, ObservedAt: now, Observations: observations}
			}
			body, _ := json.Marshal(payload)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
		})}}
	}
	t.Cleanup(func() { newDiagnosticClient = previousFactory })

	response, err := executeLocal(Options{Command: "diagnose", From: "gateway", To: "webpa", ClientService: "config", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "inconclusive") {
		t.Fatalf("diagnose error = %v", err)
	}
	if !strings.Contains(response.Message, "--UNKNOWN-->") || !strings.Contains(response.Message, "--PASSED-->") {
		t.Fatalf("diagnose output = %s", response.Message)
	}
	wantRequests := []string{"GET /diagnostics", "POST /diagnostics/cpe-webpa"}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests = %#v, want %#v", requests, wantRequests)
	}

	response, err = executeLocal(Options{Command: "diagnose", Name: "edge", From: "gateway", To: "webpa", ClientService: "config", StateRoot: stateRoot, OutputJSON: true})
	if err == nil || !strings.Contains(response.Message, `"schemaVersion": "vcpe.dev/diagnostic/v1"`) {
		t.Fatalf("JSON diagnose response = %s, error = %v", response.Message, err)
	}

	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDesiredSnapshot("other", snapshot); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	_, err = executeLocal(Options{Command: "diagnose", From: "gateway", To: "webpa", ClientService: "config", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "multiple deployments active; specify one with --name") {
		t.Fatalf("ambiguous diagnose error = %v", err)
	}
}

func TestDiagnoseWebhookUsesPersistedLoopbackEndpointsPassively(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := []byte("apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: edge}\nspec:\n  networks:\n    - role: wan\n      ipv4: {cidr: 10.0.0.0/24}\n  services:\n    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	if err := store.SaveDesiredSnapshot("edge", snapshot); err != nil {
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
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	intent := diagnostic.WebhookSubscriberIntent{SchemaVersion: diagnostic.SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	var requests []string
	previousFactory := newDiagnosticClient
	newDiagnosticClient = func(time.Duration) *diagnostic.Client {
		return &diagnostic.Client{HTTPClient: &http.Client{Transport: diagnosticRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.Method+" "+request.URL.Host+request.URL.Path)
			var payload any
			switch request.URL.Host {
			case "127.0.0.1:" + strconv.Itoa(subscriberEndpoint.HostPort):
				switch request.URL.Path {
				case "/diagnostics":
					payload = diagnostic.Capabilities{SchemaVersion: diagnostic.CapabilitiesSchema, Journeys: []string{"webhook-subscriber"}}
				case "/diagnostics/webhook-subscriber/intent":
					payload = intent
				}
			case "127.0.0.1:" + strconv.Itoa(webpaEndpoint.HostPort):
				switch request.URL.Path {
				case "/diagnostics":
					payload = diagnostic.Capabilities{SchemaVersion: diagnostic.CapabilitiesSchema, Journeys: []string{diagnostic.JourneyWebhook}}
				case "/diagnostics/webhook":
					var invocation diagnostic.Invocation
					if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.SubscriberIntent == nil || invocation.AllowActiveCallback || invocation.Event != "" || invocation.DeviceID != "" {
						t.Errorf("passive webhook invocation = %+v, error = %v", invocation, err)
					}
					payload = diagnostic.EndpointResponse{SchemaVersion: diagnostic.SchemaVersion, Journey: diagnostic.JourneyWebhook, ObservedAt: now, Observations: []diagnostic.Observation{
						{EdgeID: "argus-reachability", State: diagnostic.StatePassed, ObservedAt: now},
						{EdgeID: "argus-authentication", State: diagnostic.StatePassed, ObservedAt: now},
						{EdgeID: "registration-present", State: diagnostic.StatePassed, ObservedAt: now},
						{EdgeID: "registration-fresh", State: diagnostic.StatePassed, ObservedAt: now},
						{EdgeID: "registration-conformant", State: diagnostic.StatePassed, ObservedAt: now},
					}}
				}
			}
			if payload == nil {
				return nil, fmt.Errorf("unexpected passive webhook request %s %s", request.Method, request.URL)
			}
			body, _ := json.Marshal(payload)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
		})}}
	}
	t.Cleanup(func() { newDiagnosticClient = previousFactory })

	response, err := executeLocal(Options{Command: "diagnose", Name: "edge", From: "event-sink", To: "webhook", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "inconclusive") {
		t.Fatalf("diagnose error = %v", err)
	}
	if !strings.Contains(response.Message, "--UNKNOWN-->") || !strings.Contains(response.Message, "active callback diagnosis was not requested") {
		t.Fatalf("passive webhook graph = %s", response.Message)
	}
	wantRequests := []string{
		"GET 127.0.0.1:" + strconv.Itoa(subscriberEndpoint.HostPort) + "/diagnostics",
		"GET 127.0.0.1:" + strconv.Itoa(subscriberEndpoint.HostPort) + "/diagnostics/webhook-subscriber/intent",
		"GET 127.0.0.1:" + strconv.Itoa(webpaEndpoint.HostPort) + "/diagnostics",
		"POST 127.0.0.1:" + strconv.Itoa(webpaEndpoint.HostPort) + "/diagnostics/webhook",
	}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests = %#v, want %#v", requests, wantRequests)
	}
}

func TestDiagnoseCallbackUsesPersistedLoopbackEndpoints(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := []byte("apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: edge}\nspec:\n  networks:\n    - role: wan\n      ipv4: {cidr: 10.0.0.0/24}\n  services:\n    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	if err := store.SaveDesiredSnapshot("edge", snapshot); err != nil {
		t.Fatal(err)
	}
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
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	correlationID := ""
	previousFactory := newDiagnosticClient
	newDiagnosticClient = func(time.Duration) *diagnostic.Client {
		return &diagnostic.Client{HTTPClient: &http.Client{Transport: diagnosticRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			var payload any
			switch request.URL.Host {
			case "127.0.0.1:" + strconv.Itoa(gatewayEndpoint.HostPort):
				switch request.URL.Path {
				case "/diagnostics":
					payload = diagnostic.Capabilities{SchemaVersion: diagnostic.CapabilitiesSchema, Journeys: []string{diagnostic.JourneyCPEWebPA, diagnostic.JourneyCPEWebPACallback}}
				case "/diagnostics/cpe-webpa":
					payload = diagnostic.EndpointResponse{SchemaVersion: diagnostic.SchemaVersion, Journey: diagnostic.JourneyCPEWebPA, ObservedAt: now, Observations: callbackCPEObservations(now, false)}
				case "/diagnostics/cpe-webpa-callback":
					var invocation diagnostic.Invocation
					if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || !invocation.AllowActiveEvent || invocation.ClientService != "apparmor-simulator" || invocation.Subscriber != "event-sink" {
						t.Fatalf("callback invocation = %+v, %v", invocation, err)
					}
					correlationID = invocation.CorrelationID
					payload = diagnostic.EndpointResponse{SchemaVersion: diagnostic.SchemaVersion, Journey: diagnostic.JourneyCPEWebPACallback, ObservedAt: now, Observations: callbackCPEObservations(now, true), ActiveEvent: &diagnostic.CPEActiveEventResult{Accepted: true}}
				}
			case "127.0.0.1:" + strconv.Itoa(subscriberEndpoint.HostPort):
				switch request.URL.Path {
				case "/diagnostics":
					payload = diagnostic.Capabilities{SchemaVersion: diagnostic.CapabilitiesSchema, Journeys: []string{diagnostic.JourneyWebhookSubscriber}}
				case "/diagnostics/webhook-subscriber/intent":
					payload = diagnostic.WebhookSubscriberIntent{SchemaVersion: diagnostic.SchemaVersion, Journey: diagnostic.JourneyWebhookSubscriber, ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
				case "/diagnostics/webhook-subscriber/receipts/" + correlationID:
					payload = diagnostic.Receipt{SchemaVersion: diagnostic.SchemaVersion, CorrelationID: correlationID, Source: "caduceus", AcceptedAt: now, HTTPStatus: http.StatusNoContent}
				}
			case "127.0.0.1:" + strconv.Itoa(webpaEndpoint.HostPort):
				switch request.URL.Path {
				case "/diagnostics":
					payload = diagnostic.Capabilities{SchemaVersion: diagnostic.CapabilitiesSchema, Journeys: []string{diagnostic.JourneyWebhook}}
				case "/diagnostics/webhook":
					payload = diagnostic.EndpointResponse{SchemaVersion: diagnostic.SchemaVersion, Journey: diagnostic.JourneyWebhook, ObservedAt: now, Observations: callbackWebhookObservations(now)}
				case "/diagnostics/cpe-webpa-callback/routing":
					payload = diagnostic.RoutingObservation{SchemaVersion: diagnostic.SchemaVersion, CorrelationID: correlationID, State: "selected", ObservedAt: now}
				}
			}
			if payload == nil {
				return nil, fmt.Errorf("unexpected callback request %s %s", request.Method, request.URL)
			}
			body, _ := json.Marshal(payload)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
		})}}
	}
	t.Cleanup(func() { newDiagnosticClient = previousFactory })

	response, err := executeLocal(Options{Command: "diagnose", Name: "edge", From: "gateway", To: "callback", ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455", StateRoot: stateRoot})
	if err != nil || correlationID == "" || !strings.Contains(response.Message, "Result: passed") || strings.Contains(response.Message, correlationID) {
		t.Fatalf("callback diagnose response = %+v, error = %v, correlation ID = %q", response, err, correlationID)
	}
}

func callbackCPEObservations(observedAt time.Time, active bool) []diagnostic.Observation {
	observations := []diagnostic.Observation{
		{EdgeID: "application-parodus", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "talaria-dns", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "talaria-transport", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "talaria-authentication", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "device-registration", State: diagnostic.StatePassed, ObservedAt: observedAt},
	}
	if active {
		observations = append(observations, diagnostic.Observation{EdgeID: "active-event-acceptance", State: diagnostic.StatePassed, ObservedAt: observedAt})
	}
	return observations
}

func callbackWebhookObservations(observedAt time.Time) []diagnostic.Observation {
	return []diagnostic.Observation{
		{EdgeID: "argus-reachability", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "argus-authentication", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "registration-present", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "registration-fresh", State: diagnostic.StatePassed, ObservedAt: observedAt},
		{EdgeID: "registration-conformant", State: diagnostic.StatePassed, ObservedAt: observedAt},
	}
}

func TestNamedStatusReportsGenericHealthNotConfigured(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	snapshot := []byte("apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge\nspec:\n  services:\n    - name: client\n      type: generic-container\n      replicas: 1\n")
	if err := store.SaveDesiredSnapshot("edge", snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	response, err := executeLocal(Options{Command: "status", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(response.Message, "health client/0: not-configured") {
		t.Fatalf("missing not-configured generic health state: %s", response.Message)
	}
}

func TestNamedStatusHealthStateMatrix(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	endpoint, err := store.ReserveHealthEndpoint("edge", "bng", 0)
	if err != nil {
		t.Fatalf("reserve served endpoint: %v", err)
	}
	if _, err := store.ReserveHealthEndpoint("edge", "webpa", 0); err != nil {
		t.Fatalf("reserve unreachable endpoint: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	mode := "healthy"
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(endpoint.HostPort))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if mode == "malformed" {
			_, _ = writer.Write([]byte("not json"))
			return
		}
		response := health.Response{SchemaVersion: health.SchemaVersion, Status: health.Status(mode), ObservedAt: time.Now().UTC(), Checks: []health.Check{{Name: "service", Status: health.Status(mode)}}}
		if mode == "unsupported" {
			response.SchemaVersion = "vcpe.dev/health/v2"
			response.Status = health.StatusHealthy
		}
		_ = json.NewEncoder(writer).Encode(response)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })

	for _, testCase := range []struct {
		mode  string
		state string
	}{
		{mode: "starting", state: "starting"},
		{mode: "unhealthy", state: "unhealthy"},
		{mode: "malformed", state: "unknown"},
		{mode: "unsupported", state: "unknown"},
	} {
		t.Run(testCase.mode, func(t *testing.T) {
			mode = testCase.mode
			response, err := executeLocal(Options{Command: "status", Name: "edge", StateRoot: stateRoot})
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if !strings.Contains(response.Message, "health bng/0: "+testCase.state) || !strings.Contains(response.Message, "health webpa/0: unknown") {
				t.Fatalf("unexpected status output: %s", response.Message)
			}
		})
	}
}

func TestLogsOutputModes(t *testing.T) {
	stateRoot := t.TempDir()

	human, err := executeLocal(Options{Command: "logs", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(human.Message, "logs unavailable without --name") {
		t.Fatalf("expected usage hint, got %q", human.Message)
	}

	jsonOut, err := executeLocal(Options{Command: "logs", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("logs json: %v", err)
	}
	if !strings.Contains(jsonOut.Message, "\"timeline\"") || !strings.Contains(jsonOut.Message, "\"runtimeInitDiagnostics\"") {
		t.Fatalf("expected timeline + diagnostics, got %q", jsonOut.Message)
	}
}

func TestConfigShow(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "config", CommandArgs: []string{"show"}, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	if !strings.Contains(resp.Message, "VCPE_STATE_ROOT=") {
		t.Fatalf("expected state root in config show, got %q", resp.Message)
	}
}

func TestStateResetReinitializes(t *testing.T) {
	stateRoot := t.TempDir()

	// Seed a lease, then reset, then confirm it is cleared.
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"}}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	ps.Close()

	resp, err := executeLocal(Options{Command: "state", CommandArgs: []string{"reset"}, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("state reset: %v", err)
	}
	if !strings.Contains(resp.Message, "state reset complete") {
		t.Fatalf("expected reset confirmation, got %q", resp.Message)
	}

	ps2, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("reopen persist: %v", err)
	}
	defer ps2.Close()
	leases, err := ps2.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected reset to clear leases, got %#v", leases)
	}
}

func TestDownUnknownNameFails(t *testing.T) {
	stateRoot := t.TempDir()
	_, err := executeLocal(Options{Command: "down", Name: "ghost", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "unknown deployment") {
		t.Fatalf("expected unknown deployment error, got %v", err)
	}
}

func TestDownNoNameNoDeployments(t *testing.T) {
	stateRoot := t.TempDir()
	_, err := executeLocal(Options{Command: "down", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "no active deployments") {
		t.Fatalf("expected no active deployments error, got %v", err)
	}
}

func TestDownNoNameMultipleDeployments(t *testing.T) {
	stateRoot := t.TempDir()
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	// Seed two deployment snapshots so ListKnownDeployments returns two names.
	for _, name := range []string{"alpha", "beta"} {
		if err := ps.ReplaceCustomerLeases(name, []persist.IPAMLease{{CustomerID: name, Role: "wan", CIDR: "10.0.0.0/24"}}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	ps.Close()

	_, err = executeLocal(Options{Command: "down", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "multiple deployments active") {
		t.Fatalf("expected multiple deployments error, got %v", err)
	}
	if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("expected both deployment names in error, got %v", err)
	}
}

func TestPreflightRejectsUnsupportedType(t *testing.T) {
	stateRoot := t.TempDir()
	dir := t.TempDir()
	path := dir + "/m.yaml"
	content := "apiVersion: vcpe.dev/v1\n" +
		"kind: Deployment\n" +
		"metadata: { name: edge }\n" +
		"spec:\n" +
		"  networks:\n" +
		"    - role: wan\n" +
		"      ipv4: { cidr: 10.0.0.0/24 }\n" +
		"  services:\n" +
		"    - name: ghost\n" +
		"      type: not-a-real-type\n" +
		"      replicas: 1\n" +
		"      image: { repository: ghcr.io/x/ghost }\n" +
		"      interfaces:\n" +
		"        - { role: wan }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := executeLocal(Options{Command: "plan", ManifestPath: path, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestClassifyDisruptiveCIDRChange(t *testing.T) {
	stateRoot := t.TempDir()
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	defer ps.Close()
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"}}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}

	manifestPath := writeV1Manifest(t, "edge")
	// Mutate the wan CIDR so it differs from the seeded lease.
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	mutated := strings.Replace(string(data), "10.200.0.0/24", "10.250.0.0/24", 1)
	if err := os.WriteFile(manifestPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	doc, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	disruptive, reasons, err := classifyDisruptive(ps, doc)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !disruptive {
		t.Fatal("expected disruptive classification for CIDR change")
	}
	if len(reasons) == 0 || !strings.Contains(strings.Join(reasons, "\n"), "CIDR changes") {
		t.Fatalf("expected CIDR change reason, got %v", reasons)
	}
}

// TestBuildReportsSummary exercises runBuild end-to-end without a container
// runtime by activating the noopImageBackend via VCPE_SKIP_IMAGE=1.
func TestBuildReportsSummary(t *testing.T) {
	t.Setenv("VCPE_SKIP_IMAGE", "1")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")

	resp, err := executeLocal(Options{Command: "build", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.HasPrefix(resp.Message, "build complete for deployment") {
		t.Fatalf("expected build summary, got %q", resp.Message)
	}
}

// TestPlanShowsNetworksAndServices asserts that plan output contains the
// "networks:" and "services:" counts.
func TestPlanShowsNetworksAndServices(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")

	resp, err := executeLocal(Options{Command: "plan", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(resp.Message, "networks:") {
		t.Fatalf("expected networks in plan output, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "services:") {
		t.Fatalf("expected services in plan output, got %q", resp.Message)
	}
}

// TestPlanDisruptiveGate seeds a WAN CIDR lease and uses a manifest with a
// different WAN CIDR, expecting the plan to report "disruptive: yes".
func TestPlanDisruptiveGate(t *testing.T) {
	stateRoot := t.TempDir()
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{
		{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"},
	}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	ps.Close()

	manifestPath := writeV1Manifest(t, "edge")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	// Replace the entire WAN prefix so gateway and pool remain consistent with
	// the new subnet while only the CIDR differs from the seeded lease.
	mutated := strings.ReplaceAll(string(data), "10.200.0.", "10.250.0.")
	if err := os.WriteFile(manifestPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	resp, err := executeLocal(Options{Command: "plan", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(resp.Message, "disruptive: yes") {
		t.Fatalf("expected disruptive: yes in plan output, got %q", resp.Message)
	}
}

// TestDownClearsLeases seeds a lease and a desired snapshot, then calls down
// and asserts that all IPAM leases are cleared.
func TestDownClearsLeases(t *testing.T) {
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	stateRoot := t.TempDir()

	// Seed a lease so down has something to tear down.
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{
		{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"},
	}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	// Stamp a minimal desired snapshot so CustomerExists returns true.
	if err := ps.SaveDesiredSnapshot("edge", []byte("{}")); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if _, err := ps.ReserveHealthEndpoint("edge", "bng", 0); err != nil {
		t.Fatalf("reserve first health endpoint: %v", err)
	}
	if _, err := ps.ReserveHealthEndpoint("edge", "bng", 1); err != nil {
		t.Fatalf("reserve second health endpoint: %v", err)
	}
	ps.Close()

	_, err = executeLocal(Options{Command: "down", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("down: %v", err)
	}

	ps2, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("reopen persist: %v", err)
	}
	defer ps2.Close()
	leases, err := ps2.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected leases cleared after down, got %#v", leases)
	}
	endpoints, err := ps2.ListHealthEndpoints("edge")
	if err != nil {
		t.Fatalf("list health endpoints: %v", err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("expected health endpoints cleared after down, got %#v", endpoints)
	}
}

// TestLogsWithNameShowsDeployment asserts that logs with --name includes the
// deployment name in the output.
func TestLogsWithNameShowsDeployment(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "logs", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(resp.Message, "deployment=edge") {
		t.Fatalf("expected deployment=edge in logs output, got %q", resp.Message)
	}
}

func TestServiceTypesTable(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "service", CommandArgs: []string{"types"}, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("service types: %v", err)
	}
	for _, want := range []string{"NAME", "PULL_POLICY", "DESCRIPTION", "bng", "gateway", "webpa"} {
		if !strings.Contains(resp.Message, want) {
			t.Errorf("expected %q in service types table, got:\n%s", want, resp.Message)
		}
	}
}

func TestServiceTypesJSON(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "service", CommandArgs: []string{"types"}, StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("service types --json: %v", err)
	}
	for _, want := range []string{`"types"`, `"name"`, `"description"`, `"defaultPullPolicy"`, `"defaultImage"`, `"expectedRoles"`, `"bng"`} {
		if !strings.Contains(resp.Message, want) {
			t.Errorf("expected %q in service types JSON, got:\n%s", want, resp.Message)
		}
	}
}

func TestServiceUnknownSubcommandErrors(t *testing.T) {
	stateRoot := t.TempDir()
	_, err := executeLocal(Options{Command: "service", CommandArgs: []string{"frobnicate"}, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "unknown service subcommand") {
		t.Fatalf("expected unknown subcommand error, got %v", err)
	}
}

func TestServiceNoSubcommandErrors(t *testing.T) {
	stateRoot := t.TempDir()
	_, err := executeLocal(Options{Command: "service", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "service requires a subcommand") {
		t.Fatalf("expected missing subcommand error, got %v", err)
	}
}
