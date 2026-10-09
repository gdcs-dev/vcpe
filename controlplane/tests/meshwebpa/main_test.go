package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/xmidt-org/wrp-go/v3"
)

func TestBothManaged(t *testing.T) {
	devices := []string{"mac:001122334455", "mac:001122334466"}
	seen := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/v2/devices":
			if request.Method != http.MethodGet {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"},{"id":"mac:001122334466"}]}`))
		case "/api/v3/device":
			if request.Method != http.MethodPost {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var message wrp.Message
			if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
				t.Errorf("decode request: %v", err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, device := range devices {
				if message.Type == wrp.RetrieveMessageType && message.Source == "dns:webpa" && message.Destination == device+"/parodus/client-list" && message.TransactionUUID != "" {
					seen[device] = true
				}
			}
			message.Source, message.Destination = message.Destination, message.Source
			message.Payload = []byte(`{"client-list":["config"],"truncated":false}`)
			writer.Header().Set("Content-Type", "application/msgpack")
			if err := wrp.NewEncoder(writer, wrp.Msgpack).Encode(&message); err != nil {
				t.Errorf("encode response: %v", err)
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", devices); err != nil {
		t.Fatal(err)
	}
	if !seen[devices[0]] || !seen[devices[1]] {
		t.Fatalf("independent management requests not sent: %+v", seen)
	}
}

func TestPartialRegistrationFailsVerification(t *testing.T) {
	checks := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		checks++
		_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	err := verifyDevices(ctx, server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{"mac:001122334455", "mac:001122334466"})
	if err == nil || !strings.Contains(err.Error(), "mac:001122334466") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("partial registration result = %v", err)
	}
	if checks < 2 {
		t.Fatalf("partial registration was not checked through the bounded window: %d inventory checks", checks)
	}
}

func TestInventoryTimeoutReportsOnlyTheFailedStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-time.After(150 * time.Millisecond)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := verifyDevices(ctx, server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{"mac:001122334455", "mac:001122334466"})
	if err == nil || !strings.Contains(err.Error(), "inventory timeout") || strings.Contains(err.Error(), server.URL) || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("inventory timeout result = %v", err)
	}
}

func TestInventoryConnectionFailureReportsOnlyTheFailedStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {}))
	server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{"mac:001122334455", "mac:001122334466"})
	if err == nil || !strings.Contains(err.Error(), "inventory request failed") || strings.Contains(err.Error(), server.URL) || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("inventory connection result = %v", err)
	}
}

func TestContainerTransportKeepsCredentialsOutOfArguments(t *testing.T) {
	directory := t.TempDir()
	podman := filepath.Join(directory, "podman")
	if err := os.WriteFile(podman, []byte("#!/bin/sh\nprintf '%s' \"$*\" > \"$PODMAN_ARGS\"\nshift 3\nexec \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	arguments := filepath.Join(directory, "arguments")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PODMAN_ARGS", arguments)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" || request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || string(body) != `{"message":"client-list"}` {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("reply"))
	}))
	defer server.Close()
	request, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"message":"client-list"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	response, err := containerClient("mesh-roaming-webpa-1").Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(result) != "reply" {
		t.Fatalf("container response = %d %q", response.StatusCode, result)
	}
	commandLine, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commandLine), "dXNlcjpwYXNz") || !strings.Contains(string(commandLine), "exec -i mesh-roaming-webpa-1 curl --config -") {
		t.Fatalf("unexpected container command: %q", commandLine)
	}
}

func TestDistinctTalariaAndScytaleEndpoints(t *testing.T) {
	deviceID := "mac:001122334455"
	talaria := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/devices" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
	}))
	defer talaria.Close()
	scytale := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v3/device" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		var message wrp.Message
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		message.Source, message.Destination = message.Destination, message.Source
		message.Payload = []byte(`{"client-list":[],"truncated":false}`)
		if err := wrp.NewEncoder(writer, wrp.Msgpack).Encode(&message); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer scytale.Close()
	if err := verifyDevices(context.Background(), talaria.Client(), talaria.URL, scytale.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID}); err != nil {
		t.Fatal(err)
	}
}

