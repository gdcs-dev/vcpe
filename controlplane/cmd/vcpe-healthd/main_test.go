package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/gdcs-dev/vcpe/controlplane/internal/health"
)

func TestCheckEndpoint(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "healthy response",
			response: `{"schemaVersion":"vcpe.dev/health/v1","status":"healthy","observedAt":"2026-01-01T00:00:00Z","checks":[{"name":"ready","status":"healthy"}]}`,
		},
		{
			name:     "starting response",
			response: `{"schemaVersion":"vcpe.dev/health/v1","status":"starting","observedAt":"2026-01-01T00:00:00Z","checks":[{"name":"ready","status":"starting"}]}`,
			wantErr:  true,
		},
		{
			name:     "malformed response",
			response: `{`,
			wantErr:  true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write([]byte(testCase.response))
			}))
			defer server.Close()

			err := checkEndpoint(context.Background(), server.URL)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("checkEndpoint() error = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}

func TestServeStopsWhenWorkloadExits(t *testing.T) {
	t.Parallel()

	probe := func(context.Context) health.Response {
		return health.Response{SchemaVersion: health.SchemaVersion, Status: health.StatusHealthy, ObservedAt: time.Now().UTC()}
	}
	if err := serve(context.Background(), "127.0.0.1:0", probe, "exit 0", nil, nil); err != nil {
		t.Fatalf("serve() error = %v", err)
	}
}

func TestSubscriberDiagnosticProxyAllowsOnlyBoundedLoopbackRoutes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/diagnostics/webhook-subscriber/intent" {
			t.Fatalf("upstream request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"journey":"webhook-subscriber"}`))
	}))
	defer upstream.Close()
	routes, err := buildPassiveDiagnosticRoutes(upstream.URL, time.Second, nil)
	if err != nil {
		t.Fatalf("buildPassiveDiagnosticRoutes: %v", err)
	}
	handler := routes[diagnostic.JourneyWebhookSubscriber]

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostics/webhook-subscriber/intent", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"journey":"webhook-subscriber"}` {
		t.Fatalf("proxy response = %d %q", response.Code, response.Body.String())
	}

	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodPost, "/diagnostics/webhook-subscriber/intent", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d", wrongMethod.Code)
	}

	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/diagnostics/webhook-subscriber/unknown", nil))
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("blocked route status = %d", blocked.Code)
	}
	if _, err := buildPassiveDiagnosticRoutes("http://event-sink:8080", time.Second, nil); err == nil {
		t.Fatal("expected non-loopback subscriber endpoint rejection")
	}
}

func TestBuildPassiveDiagnosticRoutesAddsCaduceusRoutingHandler(t *testing.T) {
	t.Setenv("VCPE_CADUCEUS_URL", "http://127.0.0.1:6000/api/v4/notify")
	routes, err := buildPassiveDiagnosticRoutes("", time.Second, []string{diagnostic.JourneyCPEWebPACallback})
	if err != nil {
		t.Fatalf("buildPassiveDiagnosticRoutes: %v", err)
	}
	if routes[diagnostic.JourneyCPEWebPACallback] == nil {
		t.Fatal("callback routing route is missing")
	}
	server := diagnostic.Server{PassiveRoutes: routes}.Handler(nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostics", nil))
	var capabilities diagnostic.Capabilities
	if err := json.NewDecoder(response.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if len(capabilities.Journeys) != 1 || capabilities.Journeys[0] != diagnostic.JourneyCPEWebPACallback {
		t.Fatalf("capabilities = %+v", capabilities)
	}

	oversized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/diagnostics/cpe-webpa-callback/routing", strings.NewReader(strings.Repeat("x", diagnostic.MaxInvocationBodySize+1)))
	server.ServeHTTP(oversized, request)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized routing status = %d", oversized.Code)
	}
}

func TestBuildDiagnosticJourneys(t *testing.T) {
	journeys, err := buildDiagnosticJourneys([]string{"cpe-webpa", "cpe-webpa-callback", "parodus-clients", "argus-webhooks", "talaria-devices", "webhook"}, time.Second)
	if err != nil || journeys["cpe-webpa"] == nil || journeys["cpe-webpa-callback"] == nil || journeys["parodus-clients"] == nil || journeys["argus-webhooks"] == nil || journeys["talaria-devices"] == nil || journeys["webhook"] == nil {
		t.Fatalf("buildDiagnosticJourneys() = %#v, %v", journeys, err)
	}
	if _, err := buildDiagnosticJourneys([]string{"unknown"}, time.Second); err == nil {
		t.Fatal("expected unsupported journey error")
	}
}

func TestCPECallbackJourneyAdvertisesThroughHealthServer(t *testing.T) {
	journeys, err := buildDiagnosticJourneys([]string{diagnostic.JourneyCPEWebPA, diagnostic.JourneyCPEWebPACallback, diagnostic.JourneyParodusClients}, time.Second)
	if err != nil {
		t.Fatalf("buildDiagnosticJourneys: %v", err)
	}
	server := diagnostic.Server{Journeys: journeys}.Handler(nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostics", nil))
	var capabilities diagnostic.Capabilities
	if err := json.NewDecoder(response.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{diagnostic.JourneyCPEWebPA, diagnostic.JourneyCPEWebPACallback, diagnostic.JourneyParodusClients}, ",")
	if response.Code != http.StatusOK || strings.Join(capabilities.Journeys, ",") != want {
		t.Fatalf("status = %d, capabilities = %+v", response.Code, capabilities)
	}
}

func TestParodusClientsJourneyRejectsInvocationFields(t *testing.T) {
	journeys, err := buildDiagnosticJourneys([]string{diagnostic.JourneyParodusClients}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response := journeys[diagnostic.JourneyParodusClients](context.Background(), diagnostic.Invocation{ClientService: "apparmor-simulator"})
	if response.Journey != diagnostic.JourneyParodusClients || len(response.Observations) != 1 || response.Observations[0].ReasonID != "parodus-client-list-invalid" {
		t.Fatalf("response = %+v", response)
	}
}

func TestNamedCommandProbeReportsEachCheck(t *testing.T) {
	t.Parallel()

	probe, err := namedCommandProbe([]string{"process=true", "dependency=false"}, time.Second)
	if err != nil {
		t.Fatalf("namedCommandProbe() error = %v", err)
	}
	response := probe(context.Background())
	if response.Status != health.StatusUnhealthy {
		t.Fatalf("response status = %q, want unhealthy", response.Status)
	}
	if len(response.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(response.Checks))
	}
	if response.Checks[0].Name != "process" || response.Checks[0].Status != health.StatusHealthy {
		t.Fatalf("first check = %#v, want healthy process", response.Checks[0])
	}
	if response.Checks[1].Name != "dependency" || response.Checks[1].Status != health.StatusUnhealthy {
		t.Fatalf("second check = %#v, want unhealthy dependency", response.Checks[1])
	}
}

func TestCheckEndpointRejectsNonOKResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(writer).Encode(health.Response{})
	}))
	defer server.Close()

	if err := checkEndpoint(context.Background(), server.URL); err == nil {
		t.Fatal("checkEndpoint() error = nil, want non-OK response error")
	}
}
