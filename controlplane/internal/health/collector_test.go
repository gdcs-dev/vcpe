package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
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
