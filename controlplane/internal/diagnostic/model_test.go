package diagnostic

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func validResult() Result {
	now := time.Now().UTC()
	return Result{
		SchemaVersion: SchemaVersion,
		Journey:       JourneyCPEWebPA,
		Source:        EndpointIdentity{Deployment: "edge", Service: "gateway", Type: "gateway", Replica: 0},
		Target:        EndpointIdentity{Deployment: "edge", Service: "webpa", Type: "webpa", Replica: 0},
		Nodes: []Node{
			{ID: "app", Label: "CPE application", Kind: "application"},
			{ID: "parodus", Label: "Parodus", Kind: "service"},
			{ID: "talaria", Label: "Talaria", Kind: "service"},
		},
		Edges: []Edge{
			{ID: "app-parodus", From: "app", To: "parodus", Label: "local connection", BlocksFollowing: true},
			{ID: "talaria-dns", From: "parodus", To: "talaria", Label: "DNS resolution", BlocksFollowing: true},
		},
		Observations: []Observation{
			{EdgeID: "app-parodus", State: StatePassed, ObservedAt: now},
			{EdgeID: "talaria-dns", State: StatePassed, ObservedAt: now},
		},
		ObservedAt: now,
	}
}

func validWebhookRegistration(fingerprint string) WebhookRegistration {
	ttlSeconds := int64(3600)
	return WebhookRegistration{
		Fingerprint:    fingerprint,
		CallbackURL:    "http://event-sink/webhook",
		EventFilters:   []string{"devices/.*"},
		DeviceMatchers: []string{"mac:.*"},
		ContentType:    "application/json",
		Until:          time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
		TTLSeconds:     &ttlSeconds,
		SecretPresent:  true,
	}
}

func validWebhookInventoryResult() Result {
	result := validResult()
	result.Journey = JourneyArgusWebhooks
	result.Source = EndpointIdentity{Deployment: "edge", Service: "webpa", Type: "webpa", Replica: 0}
	result.Target = EndpointIdentity{Deployment: "edge", Service: "argus", Type: "argus", Replica: 0}
	result.Nodes = []Node{{ID: "webpa", Label: "WebPA", Kind: "service"}, {ID: "argus", Label: "Argus", Kind: "service"}}
	result.Edges = []Edge{
		{ID: "argus-reachability", From: "webpa", To: "argus", Label: "Argus reachability", BlocksFollowing: true},
		{ID: "argus-inventory", From: "webpa", To: "argus", Label: "list registered webhooks", BlocksFollowing: true},
	}
	result.Observations = []Observation{
		{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: result.ObservedAt},
		{EdgeID: "argus-inventory", State: StatePassed, ObservedAt: result.ObservedAt},
	}
	return result
}

func TestResultValidate(t *testing.T) {
	if err := validResult().Validate(); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
}

