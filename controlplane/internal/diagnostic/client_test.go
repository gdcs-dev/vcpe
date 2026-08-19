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
	for _, clientService := range []string{"", "../config", "Hermes FS"} {
		_, err := NewClient(time.Second).Run(context.Background(), Target{Host: "127.0.0.1", Port: 47000}, JourneyCPEWebPA, Invocation{ClientService: clientService})
		if err == nil || !strings.Contains(err.Error(), "stable identifier") {
			t.Fatalf("Run(%q) error = %v", clientService, err)
		}
	}
}

func TestClientPreservesCaseSensitiveClientService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var invocation Invocation
		if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.ClientService != "HermesFS" {
			t.Fatalf("invocation = %+v, error = %v", invocation, err)
		}
		_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: time.Now().UTC(), Observations: []Observation{{EdgeID: "application-parodus", State: StateUnknown, ObservedAt: time.Now().UTC()}}})
	}))
	defer server.Close()
	if _, err := NewClient(time.Second).Run(context.Background(), testTarget(t, server.URL), JourneyCPEWebPA, Invocation{ClientService: "HermesFS"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestClientRunsCPECallbackAndObservesWebPARouting(t *testing.T) {
	correlationID := strings.Repeat("a", MaxCorrelationIDLength)
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics/cpe-webpa-callback":
			if request.Method != http.MethodPost {
				t.Fatalf("active method = %s", request.Method)
			}
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.CorrelationID != correlationID || !invocation.AllowActiveEvent || invocation.ClientService != "apparmor-simulator" || invocation.Subscriber != "event-sink" {
				t.Fatalf("active invocation = %+v, %v", invocation, err)
			}
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPACallback, ObservedAt: now, Observations: []Observation{{EdgeID: "active-event-acceptance", State: StatePassed, ObservedAt: now}}, ActiveEvent: &CPEActiveEventResult{Accepted: true}})
		case "/diagnostics/cpe-webpa-callback/routing":
			if request.Method != http.MethodPost {
				t.Fatalf("routing method = %s", request.Method)
			}
			var input map[string]string
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil || len(input) != 1 || input["correlationId"] != correlationID {
				t.Fatalf("routing input = %#v, %v", input, err)
			}
			_ = json.NewEncoder(writer).Encode(RoutingObservation{SchemaVersion: SchemaVersion, CorrelationID: correlationID, State: "selected", ObservedAt: now})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	target := testTarget(t, server.URL)
	client := NewClient(time.Second)
	invocation := Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: correlationID}
	active, err := client.RunCPECallback(context.Background(), target, invocation)
	if err != nil || active.ActiveEvent == nil || !active.ActiveEvent.Accepted {
		t.Fatalf("RunCPECallback() = %+v, %v", active, err)
	}
	routing, found, err := client.ObserveRouting(context.Background(), target, correlationID)
	if err != nil || !found || routing.State != "selected" || !routing.ObservedAt.Equal(now) {
		t.Fatalf("ObserveRouting() = %+v, %t, %v", routing, found, err)
	}
}

func TestClientPreservesCPECallbackSourceFailure(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(EndpointResponse{
			SchemaVersion: SchemaVersion,
			Journey:       JourneyCPEWebPACallback,
			ObservedAt:    now,
			Observations: []Observation{{
				EdgeID:        "active-event-acceptance",
				State:         StateFailed,
				ReasonID:      ReasonActiveEventRejected,
				RemediationID: RemediationCheckActiveEvent,
				Message:       "CPE did not accept the marked diagnostic event",
				ObservedAt:    now,
			}},
		})
	}))
	defer server.Close()
	response, err := NewClient(time.Second).RunCPECallback(context.Background(), testTarget(t, server.URL), Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", MaxCorrelationIDLength)})
	if err != nil || len(response.Observations) != 1 || response.Observations[0].State != StateFailed {
		t.Fatalf("RunCPECallback() = %+v, %v", response, err)
	}
}

func TestClientObserveRoutingDistinguishesMissingAndInvalidResponse(t *testing.T) {
	correlationID := strings.Repeat("a", MaxCorrelationIDLength)
	for _, testCase := range []struct {
		name string
		code int
		body string
		want string
	}{
		{name: "missing", code: http.StatusNotFound},
		{name: "unknown field", code: http.StatusOK, body: `{"schemaVersion":"vcpe.dev/diagnostic/v1","correlationId":"` + correlationID + `","state":"selected","observedAt":"2026-08-17T12:00:00Z","secret":"no"}`, want: "unknown field"},
		{name: "wrong state", code: http.StatusOK, body: `{"schemaVersion":"vcpe.dev/diagnostic/v1","correlationId":"` + correlationID + `","state":"not-selected","observedAt":"2026-08-17T12:00:00Z"}`, want: "state"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(testCase.code)
				_, _ = writer.Write([]byte(testCase.body))
			}))
			defer server.Close()
			observation, found, err := NewClient(time.Second).ObserveRouting(context.Background(), testTarget(t, server.URL), correlationID)
			if testCase.name == "missing" {
				if err != nil || found || observation != (RoutingObservation{}) {
					t.Fatalf("ObserveRouting() = %+v, %t, %v", observation, found, err)
				}
				return
			}
			if err == nil || found || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("ObserveRouting() = %+v, %t, %v, want %q", observation, found, err, testCase.want)
			}
		})
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

