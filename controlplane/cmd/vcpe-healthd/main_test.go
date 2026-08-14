package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	if err := serve(context.Background(), "127.0.0.1:0", probe, "exit 0", nil); err != nil {
		t.Fatalf("serve() error = %v", err)
	}
}

func TestBuildDiagnosticJourneys(t *testing.T) {
	journeys, err := buildDiagnosticJourneys([]string{"cpe-webpa"}, time.Second)
	if err != nil || journeys["cpe-webpa"] == nil {
		t.Fatalf("buildDiagnosticJourneys() = %#v, %v", journeys, err)
	}
	if _, err := buildDiagnosticJourneys([]string{"unknown"}, time.Second); err == nil {
		t.Fatal("expected unsupported journey error")
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