func TestResultValidateParodusClientList(t *testing.T) {
	clients := []string{"apparmor-simulator", "config"}
	truncated := false
	result := validResult()
	result.Journey = JourneyParodusClients
	result.Target = EndpointIdentity{Deployment: "edge", Service: "parodus", Type: "parodus", Replica: 0}
	result.Nodes = []Node{
		{ID: "gateway", Label: "Gateway", Kind: "service"},
		{ID: "parodus", Label: "Parodus", Kind: "service"},
	}
	result.Edges = []Edge{{ID: "parodus-client-list", From: "gateway", To: "parodus", Label: "list registered clients", BlocksFollowing: true}}
	result.Observations = []Observation{{EdgeID: "parodus-client-list", State: StatePassed, ObservedAt: result.ObservedAt}}
	result.ParodusClients = &clients
	result.ParodusClientsTruncated = &truncated
	if err := result.Validate(); err != nil {
		t.Fatalf("valid Parodus client list rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Result)
		want   string
	}{
		{name: "missing clients", mutate: func(value *Result) { value.ParodusClients = nil }, want: "present together"},
		{name: "missing truncation", mutate: func(value *Result) { value.ParodusClientsTruncated = nil }, want: "present together"},
		{name: "unsorted", mutate: func(value *Result) {
			values := []string{"config", "apparmor-simulator"}
			value.ParodusClients = &values
		}, want: "sorted"},
		{name: "invalid client", mutate: func(value *Result) { values := []string{"invalid client"}; value.ParodusClients = &values }, want: "stable identifier"},
		{name: "too many clients", mutate: func(value *Result) {
			values := make([]string, MaxParodusClients+1)
			for index := range values {
				values[index] = "client" + string(rune('a'+index))
			}
			value.ParodusClients = &values
		}, want: "maximum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := result
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestResultValidateWebhookRegistrationList(t *testing.T) {
	first := validWebhookRegistration(strings.Repeat("a", 64))
	second := validWebhookRegistration(strings.Repeat("b", 64))
	registrations := []WebhookRegistration{first, second}
	result := validWebhookInventoryResult()
	result.WebhookRegistrations = &registrations
	if err := result.Validate(); err != nil {
		t.Fatalf("valid webhook registrations rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Result)
		want   string
	}{
		{name: "unsorted", mutate: func(value *Result) {
			values := []WebhookRegistration{second, first}
			value.WebhookRegistrations = &values
		}, want: "sorted"},
		{name: "unsafe callback URL", mutate: func(value *Result) {
			value.WebhookRegistrations = &[]WebhookRegistration{{Fingerprint: first.Fingerprint, CallbackURL: "http://event-sink/webhook?token=private", EventFilters: first.EventFilters, DeviceMatchers: first.DeviceMatchers, ContentType: first.ContentType, Until: first.Until}}
		}, want: "normalized safe identity"},
		{name: "unsorted filter", mutate: func(value *Result) {
			value.WebhookRegistrations = &[]WebhookRegistration{{Fingerprint: first.Fingerprint, CallbackURL: first.CallbackURL, EventFilters: []string{"z", "a"}, DeviceMatchers: first.DeviceMatchers, ContentType: first.ContentType, Until: first.Until}}
		}, want: "event filter list must be sorted"},
		{name: "too many registrations", mutate: func(value *Result) {
			values := make([]WebhookRegistration, MaxWebhookRegistrations+1)
			for index := range values {
				values[index] = validWebhookRegistration(fmt.Sprintf("%064x", index))
			}
			value.WebhookRegistrations = &values
		}, want: "maximum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := result
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestResultValidateRejectsBrokenInvariants(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Result)
		wantErr string
	}{
		{name: "schema", mutate: func(result *Result) { result.SchemaVersion = "v2" }, wantErr: "schema version"},
		{name: "journey", mutate: func(result *Result) { result.Journey = "other" }, wantErr: "unsupported diagnostic journey"},
		{name: "duplicate node", mutate: func(result *Result) { result.Nodes[1].ID = result.Nodes[0].ID }, wantErr: "duplicate diagnostic node"},
		{name: "unknown graph reference", mutate: func(result *Result) { result.Edges[0].To = "missing" }, wantErr: "unknown target node"},
		{name: "observation order", mutate: func(result *Result) { result.Observations[0].EdgeID = "talaria-dns" }, wantErr: "expected"},
		{name: "invalid state", mutate: func(result *Result) { result.Observations[0].State = "bad" }, wantErr: "invalid state"},
		{name: "missing timestamp", mutate: func(result *Result) { result.Observations[0].ObservedAt = time.Time{} }, wantErr: "observation time"},
		{name: "first failure mismatch", mutate: func(result *Result) {
			result.Observations[0].State = StateFailed
			result.Observations[1].State = StateSkipped
		}, wantErr: "firstFailure"},
		{name: "downstream not skipped", mutate: func(result *Result) { result.Observations[0].State = StateUnknown }, wantErr: "must be skipped"},
		{name: "too much evidence", mutate: func(result *Result) {
			result.Observations[0].Evidence = make([]Evidence, MaxEvidencePerEdge+1)
		}, wantErr: "evidence entries"},
		{name: "message too long", mutate: func(result *Result) {
			result.Observations[0].Message = strings.Repeat("x", MaxMessageLength+1)
		}, wantErr: "observation message"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validResult()
			test.mutate(&result)
			err := result.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestCapabilitiesValidate(t *testing.T) {
	capabilities := Capabilities{SchemaVersion: CapabilitiesSchema, Journeys: []string{JourneyCPEWebPA}}
	if err := capabilities.Validate(); err != nil {
		t.Fatalf("valid capabilities rejected: %v", err)
	}
	capabilities.Journeys = []string{JourneyCPEWebPA, JourneyCPEWebPA}
	if err := capabilities.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate capability error, got %v", err)
	}
}

func TestInvocationValidateFor(t *testing.T) {
	tests := []struct {
		name       string
		journey    string
		invocation Invocation
		wantErr    string
	}{
		{name: "cpe webpa", journey: JourneyCPEWebPA, invocation: Invocation{ClientService: "apparmor-simulator"}},
		{name: "cpe webpa preserves case-sensitive client service", journey: JourneyCPEWebPA, invocation: Invocation{ClientService: "HermesFS"}},
		{name: "webhook passive", journey: JourneyWebhook, invocation: Invocation{}},
		{name: "webhook active", journey: JourneyWebhook, invocation: Invocation{AllowActiveCallback: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455"}},
		{name: "callback active", journey: JourneyCPEWebPACallback, invocation: Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", 64)}},
		{name: "callback active preserves case-sensitive client service", journey: JourneyCPEWebPACallback, invocation: Invocation{ClientService: "HermesFS", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", 64)}},
		{name: "Parodus clients", journey: JourneyParodusClients, invocation: Invocation{}},
		{name: "Argus webhooks", journey: JourneyArgusWebhooks, invocation: Invocation{}},
		{name: "webhook rejects client service", journey: JourneyWebhook, invocation: Invocation{ClientService: "config"}, wantErr: "client service"},
		{name: "cpe rejects webhook field", journey: JourneyCPEWebPA, invocation: Invocation{ClientService: "config", AllowActiveCallback: true}, wantErr: "webhook invocation fields"},
		{name: "callback requires consent", journey: JourneyCPEWebPACallback, invocation: Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", 64)}, wantErr: "active event consent"},
		{name: "callback requires correlation", journey: JourneyCPEWebPACallback, invocation: Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455"}, wantErr: "correlation ID"},
		{name: "callback rejects webhook phase", journey: JourneyCPEWebPACallback, invocation: Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", 64), ActivePhase: WebhookActiveDirect}, wantErr: "webhook invocation fields"},
		{name: "passive rejects event", journey: JourneyWebhook, invocation: Invocation{Event: "apparmor/diagnostic"}, wantErr: "require active callback consent"},
		{name: "active requires event", journey: JourneyWebhook, invocation: Invocation{AllowActiveCallback: true, DeviceID: "mac:001122334455"}, wantErr: "event"},
		{name: "active requires device identity", journey: JourneyWebhook, invocation: Invocation{AllowActiveCallback: true, Event: "apparmor/diagnostic"}, wantErr: "device identity"},
		{name: "invalid event", journey: JourneyWebhook, invocation: Invocation{AllowActiveCallback: true, Event: "event:apparmor", DeviceID: "mac:001122334455"}, wantErr: "event"},
		{name: "invalid device identity", journey: JourneyWebhook, invocation: Invocation{AllowActiveCallback: true, Event: "apparmor/diagnostic", DeviceID: "001122334455"}, wantErr: "device identity"},
		{name: "Parodus clients rejects client service", journey: JourneyParodusClients, invocation: Invocation{ClientService: "config"}, wantErr: "does not accept"},
		{name: "Argus webhooks rejects client service", journey: JourneyArgusWebhooks, invocation: Invocation{ClientService: "config"}, wantErr: "does not accept"},
		{name: "unknown journey", journey: "other", wantErr: "unsupported diagnostic journey"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.invocation.ValidateFor(test.journey)
			if test.wantErr == "" && err != nil {
				t.Fatalf("ValidateFor() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("ValidateFor() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestResultValidateWebhookEvidence(t *testing.T) {
	result := validResult()
	result.Observations[0].Evidence = []Evidence{
		{Key: "registration-fingerprint", Value: strings.Repeat("a", 64)},
		{Key: "http-status", Value: "202"},
		{Key: "correlation-state", Value: "recorded"},
		{Key: "participant-observed-at", Value: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid webhook evidence rejected: %v", err)
	}
	tests := []struct {
		name  string
		entry Evidence
		want  string
	}{
		{name: "unknown key", entry: Evidence{Key: "secret", Value: "value"}, want: "not allowed"},
		{name: "invalid fingerprint", entry: Evidence{Key: "registration-fingerprint", Value: "abc"}, want: "fingerprint"},
		{name: "invalid status", entry: Evidence{Key: "http-status", Value: "700"}, want: "HTTP status"},
		{name: "invalid correlation state", entry: Evidence{Key: "correlation-state", Value: "anything"}, want: "correlation state"},
		{name: "invalid participant timestamp", entry: Evidence{Key: "participant-observed-at", Value: "soon"}, want: "participant observation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := validResult()
			invalid.Observations[0].Evidence = []Evidence{test.entry}
			if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