func TestLiveVerifierUsesPlannedWANIdentities(t *testing.T) {
	root := "mac:" + strings.ReplaceAll(plan.CanonicalMAC("mesh-roaming", "root", "wan", 0), ":", "")
	remote := "mac:" + strings.ReplaceAll(plan.CanonicalMAC("mesh-roaming", "remote", "wan", 0), ":", "")
	seen := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		if request.URL.Path == "/api/v2/devices" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"devices": []map[string]string{{"id": root}, {"id": remote}}})
			return
		}
		if request.URL.Path != "/api/v3/device" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		var message wrp.Message
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		for _, device := range []string{root, remote} {
			if message.Destination == device+"/parodus/client-list" {
				seen[device] = true
			}
		}
		message.Source, message.Destination = message.Destination, message.Source
		message.Payload = []byte(`{"client-list":[],"truncated":false}`)
		if err := wrp.NewEncoder(writer, wrp.Msgpack).Encode(&message); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "podman"), []byte(`#!/bin/sh
case "$*" in
    *"cat /etc/talaria/talaria.yaml"*) printf 'inbound:\n  authKey: dXNlcjpwYXNz\n' ;;
    *"cat /etc/scytale/scytale.yaml"*) printf 'authToken: dXNlcjpwYXNz\n' ;;
    *) sed "s@http://127.0.0.1:6200@$TEST_WEBPA_URL@g; s@http://127.0.0.1:6300@$TEST_WEBPA_URL@g" | curl --config - ;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_WEBPA_URL", server.URL)
	manifestPath := filepath.Join("..", "..", "..", "manifests", "dev", "mesh-roaming.yaml")
	if err := runLive(context.Background(), manifestPath); err != nil {
		t.Fatal(err)
	}
	if !seen[root] || !seen[remote] {
		t.Fatalf("planned Gateways not both managed: %+v", seen)
	}
}

func TestVerifierCommandRequiresManifest(t *testing.T) {
	command := exec.Command("go", "run", ".")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "usage: meshwebpa <manifest>") {
		t.Fatalf("missing manifest result = %v %q", err, output)
	}
}

func TestManagementAuthenticationRejectionNamesDeviceWithoutSecrets(t *testing.T) {
	deviceID := "mac:001122334455"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
			return
		}
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte("credential=dXNlcjpwYXNz"))
	}))
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "authentication rejected") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("authentication failure = %v", err)
	}
}

func TestMissingManagementResponseTimesOutWithoutLeakingRequest(t *testing.T) {
	deviceID := "mac:001122334455"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
			return
		}
		<-time.After(150 * time.Millisecond)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := verifyDevices(ctx, server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "management timeout") ||
		strings.Contains(err.Error(), server.URL) || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("missing response result = %v", err)
	}
}

func TestOversizedInventoryFailsBeforeManagementRequest(t *testing.T) {
	deviceID := "mac:001122334455"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
			_, _ = writer.Write([]byte(strings.Repeat(" ", maxResponseBytes)))
			return
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), "inventory exceeds limit") || strings.Contains(err.Error(), "management") {
		t.Fatalf("oversized inventory = %v", err)
	}
}

func TestMalformedMessagePackFailsManagement(t *testing.T) {
	deviceID := "mac:001122334455"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
			return
		}
		_, _ = writer.Write([]byte{0xc1})
	}))
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "invalid management response") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("malformed MessagePack result = %v", err)
	}
}

func TestWrongWRPTypeFailsManagementCorrelation(t *testing.T) {
	deviceID := "mac:001122334455"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"}]}`))
			return
		}
		var message wrp.Message
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		message.Source, message.Destination = message.Destination, message.Source
		message.Type = 0
		message.Payload = []byte(`{"client-list":[],"truncated":false}`)
		if err := wrp.NewEncoder(writer, wrp.Msgpack).Encode(&message); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "mismatched management response") {
		t.Fatalf("wrong response type = %v", err)
	}
}

func TestWrongWRPSourceFailsManagementCorrelation(t *testing.T) {
	deviceID := "mac:001122334455"
	server := newCorrelatedReplyServer(t, deviceID, func(message *wrp.Message) {
		message.Source = "mac:001122334466/parodus/client-list"
	})
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "mismatched management response") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("wrong response source = %v", err)
	}
}

