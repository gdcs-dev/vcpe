package diagnostic

import (
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

func TestResultValidate(t *testing.T) {
	if err := validResult().Validate(); err != nil {
		t.Fatalf("valid result rejected: %v", err)
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
		{name: "webhook passive", journey: JourneyWebhook, invocation: Invocation{}},
		{name: "webhook active", journey: JourneyWebhook, invocation: Invocation{AllowActiveCallback: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455"}},
		{name: "callback active", journey: JourneyCPEWebPACallback, invocation: Invocation{ClientService: "apparmor-simulator", Subscriber: "event-sink", AllowActiveEvent: true, Event: "apparmor/diagnostic", DeviceID: "mac:001122334455", CorrelationID: strings.Repeat("a", 64)}},
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
