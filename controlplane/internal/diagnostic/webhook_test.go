package diagnostic

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/wrp-go/v3"
)

func TestWebhookProbeUsesOwnerlessArgusLookupAndBoundsCandidateData(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	item, err := ancla.InternalWebhookToItem(func() time.Time { return now }, ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Config:   ancla.DeliveryConfig{URL: "http://event-sink:8080/webhook", ContentType: "application/json", Secret: "diagnostic-secret"},
			Events:   []string{"devices/.*"},
			Matcher:  ancla.MetadataMatcherConfig{DeviceID: []string{"mac:.*"}},
			Duration: 12 * time.Hour,
			Until:    now.Add(12 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("make Argus item: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/store/webhooks" {
			t.Fatalf("Argus request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Xmidt-Owner") != "" {
			t.Fatalf("unexpected owner header %q", request.Header.Get("X-Xmidt-Owner"))
		}
		if request.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode([]any{item})
	}))
	defer server.Close()

	probe := WebhookProbe{ArgusURL: server.URL, BasicAuth: "dXNlcjpwYXNz", HTTPClient: server.Client(), Now: func() time.Time { return now }}
	candidates, err := probe.Candidates(context.Background())
	if err != nil {
		t.Fatalf("Candidates() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want one", len(candidates))
	}
	candidate := candidates[0]
	if candidate.Fingerprint != item.ID || candidate.CallbackURL != "http://event-sink:8080/webhook" || !candidate.SecretPresent || candidate.TTLSeconds != 12*60*60 {
		t.Fatalf("candidate = %+v", candidate)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	if strings.Contains(string(encoded), "diagnostic-secret") {
		t.Fatal("candidate serialized its webhook secret")
	}
}

func TestWebhookProbeRefusesOversizedCandidateSet(t *testing.T) {
	items := make(chrysom.Items, MaxWebhookCandidates+1)
	probe := WebhookProbe{
		MaxItems: MaxWebhookCandidates,
		getItems: func(context.Context) (chrysom.Items, error) { return items, nil },
	}
	if _, err := probe.Candidates(context.Background()); !errors.Is(err, ErrWebhookCandidateLimit) {
		t.Fatalf("Candidates() error = %v, want candidate limit", err)
	}
}

func TestWebhookProbeInventoryListsAllSafeRegistrations(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	first, err := ancla.InternalWebhookToItem(func() time.Time { return now }, ancla.InternalWebhook{Webhook: ancla.Webhook{
		Config:   ancla.DeliveryConfig{URL: "http://user:password@EVENT-SINK:80/webhook?token=private", ContentType: "application/json", Secret: "first-secret"},
		Events:   []string{"devices/.*", "apparmor/.*"},
		Matcher:  ancla.MetadataMatcherConfig{DeviceID: []string{"serial:.*", "mac:.*"}},
		Duration: webhookDuration,
		Until:    now.Add(webhookDuration),
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ancla.InternalWebhookToItem(func() time.Time { return now }, ancla.InternalWebhook{Webhook: ancla.Webhook{
		Config:   ancla.DeliveryConfig{URL: "http://other/webhook", ContentType: "application/json", Secret: "second-secret"},
		Events:   []string{"devices/.*"},
		Matcher:  ancla.MetadataMatcherConfig{DeviceID: []string{"mac:.*"}},
		Duration: webhookDuration,
		Until:    now.Add(webhookDuration),
	}})
	if err != nil {
		t.Fatal(err)
	}
	probe := WebhookProbe{getItems: func(context.Context) (chrysom.Items, error) { return chrysom.Items{second, first}, nil }}
	registrations, err := probe.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	if len(registrations) != 2 || registrations[0].Fingerprint >= registrations[1].Fingerprint {
		t.Fatalf("registrations = %+v", registrations)
	}
	for _, registration := range registrations {
		if registration.CallbackURL == "http://event-sink/webhook" && (len(registration.EventFilters) != 2 || registration.EventFilters[0] != "apparmor/.*") {
			t.Fatalf("registration filters = %+v", registration.EventFilters)
		}
	}
	encoded, err := json.Marshal(registrations)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "first-secret") || strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "token=private") {
		t.Fatalf("inventory exposed unsafe data: %s", encoded)
	}
}

func TestWebhookProbeInventoryRefusesOversizedOrMalformedBucket(t *testing.T) {
	probe := WebhookProbe{getItems: func(context.Context) (chrysom.Items, error) {
		return make(chrysom.Items, MaxWebhookRegistrations+1), nil
	}}
	if _, err := probe.Inventory(context.Background()); !errors.Is(err, ErrWebhookInventoryLimit) {
		t.Fatalf("Inventory() error = %v, want inventory limit", err)
	}
	malformed := WebhookProbe{getItems: func(context.Context) (chrysom.Items, error) {
		return chrysom.Items{{ID: strings.Repeat("a", 64), Data: map[string]any{"webhook": true}}}, nil
	}}
	if _, err := malformed.Inventory(context.Background()); !errors.Is(err, ErrWebhookInventoryInvalid) {
		t.Fatalf("Inventory() error = %v, want invalid inventory", err)
	}
}

func TestWebhookProbeRunInventorySeparatesFailures(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name       string
		getItems   func(context.Context) (chrysom.Items, error)
		wantEdges  []string
		wantState  State
		wantReason string
	}{
		{name: "authentication", getItems: func(context.Context) (chrysom.Items, error) { return nil, chrysom.ErrFailedAuthentication }, wantEdges: []string{"argus-reachability", "argus-inventory"}, wantState: StateFailed, wantReason: ReasonArgusAuthenticationFailed},
		{name: "transport", getItems: func(context.Context) (chrysom.Items, error) { return nil, context.DeadlineExceeded }, wantEdges: []string{"argus-reachability"}, wantState: StateFailed, wantReason: ReasonArgusUnreachable},
		{name: "limit", getItems: func(context.Context) (chrysom.Items, error) {
			return make(chrysom.Items, MaxWebhookRegistrations+1), nil
		}, wantEdges: []string{"argus-reachability", "argus-inventory"}, wantState: StateUnknown, wantReason: ReasonArgusInventoryUnavailable},
		{name: "decode", getItems: func(context.Context) (chrysom.Items, error) {
			return chrysom.Items{{ID: strings.Repeat("a", 64), Data: map[string]any{"webhook": true}}}, nil
		}, wantEdges: []string{"argus-reachability", "argus-inventory"}, wantState: StateUnknown, wantReason: ReasonArgusInventoryUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := (WebhookProbe{Now: func() time.Time { return now }, getItems: testCase.getItems}).RunInventory(context.Background(), Invocation{})
			if response.WebhookRegistrations != nil || len(response.Observations) != len(testCase.wantEdges) {
				t.Fatalf("response = %+v", response)
			}
			for index, edgeID := range testCase.wantEdges {
				if response.Observations[index].EdgeID != edgeID {
					t.Fatalf("observation %d = %+v, want %q", index, response.Observations[index], edgeID)
				}
			}
			last := response.Observations[len(response.Observations)-1]
			if last.State != testCase.wantState || last.ReasonID != testCase.wantReason {
				t.Fatalf("last observation = %+v", last)
			}
		})
	}
}

func TestMatchWebhookCandidateNormalizesSafeCallbackIdentity(t *testing.T) {
	candidate, err := MatchWebhookCandidate("http://user:password@EVENT-SINK:80/webhook?token=private#fragment", []WebhookCandidate{
		{Fingerprint: "first", CallbackURL: "http://event-sink/webhook"},
	})
	if err != nil {
		t.Fatalf("MatchWebhookCandidate() error = %v", err)
	}
	if candidate.Fingerprint != "first" {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestMatchWebhookCandidateDistinguishesRegistrationOutcomes(t *testing.T) {
	matching := WebhookCandidate{Fingerprint: "first", CallbackURL: "http://event-sink/webhook"}
	for _, testCase := range []struct {
		name       string
		candidates []WebhookCandidate
		want       error
	}{
		{name: "missing", candidates: []WebhookCandidate{{Fingerprint: "other", CallbackURL: "http://other/webhook"}}, want: ErrWebhookRegistrationMissing},
		{name: "ambiguous", candidates: []WebhookCandidate{matching, {Fingerprint: "second", CallbackURL: "http://event-sink/webhook"}}, want: ErrWebhookRegistrationAmbiguous},
		{name: "excessive", candidates: make([]WebhookCandidate, MaxWebhookCandidates+1), want: ErrWebhookCandidateLimit},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := MatchWebhookCandidate("http://event-sink/webhook", testCase.candidates)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("MatchWebhookCandidate() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestEvaluateWebhookFreshness(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name      string
		candidate WebhookCandidate
		want      error
	}{
		{name: "fresh", candidate: WebhookCandidate{Duration: webhookDuration, Until: now.Add(12 * time.Hour), TTLKnown: true, TTLSeconds: 12 * 60 * 60}},
		{name: "expired until", candidate: WebhookCandidate{Duration: webhookDuration, Until: now.Add(-time.Second)}, want: ErrWebhookRegistrationExpired},
		{name: "expired item TTL", candidate: WebhookCandidate{Duration: webhookDuration, Until: now.Add(12 * time.Hour), TTLKnown: true, TTLSeconds: 0}, want: ErrWebhookRegistrationExpired},
		{name: "near refresh deadline", candidate: WebhookCandidate{Duration: webhookDuration, Until: now.Add(webhookRefreshPeriod)}, want: ErrWebhookRegistrationStale},
		{name: "incorrect duration", candidate: WebhookCandidate{Duration: time.Hour, Until: now.Add(12 * time.Hour)}, want: ErrWebhookRegistrationStale},
		{name: "near item TTL deadline", candidate: WebhookCandidate{Duration: webhookDuration, Until: now.Add(12 * time.Hour), TTLKnown: true, TTLSeconds: int64(webhookRefreshPeriod / time.Second)}, want: ErrWebhookRegistrationStale},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := EvaluateWebhookFreshness(now, testCase.candidate)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("EvaluateWebhookFreshness() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestCompareWebhookConformanceDoesNotExposeSecret(t *testing.T) {
	intent := WebhookRegistrationIntent{
		CallbackURL:      "http://event-sink:8080/webhook?token=not-an-identity",
		EventFilter:      "devices/.*",
		DeviceMatcher:    "mac:.*",
		ContentType:      "application/json; charset=utf-8",
		SecretConfigured: true,
		secret:           "expected-secret",
	}
	candidate := WebhookCandidate{
		CallbackURL:    "http://event-sink:8080/webhook",
		EventFilters:   []string{"devices/.*"},
		DeviceMatchers: []string{"mac:.*"},
		ContentType:    "Application/JSON; charset=utf-8",
		SecretPresent:  true,
		secret:         "different-secret",
	}
	mismatches := CompareWebhookConformance(intent, candidate)
	if len(mismatches) != 1 || mismatches[0] != "secret" {
		t.Fatalf("mismatches = %v", mismatches)
	}
	encoded, err := json.Marshal(struct {
		Intent     WebhookRegistrationIntent
		Candidate  WebhookCandidate
		Mismatches []string
	}{intent, candidate, mismatches})
	if err != nil {
		t.Fatalf("marshal conformance result: %v", err)
	}
	if strings.Contains(string(encoded), "expected-secret") || strings.Contains(string(encoded), "different-secret") {
		t.Fatal("conformance data serialized a secret")
	}
}

func TestCompareWebhookConformanceIdentifiesOnlyMismatchedFields(t *testing.T) {
	intent := WebhookRegistrationIntent{
		CallbackURL:      "http://event-sink/webhook",
		EventFilter:      "devices/.*",
		DeviceMatcher:    "mac:.*",
		ContentType:      "application/json",
		SecretConfigured: true,
	}
	candidate := WebhookCandidate{
		CallbackURL:    "http://other/webhook",
		EventFilters:   []string{"different"},
		DeviceMatchers: []string{"different"},
		ContentType:    "text/plain",
		SecretPresent:  false,
	}
	got := CompareWebhookConformance(intent, candidate)
	want := []string{"callback-url", "event-filter", "device-matcher", "content-type", "secret-configured"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("mismatches = %v, want %v", got, want)
	}
}

func TestWebhookProbeRunSeparatesArgusFailuresByStage(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name       string
		getItems   func(context.Context) (chrysom.Items, error)
		wantEdges  []string
		wantState  State
		wantReason string
	}{
		{
			name: "authentication",
			getItems: func(context.Context) (chrysom.Items, error) {
				return nil, chrysom.ErrFailedAuthentication
			},
			wantEdges:  []string{"argus-reachability", "argus-authentication"},
			wantState:  StateFailed,
			wantReason: ReasonArgusAuthenticationFailed,
		},
		{
			name: "transport",
			getItems: func(context.Context) (chrysom.Items, error) {
				return nil, context.DeadlineExceeded
			},
			wantEdges:  []string{"argus-reachability"},
			wantState:  StateFailed,
			wantReason: ReasonArgusUnreachable,
		},
		{
			name: "excessive",
			getItems: func(context.Context) (chrysom.Items, error) {
				return make(chrysom.Items, MaxWebhookCandidates+1), nil
			},
			wantEdges:  []string{"argus-reachability", "argus-authentication", "registration-present"},
			wantState:  StateFailed,
			wantReason: ReasonRegistrationAmbiguous,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			probe := WebhookProbe{Now: func() time.Time { return now }, getItems: testCase.getItems}
			response := probe.RunWithInvocation(context.Background(), Invocation{})
			if len(response.Observations) != len(testCase.wantEdges) {
				t.Fatalf("observations = %+v", response.Observations)
			}
			for index, edge := range testCase.wantEdges {
				if response.Observations[index].EdgeID != edge {
					t.Fatalf("observation %d edge = %q, want %q", index, response.Observations[index].EdgeID, edge)
				}
			}
			observation := response.Observations[len(response.Observations)-1]
			if observation.State != testCase.wantState || observation.ReasonID != testCase.wantReason {
				t.Fatalf("last observation = %+v", observation)
			}
		})
	}
}

func TestWebhookProbeRunReportsHTTPAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	probe := WebhookProbe{ArgusURL: server.URL, BasicAuth: "dXNlcjpwYXNz", HTTPClient: server.Client()}
	response := probe.RunWithInvocation(context.Background(), Invocation{})
	if len(response.Observations) != 2 {
		t.Fatalf("observations = %+v", response.Observations)
	}
	if response.Observations[0].State != StatePassed || response.Observations[1].EdgeID != "argus-authentication" || response.Observations[1].ReasonID != ReasonArgusAuthenticationFailed {
		t.Fatalf("observations = %+v", response.Observations)
	}
}

func TestWebhookProbeRunInspectsSubscriberIntentWithoutActiveTraffic(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	intent := &WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	item, err := ancla.InternalWebhookToItem(func() time.Time { return now }, ancla.InternalWebhook{Webhook: ancla.Webhook{
		Config:   ancla.DeliveryConfig{URL: intent.CallbackURL, ContentType: intent.ContentType, Secret: "stored-secret"},
		Events:   []string{intent.EventFilter},
		Matcher:  ancla.MetadataMatcherConfig{DeviceID: []string{intent.DeviceMatcher}},
		Duration: webhookDuration,
		Until:    now.Add(webhookDuration),
	}})
	if err != nil {
		t.Fatal(err)
	}
	probe := WebhookProbe{
		Now: func() time.Time { return now },
		getItems: func(context.Context) (chrysom.Items, error) {
			return chrysom.Items{item}, nil
		},
	}
	response := probe.RunWithInvocation(context.Background(), Invocation{SubscriberIntent: intent})
	if len(response.Observations) != 5 {
		t.Fatalf("observations = %+v", response.Observations)
	}
	for _, observation := range response.Observations {
		if observation.State != StatePassed {
			t.Fatalf("observation = %+v", observation)
		}
	}
}

func TestWebhookProbeRunReportsActiveFailureObservations(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	intent := &WebhookSubscriberIntent{SchemaVersion: SchemaVersion, Journey: "webhook-subscriber", ObservedAt: now, CallbackURL: "http://event-sink:8080/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true}
	item, err := ancla.InternalWebhookToItem(func() time.Time { return now }, ancla.InternalWebhook{Webhook: ancla.Webhook{
		Config:   ancla.DeliveryConfig{URL: intent.CallbackURL, ContentType: intent.ContentType, Secret: "stored-secret"},
		Events:   []string{intent.EventFilter},
		Matcher:  ancla.MetadataMatcherConfig{DeviceID: []string{intent.DeviceMatcher}},
		Duration: webhookDuration,
		Until:    now.Add(webhookDuration),
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name       string
		invocation Invocation
		configure  func(*WebhookProbe)
		wantEdge   string
		wantReason string
	}{
		{
			name:       "direct callback DNS",
			invocation: Invocation{SubscriberIntent: intent, AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455", ActivePhase: WebhookActiveDirect},
			configure: func(probe *WebhookProbe) {
				probe.LookupHost = func(context.Context, string) ([]string, error) { return nil, errors.New("not found") }
			},
			wantEdge:   "callback-dns",
			wantReason: ReasonCallbackDNSFailed,
		},
		{
			name:       "representative event mismatch",
			invocation: Invocation{SubscriberIntent: intent, AllowActiveCallback: true, Event: "other/diagnostic", DeviceID: "mac:001122334455", ActivePhase: WebhookActiveCaduceus},
			configure:  func(*WebhookProbe) {},
			wantEdge:   "caduceus-ingestion",
			wantReason: ReasonCaduceusIngestionRejected,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			probe := WebhookProbe{Now: func() time.Time { return now }, getItems: func(context.Context) (chrysom.Items, error) {
				return chrysom.Items{item}, nil
			}}
			testCase.configure(&probe)
			response := probe.RunWithInvocation(context.Background(), testCase.invocation)
			last := response.Observations[len(response.Observations)-1]
			if response.Active != nil || last.EdgeID != testCase.wantEdge || last.State != StateFailed || last.ReasonID != testCase.wantReason {
				t.Fatalf("response = %+v", response)
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "stored-secret") {
				t.Fatalf("active failure response exposed stored secret: %s", encoded)
			}
		})
	}
}

func TestWebhookProbeRejectsMalformedArgusItem(t *testing.T) {
	item, err := ancla.InternalWebhookToItem(time.Now, ancla.InternalWebhook{Webhook: ancla.Webhook{Config: ancla.DeliveryConfig{URL: "http://event-sink/webhook"}}})
	if err != nil {
		t.Fatalf("make Argus item: %v", err)
	}
	item.Data = map[string]any{"Webhook": true}
	probe := WebhookProbe{getItems: func(context.Context) (chrysom.Items, error) {
		return chrysom.Items{item}, nil
	}}
	if _, err := probe.Candidates(context.Background()); err == nil || strings.Contains(err.Error(), "diagnostic-secret") {
		t.Fatalf("Candidates() error = %v", err)
	}
}

func TestCompareWebhookConformanceAcceptsMatchingRegistration(t *testing.T) {
	intent := WebhookRegistrationIntent{CallbackURL: "http://event-sink/webhook", EventFilter: "devices/.*", DeviceMatcher: "mac:.*", ContentType: "application/json", SecretConfigured: true, secret: "same"}
	candidate := WebhookCandidate{CallbackURL: "http://event-sink/webhook", EventFilters: []string{"devices/.*"}, DeviceMatchers: []string{"mac:.*"}, ContentType: "application/json", SecretPresent: true, secret: "same"}
	if mismatches := CompareWebhookConformance(intent, candidate); len(mismatches) != 0 {
		t.Fatalf("mismatches = %v", mismatches)
	}
}

func TestValidateRepresentativeSelection(t *testing.T) {
	candidate := WebhookCandidate{EventFilters: []string{"devices/.*"}, DeviceMatchers: []string{"mac:.*"}}
	if err := ValidateRepresentativeSelection(candidate, "devices/diagnostic", "mac:001122334455"); err != nil {
		t.Fatalf("ValidateRepresentativeSelection() error = %v", err)
	}
	for _, testCase := range []struct {
		name      string
		candidate WebhookCandidate
		event     string
		deviceID  string
		want      error
	}{
		{name: "event mismatch", candidate: candidate, event: "other/diagnostic", deviceID: "mac:001122334455", want: ErrWebhookEventMismatch},
		{name: "device mismatch", candidate: candidate, event: "devices/diagnostic", deviceID: "serial:001122334455", want: ErrWebhookDeviceMismatch},
		{name: "invalid event regex", candidate: WebhookCandidate{EventFilters: []string{"["}, DeviceMatchers: candidate.DeviceMatchers}, event: "devices/diagnostic", deviceID: "mac:001122334455", want: ErrWebhookEventFilterInvalid},
		{name: "invalid device regex", candidate: WebhookCandidate{EventFilters: candidate.EventFilters, DeviceMatchers: []string{"["}}, event: "devices/diagnostic", deviceID: "mac:001122334455", want: ErrWebhookDeviceMatcherInvalid},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateRepresentativeSelection(testCase.candidate, testCase.event, testCase.deviceID)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("ValidateRepresentativeSelection() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestWebhookProbePreflightDirectCallbackGatesNetworkActivity(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		invocation      Invocation
		registrationErr error
	}{
		{name: "passive", invocation: Invocation{}},
		{name: "registration failed", invocation: Invocation{AllowActiveCallback: true, Event: "devices/test", DeviceID: "mac:001122334455"}, registrationErr: ErrWebhookRegistrationMissing},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			lookupCalled := false
			dialCalled := false
			probe := WebhookProbe{
				LookupHost:  func(context.Context, string) ([]string, error) { lookupCalled = true; return nil, nil },
				DialContext: func(context.Context, string, string) (net.Conn, error) { dialCalled = true; return nil, nil },
			}
			_, err := probe.preflightDirectCallback(context.Background(), testCase.invocation, WebhookCandidate{CallbackURL: "http://event-sink/webhook"}, testCase.registrationErr)
			if !errors.Is(err, ErrActiveCallbackNotPermitted) || lookupCalled || dialCalled {
				t.Fatalf("preflight error = %v, lookup %t, dial %t", err, lookupCalled, dialCalled)
			}
		})
	}
}

func TestWebhookProbePreflightDirectCallbackUsesStoredDestination(t *testing.T) {
	probe := WebhookProbe{
		HTTPClient: &http.Client{Timeout: time.Second},
		LookupHost: func(_ context.Context, host string) ([]string, error) {
			if host != "event-sink" {
				t.Fatalf("lookup host = %q", host)
			}
			return []string{"10.0.0.3"}, nil
		},
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != "event-sink:8080" {
				t.Fatalf("dial = %s %s", network, address)
			}
			return stubConnection{ReadWriteCloser: nopReadWriteCloser{}}, nil
		},
	}
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/test", DeviceID: "mac:001122334455"}
	target, err := probe.preflightDirectCallback(context.Background(), invocation, WebhookCandidate{CallbackURL: "http://event-sink:8080/webhook"}, nil)
	if err != nil || target.address != "event-sink:8080" || target.url.String() != "http://event-sink:8080/webhook" {
		t.Fatalf("preflight target = %+v, %v", target, err)
	}
}

func TestWebhookProbePreflightDirectCallbackReportsDNSAndTransportFailures(t *testing.T) {
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/test", DeviceID: "mac:001122334455"}
	for _, testCase := range []struct {
		name  string
		probe WebhookProbe
		want  error
	}{
		{
			name:  "DNS",
			probe: WebhookProbe{LookupHost: func(context.Context, string) ([]string, error) { return nil, errors.New("no records") }},
			want:  ErrCallbackDNS,
		},
		{
			name: "TCP",
			probe: WebhookProbe{
				LookupHost:  func(context.Context, string) ([]string, error) { return []string{"10.0.0.3"}, nil },
				DialContext: func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("connection refused") },
			},
			want: ErrCallbackTransport,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.probe.preflightDirectCallback(context.Background(), invocation, WebhookCandidate{CallbackURL: "http://event-sink:8080/webhook"}, nil)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("preflight error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestWebhookProbeSendsOneSignedDiagnosticCallback(t *testing.T) {
	const secret = "callback-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request = %s, content-type %q", request.Method, request.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		mac := hmac.New(sha1.New, []byte(secret))
		_, _ = mac.Write(body)
		if request.Header.Get("X-Webpa-Signature") != "sha1="+hex.EncodeToString(mac.Sum(nil)) {
			t.Fatal("diagnostic callback signature did not match body")
		}
		var marker struct {
			Diagnostic    string `json:"vcpe_diagnostic"`
			CorrelationID string `json:"correlation_id"`
		}
		if err := json.Unmarshal(body, &marker); err != nil {
			t.Fatalf("decode marker: %v", err)
		}
		if marker.Diagnostic != "webhook-registration-callback-diagnostics" || marker.CorrelationID != strings.Repeat("a", 64) {
			t.Fatalf("marker = %+v", marker)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	probe := WebhookProbe{HTTPClient: server.Client(), newCorrelationID: func() (string, error) { return strings.Repeat("a", 64), nil }}
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/test", DeviceID: "mac:001122334455"}
	result, err := probe.sendDiagnosticCallback(context.Background(), invocation, WebhookCandidate{CallbackURL: server.URL, secret: secret}, nil)
	if err != nil || result.correlationID != strings.Repeat("a", 64) || result.httpStatus != http.StatusNoContent {
		t.Fatalf("callback result = %+v, %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal callback result: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("callback result exposed its secret")
	}
}

func TestWebhookProbeRejectsRedirectAndNonSuccessCallbackResponses(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if status == http.StatusFound {
					writer.Header().Set("Location", "http://unexpected.example/other")
				}
				writer.WriteHeader(status)
			}))
			defer server.Close()

			probe := WebhookProbe{HTTPClient: server.Client(), newCorrelationID: func() (string, error) { return strings.Repeat("b", 64), nil }}
			invocation := Invocation{AllowActiveCallback: true, Event: "devices/test", DeviceID: "mac:001122334455"}
			result, err := probe.sendDiagnosticCallback(context.Background(), invocation, WebhookCandidate{CallbackURL: server.URL, secret: "callback-secret"}, nil)
			if !errors.Is(err, ErrCallbackRejected) || result.httpStatus != status || strings.Contains(err.Error(), "callback-secret") {
				t.Fatalf("callback result = %+v, error = %v", result, err)
			}
		})
	}
}

func TestWebhookProbeReportsCallbackTimeoutWithoutSensitiveDetails(t *testing.T) {
	probe := WebhookProbe{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})},
		LookupHost: func(context.Context, string) ([]string, error) { return []string{"10.0.0.3"}, nil },
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return stubConnection{ReadWriteCloser: nopReadWriteCloser{}}, nil
		},
		newCorrelationID: func() (string, error) { return strings.Repeat("c", 64), nil },
	}
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/test", DeviceID: "mac:001122334455"}
	_, err := probe.sendDiagnosticCallback(context.Background(), invocation, WebhookCandidate{CallbackURL: "http://event-sink:8080/webhook", secret: "callback-secret"}, nil)
	if err == nil || strings.Contains(err.Error(), "callback-secret") {
		t.Fatalf("callback error = %v", err)
	}
}