func TestClientPollReceiptUsesPersistedEndpointAndRetriesMissingState(t *testing.T) {
	const correlationID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/diagnostics/webhook-subscriber/receipts/"+correlationID {
			t.Fatalf("receipt request = %s %s", request.Method, request.URL.Path)
		}
		attempts++
		if attempts < 3 {
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(Receipt{SchemaVersion: SchemaVersion, CorrelationID: correlationID, Source: "direct", AcceptedAt: time.Now().UTC(), HTTPStatus: http.StatusNoContent})
	}))
	defer server.Close()

	client := NewClient(time.Second)
	client.ReceiptPollInterval = time.Millisecond
	receipt, err := client.PollReceipt(context.Background(), testTarget(t, server.URL), correlationID)
	if err != nil || attempts != 3 || receipt.Source != "direct" || receipt.HTTPStatus != http.StatusNoContent {
		t.Fatalf("PollReceipt() = %+v, %v after %d attempts", receipt, err, attempts)
	}
}

func TestClientPollReceiptDistinguishesMissingAndInvalidResponses(t *testing.T) {
	const correlationID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, testCase := range []struct {
		name string
		body string
		code int
		want string
	}{
		{name: "missing", code: http.StatusNotFound, want: ErrReceiptMissing.Error()},
		{name: "unexpected status", code: http.StatusUnauthorized, want: "HTTP 401"},
		{name: "mismatched correlation", code: http.StatusOK, body: `{"schemaVersion":"vcpe.dev/diagnostic/v1","correlationId":"other","source":"direct","acceptedAt":"2026-08-14T12:00:00Z","httpStatus":204}`, want: "does not match"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(testCase.code)
				_, _ = writer.Write([]byte(testCase.body))
			}))
			defer server.Close()
			client := NewClient(time.Second)
			client.ReceiptPollInterval = time.Millisecond
			_, err := client.PollReceipt(context.Background(), testTarget(t, server.URL), correlationID)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("PollReceipt() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestClientRetrievesAndForwardsWebhookSubscriberIntent(t *testing.T) {
	now := time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC)
	intent := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true, InitialSuccessAt: now}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/diagnostics/webhook-subscriber/intent":
			if request.Method != http.MethodGet {
				t.Fatalf("intent method = %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(intent)
		case "/diagnostics/webhook":
			var invocation Invocation
			if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil || invocation.SubscriberIntent == nil || invocation.SubscriberIntent.CallbackURL != intent.CallbackURL || !invocation.AllowActiveCallback {
				t.Fatalf("webhook invocation = %+v, error = %v", invocation, err)
			}
			_ = json.NewEncoder(writer).Encode(EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now}}, Active: &WebhookActiveResult{Phase: WebhookActiveDirect, CorrelationID: strings.Repeat("a", 64), HTTPStatus: http.StatusNoContent}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := NewClient(time.Second)
	gotIntent, err := client.SubscriberIntent(context.Background(), testTarget(t, server.URL))
	if err != nil || gotIntent.CallbackURL != intent.CallbackURL {
		t.Fatalf("SubscriberIntent() = %+v, %v", gotIntent, err)
	}
	response, err := client.RunWebhook(context.Background(), testTarget(t, server.URL), gotIntent, Invocation{AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455", ActivePhase: WebhookActiveDirect})
	if err != nil || response.Journey != JourneyWebhook {
		t.Fatalf("RunWebhook() = %+v, %v", response, err)
	}
}

func TestWebhookSubscriberIntentRejectsUnsafeFields(t *testing.T) {
	valid := WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: time.Now().UTC(), CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json"}
	for _, testCase := range []struct {
		name   string
		mutate func(*WebhookSubscriberIntent)
	}{
		{name: "credential URL", mutate: func(intent *WebhookSubscriberIntent) { intent.CallbackURL = "http://user:password@event-sink/webhook" }},
		{name: "query URL", mutate: func(intent *WebhookSubscriberIntent) { intent.CallbackURL = "http://event-sink/webhook?token=secret" }},
		{name: "wrong schema", mutate: func(intent *WebhookSubscriberIntent) { intent.SchemaVersion = "v2" }},
		{name: "wrong journey", mutate: func(intent *WebhookSubscriberIntent) { intent.Journey = JourneyWebhook }},
		{name: "oversized filter", mutate: func(intent *WebhookSubscriberIntent) {
			intent.EventFilter = strings.Repeat("x", MaxInvocationTextLength+1)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			intent := valid
			testCase.mutate(&intent)
			if err := intent.Validate(); err == nil {
				t.Fatal("unsafe intent was accepted")
			}
		})
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
