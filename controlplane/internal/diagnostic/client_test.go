package diagnostic

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientDiscoveryAndRun(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics":
			_ = json.NewEncoder(writer).Encode(Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA}})
		case "/diagnostics/cpe-webpa":
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("active request = %s, content length %d", request.Method, request.ContentLength)
			}
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.ClientService != "config" {
				t.Errorf("invocation = %+v, error = %v", invocation, err)
			}
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: []Observation{{EdgeID: "application-parodus", State: StateUnknown, ObservedAt: now}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	target := testTarget(t, server.URL)
	client := NewClient(time.Second)
	capabilities, err := client.Discover(context.Background(), target)
	if err != nil || len(capabilities.Journeys) != 1 {
		t.Fatalf("Discover = %+v, %v", capabilities, err)
	}
	response, err := client.Run(context.Background(), target, JourneyCPEWebPA, Invocation{ClientService: "config"})
	if err != nil || response.Journey != JourneyCPEWebPA {
		t.Fatalf("Run = %+v, %v", response, err)
	}
}

func TestClientRejectsInvalidClientService(t *testing.T) {
	for _, clientService := range []string{"", "../config"} {
		_, err := NewClient(time.Second).Run(context.Background(), Target{Host: "127.0.0.1", Port: 47000}, JourneyCPEWebPA, Invocation{ClientService: clientService})
		if err == nil || !strings.Contains(err.Error(), "stable identifier") {
			t.Fatalf("Run(%q) error = %v", clientService, err)
		}
	}
}

func TestClientRejectsInvalidAndOversizedResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown field", body: `{"schemaVersion":"vcpe.dev/diagnostics/v1","journeys":[],"extra":true}`, want: "unknown field"},
		{name: "oversized", body: `{"schemaVersion":"vcpe.dev/diagnostics/v1","journeys":[],"padding":"` + strings.Repeat("x", MaxCapabilitiesBodySize) + `"}`, want: "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _, _ = writer.Write([]byte(test.body)) }))
			defer server.Close()
			_, err := NewClient(time.Second).Discover(context.Background(), testTarget(t, server.URL))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Discover error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestClientRequiresPersistedLoopbackTarget(t *testing.T) {
	_, err := NewClient(time.Second).Discover(context.Background(), Target{Host: "0.0.0.0", Port: 47000})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("Discover error = %v", err)
	}
}

func testTarget(t *testing.T, rawURL string) Target {
	t.Helper()
	hostPort := strings.TrimPrefix(rawURL, "http://")
	host, rawPort, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return Target{Host: host, Port: port}
}