func TestWrongWRPDestinationFailsManagementCorrelation(t *testing.T) {
	deviceID := "mac:001122334455"
	server := newCorrelatedReplyServer(t, deviceID, func(message *wrp.Message) {
		message.Destination = "dns:another-client"
	})
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "mismatched management response") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("wrong response destination = %v", err)
	}
}

func TestWrongWRPTransactionFailsManagementCorrelation(t *testing.T) {
	deviceID := "mac:001122334455"
	server := newCorrelatedReplyServer(t, deviceID, func(message *wrp.Message) {
		message.TransactionUUID = "00000000-0000-0000-0000-000000000000"
	})
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "mismatched management response") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("wrong response transaction = %v", err)
	}
}

func TestMalformedClientListFailsManagement(t *testing.T) {
	deviceID := "mac:001122334455"
	server := newCorrelatedReplyServer(t, deviceID, func(message *wrp.Message) {
		message.Payload = []byte(`{"client-list":[],"truncated":"not-a-boolean"}`)
	})
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "invalid client list") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("malformed client-list result = %v", err)
	}
}

func TestMalformedClientNameFailsManagement(t *testing.T) {
	deviceID := "mac:001122334455"
	server := newCorrelatedReplyServer(t, deviceID, func(message *wrp.Message) {
		message.Payload = []byte(`{"client-list":["!!!"],"truncated":false}`)
	})
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
	if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "invalid client list") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("malformed client name result = %v", err)
	}
}

func TestInvalidClientListBoundsAndOrdering(t *testing.T) {
	for _, test := range []struct {
		name    string
		clients []string
	}{
		{name: "too many clients", clients: make([]string, 65)},
		{name: "long client name", clients: []string{strings.Repeat("a", 65)}},
		{name: "unsorted clients", clients: []string{"config", "apparmor-simulator"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "too many clients" {
				for index := range test.clients {
					test.clients[index] = "client"
				}
			}
			deviceID := "mac:001122334455"
			server := newCorrelatedReplyServer(t, deviceID, func(message *wrp.Message) {
				message.Payload, _ = json.Marshal(map[string]any{"client-list": test.clients, "truncated": false})
			})
			defer server.Close()
			err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", []string{deviceID})
			if err == nil || !strings.Contains(err.Error(), deviceID) || !strings.Contains(err.Error(), "invalid client list") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
				t.Fatalf("invalid client list result = %v", err)
			}
		})
	}
}

func TestOnlyOneManagementResponseFailsVerification(t *testing.T) {
	devices := []string{"mac:001122334455", "mac:001122334466"}
	rootResponded := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_, _ = writer.Write([]byte(`{"devices":[{"id":"mac:001122334455"},{"id":"mac:001122334466"}]}`))
			return
		}
		var message wrp.Message
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if message.Destination == devices[1]+"/parodus/client-list" {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("credential=dXNlcjpwYXNz"))
			return
		}
		rootResponded = true
		message.Source, message.Destination = message.Destination, message.Source
		message.Payload = []byte(`{"client-list":[],"truncated":false}`)
		if err := wrp.NewEncoder(writer, wrp.Msgpack).Encode(&message); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()
	err := verifyDevices(context.Background(), server.Client(), server.URL, server.URL, "dXNlcjpwYXNz", "dXNlcjpwYXNz", devices)
	if !rootResponded || err == nil || !strings.Contains(err.Error(), devices[1]) || !strings.Contains(err.Error(), "management returned HTTP 503") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("partial management result = %v; root responded = %t", err, rootResponded)
	}
}

func newCorrelatedReplyServer(t *testing.T, deviceID string, alterReply func(*wrp.Message)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/devices" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"devices": []map[string]string{{"id": deviceID}}})
			return
		}
		var message wrp.Message
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		message.Source, message.Destination = message.Destination, message.Source
		message.Payload = []byte(`{"client-list":[],"truncated":false}`)
		alterReply(&message)
		if err := wrp.NewEncoder(writer, wrp.Msgpack).Encode(&message); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
}
