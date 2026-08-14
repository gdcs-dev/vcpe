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