func TestWebhookProbeInjectsBoundedCaduceusEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v4/notify" || request.Header.Get("Content-Type") != wrpMsgpackContentType || request.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var message wrp.Message
		if err := wrp.NewDecoder(request.Body, wrp.Msgpack).Decode(&message); err != nil {
			t.Fatal(err)
		}
		if message.Type != wrp.SimpleEventMessageType || message.Destination != "event:devices/diagnostic/mac:001122334455" {
			t.Fatalf("message = %+v", message)
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	probe := WebhookProbe{CaduceusURL: server.URL + "/api/v4/notify", CaduceusAuth: "Basic dXNlcjpwYXNz", HTTPClient: server.Client(), newCorrelationID: func() (string, error) { return strings.Repeat("d", 64), nil }}
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"}
	candidate := WebhookCandidate{EventFilters: []string{"devices/.*"}, DeviceMatchers: []string{"mac:.*"}}
	result, err := probe.injectCaduceusEvent(context.Background(), invocation, candidate, nil)
	if err != nil || result.correlationID != strings.Repeat("d", 64) || result.httpStatus != http.StatusAccepted {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestWebhookProbeSeparatesCaduceusAcknowledgementFromReceipt(t *testing.T) {
	const correlationID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	subscriber := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(Receipt{SchemaVersion: SchemaVersion, CorrelationID: correlationID, Source: "caduceus", AcceptedAt: time.Now().UTC(), HTTPStatus: http.StatusNoContent})
	}))
	defer subscriber.Close()
	ingress := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusAccepted) }))
	defer ingress.Close()
	probe := WebhookProbe{CaduceusURL: ingress.URL, CaduceusAuth: "Basic dXNlcjpwYXNz", HTTPClient: ingress.Client(), newCorrelationID: func() (string, error) { return correlationID, nil }}
	client := NewClient(time.Second)
	client.ReceiptPollInterval = time.Millisecond
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"}
	candidate := WebhookCandidate{EventFilters: []string{"devices/.*"}, DeviceMatchers: []string{"mac:.*"}}
	result, err := probe.injectCaduceusAndPoll(context.Background(), client, testTarget(t, subscriber.URL), invocation, candidate, nil)
	if err != nil || result.ingestion.httpStatus != http.StatusAccepted || result.receipt.Source != "caduceus" {
		t.Fatalf("delivery result = %+v, %v", result, err)
	}
}

