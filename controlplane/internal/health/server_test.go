package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerReturnsValidatedProbeResponse(t *testing.T) {
	server := httptest.NewServer(Server{Probe: func(context.Context) Response { return validResponse() }}.Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
}
