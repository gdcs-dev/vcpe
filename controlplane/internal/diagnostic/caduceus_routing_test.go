package diagnostic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCaduceusRoutingProbeReturnsOnlySelectedObservation(t *testing.T) {
	correlationID := strings.Repeat("a", MaxCorrelationIDLength)
	observedAt := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != caduceusRoutingPath+correlationID {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Basic local-only" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode(caduceusRoutingResponse{CorrelationID: correlationID, State: "selected", ObservedAt: observedAt})
	}))
	defer server.Close()

	probe := CaduceusRoutingProbe{Endpoint: mustCaduceusRoutingEndpoint(t, server.URL), Auth: "Basic local-only", HTTPClient: server.Client()}
	observation, found, err := probe.Lookup(context.Background(), correlationID)
	if err != nil || !found {
		t.Fatalf("Lookup() = %+v, %t, %v", observation, found, err)
	}
	if observation.SchemaVersion != SchemaVersion || observation.CorrelationID != correlationID || observation.State != "selected" || !observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("observation = %+v", observation)
	}
}

func TestCaduceusRoutingHandlerRejectsUnsafeInput(t *testing.T) {
	probe := CaduceusRoutingProbe{Endpoint: mustCaduceusRoutingEndpoint(t, "http://127.0.0.1:6000"), Auth: "Basic local-only", HTTPClient: http.DefaultClient}
	handler := probe.Handler()
	for _, testCase := range []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "wrong method", method: http.MethodGet, path: "/diagnostics/cpe-webpa-callback/routing", status: http.StatusMethodNotAllowed},
		{name: "wrong path", method: http.MethodPost, path: "/diagnostics/cpe-webpa-callback/other", body: `{}`, status: http.StatusNotFound},
		{name: "missing correlation", method: http.MethodPost, path: "/diagnostics/cpe-webpa-callback/routing", body: `{}`, status: http.StatusBadRequest},
		{name: "extra field", method: http.MethodPost, path: "/diagnostics/cpe-webpa-callback/routing", body: `{"correlationId":"` + strings.Repeat("a", MaxCorrelationIDLength) + `","url":"http://example.test"}`, status: http.StatusBadRequest},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body)))
			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d", response.Code, testCase.status)
			}
		})
	}
}

func TestCaduceusRoutingProbeTreatsAbsentObservationAsNotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	probe := CaduceusRoutingProbe{Endpoint: mustCaduceusRoutingEndpoint(t, server.URL), Auth: "Basic local-only", HTTPClient: server.Client()}
	observation, found, err := probe.Lookup(context.Background(), strings.Repeat("a", MaxCorrelationIDLength))
	if err != nil || found || observation != (RoutingObservation{}) {
		t.Fatalf("Lookup() = %+v, %t, %v", observation, found, err)
	}
}

func mustCaduceusRoutingEndpoint(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	endpoint, err := caduceusRoutingEndpoint(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}