func TestWebhookProbeCaduceusDeliveryBoundaries(t *testing.T) {
	const correlationID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	invocation := Invocation{AllowActiveCallback: true, Event: "devices/diagnostic", DeviceID: "mac:001122334455"}
	matchingCandidate := WebhookCandidate{EventFilters: []string{"devices/.*"}, DeviceMatchers: []string{"mac:.*"}}
	for _, testCase := range []struct {
		name              string
		candidate         WebhookCandidate
		ingressStatus     int
		ingressTransport  http.RoundTripper
		subscriberHandler http.HandlerFunc
		want              error
		wantAccepted      bool
		wantIngressCalls  int
	}{
		{
			name:             "event mismatch",
			candidate:        WebhookCandidate{EventFilters: []string{"other/.*"}, DeviceMatchers: matchingCandidate.DeviceMatchers},
			want:             ErrWebhookEventMismatch,
			wantIngressCalls: 0,
		},
		{
			name:             "device mismatch",
			candidate:        WebhookCandidate{EventFilters: matchingCandidate.EventFilters, DeviceMatchers: []string{"serial:.*"}},
			want:             ErrWebhookDeviceMismatch,
			wantIngressCalls: 0,
		},
		{
			name:             "ingestion rejected",
			candidate:        matchingCandidate,
			ingressStatus:    http.StatusServiceUnavailable,
			want:             ErrCaduceusIngestionRejected,
			wantIngressCalls: 1,
		},
		{
			name:             "ingestion timeout",
			candidate:        matchingCandidate,
			ingressTransport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }),
			want:             context.DeadlineExceeded,
			wantIngressCalls: 0,
		},
		{
			name:          "accepted without receipt",
			candidate:     matchingCandidate,
			ingressStatus: http.StatusAccepted,
			subscriberHandler: func(writer http.ResponseWriter, request *http.Request) {
				http.NotFound(writer, request)
			},
			want:             ErrReceiptMissing,
			wantAccepted:     true,
			wantIngressCalls: 1,
		},
		{
			name:          "duplicate receipt remains delivered",
			candidate:     matchingCandidate,
			ingressStatus: http.StatusAccepted,
			subscriberHandler: func(writer http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(writer).Encode(Receipt{SchemaVersion: SchemaVersion, CorrelationID: correlationID, Source: "caduceus", AcceptedAt: time.Now().UTC(), HTTPStatus: http.StatusNoContent})
			},
			wantAccepted:     true,
			wantIngressCalls: 1,
		},
		{
			name:          "subscriber restart loses receipt state",
			candidate:     matchingCandidate,
			ingressStatus: http.StatusAccepted,
			subscriberHandler: func(writer http.ResponseWriter, request *http.Request) {
				http.NotFound(writer, request)
			},
			want:             ErrReceiptMissing,
			wantAccepted:     true,
			wantIngressCalls: 1,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ingressCalls := 0
			ingress := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				ingressCalls++
				writer.WriteHeader(testCase.ingressStatus)
			}))
			defer ingress.Close()
			subscriber := httptest.NewServer(testCase.subscriberHandler)
			defer subscriber.Close()

			httpClient := ingress.Client()
			if testCase.ingressTransport != nil {
				httpClient = &http.Client{Transport: testCase.ingressTransport}
			}
			probe := WebhookProbe{CaduceusURL: ingress.URL, CaduceusAuth: "Basic dXNlcjpwYXNz", HTTPClient: httpClient, newCorrelationID: func() (string, error) { return correlationID, nil }}
			client := NewClient(time.Second)
			client.ReceiptPollInterval = time.Millisecond
			result, err := probe.injectCaduceusAndPoll(context.Background(), client, testTarget(t, subscriber.URL), invocation, testCase.candidate, nil)
			if !errors.Is(err, testCase.want) || ingressCalls != testCase.wantIngressCalls {
				t.Fatalf("delivery result = %+v, error = %v, ingress calls = %d", result, err, ingressCalls)
			}
			if testCase.wantAccepted && result.ingestion.httpStatus != http.StatusAccepted {
				t.Fatalf("ingestion result = %+v, want HTTP %d", result.ingestion, http.StatusAccepted)
			}
		})
	}
}
