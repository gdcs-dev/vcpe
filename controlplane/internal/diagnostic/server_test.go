package diagnostic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticServerDiscoveryAndMethodIsolation(t *testing.T) {
	now := time.Now().UTC()
	activeCalls := 0
	var gotInvocation Invocation
	server := Server{Journeys: map[string]JourneyHandler{
		JourneyCPEWebPA: func(_ context.Context, invocation Invocation) EndpointResponse {
			activeCalls++
			gotInvocation = invocation
			return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: []Observation{{EdgeID: "application-parodus", State: StateUnknown, ObservedAt: now}}}
		},
	}}.Handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte("health"))
	}))

	discovery := httptest.NewRecorder()
	server.ServeHTTP(discovery, httptest.NewRequest(http.MethodGet, "/diagnostics", nil))
	if discovery.Code != http.StatusOK || activeCalls != 0 {
		t.Fatalf("discovery status = %d, active calls = %d", discovery.Code, activeCalls)
	}
	var capabilities Capabilities
	if err := json.Unmarshal(discovery.Body.Bytes(), &capabilities); err != nil || capabilities.Validate() != nil {
		t.Fatalf("capabilities = %+v, decode error = %v", capabilities, err)
	}

	active := httptest.NewRecorder()
	server.ServeHTTP(active, httptest.NewRequest(http.MethodPost, "/diagnostics/cpe-webpa", strings.NewReader(`{"clientService":"config"}`)))
	if active.Code != http.StatusOK || activeCalls != 1 || gotInvocation.ClientService != "config" {
		t.Fatalf("active status = %d, calls = %d, body = %s", active.Code, activeCalls, active.Body.String())
	}

	wrongMethod := httptest.NewRecorder()
	server.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet, "/diagnostics/cpe-webpa", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || activeCalls != 1 {
		t.Fatalf("wrong method status = %d, calls = %d", wrongMethod.Code, activeCalls)
	}

	health := httptest.NewRecorder()
	server.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || health.Body.String() != "health" || activeCalls != 1 {
		t.Fatalf("health status = %d, body = %q, calls = %d", health.Code, health.Body.String(), activeCalls)
	}
}

func TestDiagnosticServerAdvertisesAndDispatchesPassiveRoutes(t *testing.T) {
	passiveCalls := 0
	server := Server{
		Journeys: map[string]JourneyHandler{JourneyCPEWebPA: func(context.Context, Invocation) EndpointResponse {
			return EndpointResponse{}
		}},
		PassiveRoutes: map[string]http.Handler{
			"webhook-subscriber": http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				passiveCalls++
				if request.Method != http.MethodGet || request.URL.Path != "/diagnostics/webhook-subscriber/intent" {
					t.Fatalf("passive request = %s %s", request.Method, request.URL.Path)
				}
				writer.WriteHeader(http.StatusNoContent)
			}),
		},
	}.Handler(nil)

	discovery := httptest.NewRecorder()
	server.ServeHTTP(discovery, httptest.NewRequest(http.MethodGet, "/diagnostics", nil))
	var capabilities Capabilities
	if err := json.Unmarshal(discovery.Body.Bytes(), &capabilities); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(capabilities.Journeys, ","), "cpe-webpa,webhook-subscriber"; got != want {
		t.Fatalf("journeys = %q, want %q", got, want)
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostics/webhook-subscriber/intent", nil))
	if response.Code != http.StatusNoContent || passiveCalls != 1 {
		t.Fatalf("passive response = %d, calls = %d", response.Code, passiveCalls)
	}
}

func TestDiagnosticServerPrefersExactActiveJourneyOverPassiveRoute(t *testing.T) {
	now := time.Now().UTC()
	activeCalls := 0
	passiveCalls := 0
	server := Server{
		Journeys: map[string]JourneyHandler{
			JourneyCPEWebPACallback: func(context.Context, Invocation) EndpointResponse {
				activeCalls++
				return EndpointResponse{
					SchemaVersion: SchemaVersion,
					Journey:       JourneyCPEWebPACallback,
					ObservedAt:    now,
					Observations:  []Observation{{EdgeID: "active-event-acceptance", State: StatePassed, ObservedAt: now}},
					ActiveEvent:   &CPEActiveEventResult{Accepted: true},
				}
			},
		},
		PassiveRoutes: map[string]http.Handler{
			JourneyCPEWebPACallback: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passiveCalls++ }),
		},
	}.Handler(nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/diagnostics/"+JourneyCPEWebPACallback, strings.NewReader(`{"clientService":"apparmor-simulator","subscriber":"event-sink","allowActiveEvent":true,"event":"apparmor/diagnostic","deviceId":"mac:001122334455","correlationId":"`+strings.Repeat("a", MaxCorrelationIDLength)+`"}`))
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || activeCalls != 1 || passiveCalls != 0 {
		t.Fatalf("response = %d %q; active/passive calls = %d/%d", response.Code, response.Body.String(), activeCalls, passiveCalls)
	}
}

func TestDiagnosticServerRejectsInvalidInvocationAndResponse(t *testing.T) {
	server := Server{Journeys: map[string]JourneyHandler{
		JourneyCPEWebPA: func(context.Context, Invocation) EndpointResponse { return EndpointResponse{} },
	}}.Handler(nil)

	for _, body := range []string{"", `{}`, `{"target":"arbitrary"}`, `{"clientService":"../config"}`, `{"clientService":"config"} {}`} {
		invalidRequest := httptest.NewRecorder()
		server.ServeHTTP(invalidRequest, httptest.NewRequest(http.MethodPost, "/diagnostics/cpe-webpa", strings.NewReader(body)))
		if invalidRequest.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d", body, invalidRequest.Code)
		}
	}

	invalid := httptest.NewRecorder()
	server.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/diagnostics/cpe-webpa", strings.NewReader(`{"clientService":"config"}`)))
	if invalid.Code != http.StatusInternalServerError {
		t.Fatalf("invalid response status = %d", invalid.Code)
	}
}
