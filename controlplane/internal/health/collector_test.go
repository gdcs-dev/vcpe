package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestCollectorCollectsAndValidatesResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(validResponse())
	}))
	defer server.Close()
	invalidServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("not json"))
	}))
	defer invalidServer.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	invalidHost, invalidPortText, err := net.SplitHostPort(invalidServer.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split invalid server address: %v", err)
	}
	invalidPort, err := strconv.Atoi(invalidPortText)
	if err != nil {
		t.Fatalf("parse invalid server port: %v", err)
	}

	observations := NewCollector(0, 2).Collect(context.Background(), []Target{
		{Deployment: "edge", Service: "bng", Replica: 0, Host: host, Port: port},
		{Deployment: "edge", Service: "invalid", Replica: 0, Host: invalidHost, Port: invalidPort},
		{Deployment: "edge", Service: "bad", Replica: 0, Host: "127.0.0.1", Port: 1},
	})
	if len(observations) != 3 {
		t.Fatalf("observation count = %d, want 3", len(observations))
	}
	byService := map[string]Observation{}
	for _, observation := range observations {
		byService[observation.Target.Service] = observation
	}
	if observation := byService["bng"]; observation.State != "healthy" || observation.Response == nil {
		t.Fatalf("healthy observation = %#v", observation)
	}
	if observation := byService["bad"]; observation.State != "unknown" || observation.Error == "" {
		t.Fatalf("unreachable observation = %#v", observation)
	}
	if observation := byService["invalid"]; observation.State != "unknown" || observation.Error == "" {
		t.Fatalf("malformed observation = %#v", observation)
	}
}

// TestCollectorTimesOutSlowEndpointWithoutAffectingOthers proves collection
// stays HTTP-only: a bounded timeout isolates one slow, still-listening
// endpoint (never a Podman/exec discovery fallback) and leaves the other
// endpoint's result unaffected.
func TestCollectorTimesOutSlowEndpointWithoutAffectingOthers(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(writer).Encode(validResponse())
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(validResponse())
	}))
	defer fast.Close()
	slowHost, slowPortText, err := net.SplitHostPort(slow.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split slow server address: %v", err)
	}
	slowPort, err := strconv.Atoi(slowPortText)
	if err != nil {
		t.Fatalf("parse slow server port: %v", err)
	}
	fastHost, fastPortText, err := net.SplitHostPort(fast.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split fast server address: %v", err)
	}
	fastPort, err := strconv.Atoi(fastPortText)
	if err != nil {
		t.Fatalf("parse fast server port: %v", err)
	}

	observations := NewCollector(20*time.Millisecond, 2).Collect(context.Background(), []Target{
		{Deployment: "edge", Service: "slow", Replica: 0, Host: slowHost, Port: slowPort},
		{Deployment: "edge", Service: "fast", Replica: 0, Host: fastHost, Port: fastPort},
	})
	byService := map[string]Observation{}
	for _, observation := range observations {
		byService[observation.Target.Service] = observation
	}
	if observation := byService["slow"]; observation.State != "unknown" || observation.Error == "" {
		t.Fatalf("expected the slow endpoint to time out as unknown, got %#v", observation)
	}
	if observation := byService["fast"]; observation.State != "healthy" || observation.Response == nil {
		t.Fatalf("expected the fast endpoint's result unaffected by the other's timeout, got %#v", observation)
	}
}
