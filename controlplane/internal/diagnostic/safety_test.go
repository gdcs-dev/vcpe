package diagnostic

import (
	"strings"
	"testing"
)

func TestSanitizeBoundsAndRedactsEvidence(t *testing.T) {
	result := validResult()
	result.Observations[0].Message = "Authorization: Bearer abc.def"
	result.Observations[0].Evidence = []Evidence{
		{Key: "authorization", Value: "Bearer abc"},
		{Key: "endpoint", Value: strings.Repeat("x", MaxEvidenceValueLength+20)},
		{Key: "device-id", Value: "password=hunter2"},
	}
	clean, err := Sanitize(result)
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if clean.Observations[0].Message != "[REDACTED]" {
		t.Errorf("message = %q", clean.Observations[0].Message)
	}
	if len(clean.Observations[0].Evidence) != 2 {
		t.Fatalf("evidence count = %d, want 2", len(clean.Observations[0].Evidence))
	}
	if got := clean.Observations[0].Evidence[0].Value; len(got) != MaxEvidenceValueLength || !strings.HasSuffix(got, "...") {
		t.Errorf("bounded value length = %d, value suffix = %q", len(got), got[len(got)-3:])
	}
	if got := clean.Observations[0].Evidence[1].Value; got != "[REDACTED]" {
		t.Errorf("sensitive evidence = %q", got)
	}
}

func TestSanitizeRedactsCorrelationToken(t *testing.T) {
	result := validResult()
	token := strings.Repeat("a", MaxCorrelationIDLength)
	result.Observations[0].Message = "accepted correlation " + token
	result.Observations[0].Evidence = []Evidence{{Key: "endpoint", Value: token}}
	clean, err := Sanitize(result)
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if strings.Contains(clean.Observations[0].Message, token) || strings.Contains(clean.Observations[0].Evidence[0].Value, token) {
		t.Fatalf("correlation token was not redacted: %+v", clean.Observations[0])
	}
}

func TestSanitizeCapsEvidence(t *testing.T) {
	result := validResult()
	for index := 0; index < MaxEvidencePerEdge+3; index++ {
		result.Observations[0].Evidence = append(result.Observations[0].Evidence, Evidence{Key: "endpoint", Value: "talaria"})
	}
	clean, err := Sanitize(result)
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if len(clean.Observations[0].Evidence) != 1 {
		t.Fatalf("deduplicated evidence count = %d, want 1", len(clean.Observations[0].Evidence))
	}
}

func TestSanitizePreservesOnlyValidatedWebhookEvidence(t *testing.T) {
	result := validResult()
	result.Observations[0].Evidence = []Evidence{
		{Key: "registration-fingerprint", Value: strings.Repeat("a", 64)},
		{Key: "http-status", Value: "202"},
		{Key: "correlation-state", Value: "recorded"},
		{Key: "participant-observed-at", Value: "2026-08-14T18:58:55Z"},
		{Key: "secret", Value: "never-emit"},
	}
	clean, err := Sanitize(result)
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if len(clean.Observations[0].Evidence) != 4 {
		t.Fatalf("evidence = %#v", clean.Observations[0].Evidence)
	}
}

func TestSanitizeRejectsExcessiveGraph(t *testing.T) {
	result := validResult()
	result.Nodes = make([]Node, MaxNodes+1)
	if _, err := Sanitize(result); err == nil || !strings.Contains(err.Error(), "structural limits") {
		t.Fatalf("expected structural limit error, got %v", err)
	}
}

func TestSanitizeCopiesParodusClientList(t *testing.T) {
	clients := []string{"apparmor-simulator"}
	truncated := false
	result := validResult()
	result.Journey = JourneyParodusClients
	result.Target = EndpointIdentity{Deployment: "edge", Service: "parodus", Type: "parodus", Replica: 0}
	result.Nodes = []Node{{ID: "gateway", Label: "Gateway", Kind: "service"}, {ID: "parodus", Label: "Parodus", Kind: "service"}}
	result.Edges = []Edge{{ID: "parodus-client-list", From: "gateway", To: "parodus", Label: "list registered clients", BlocksFollowing: true}}
	result.Observations = []Observation{{EdgeID: "parodus-client-list", State: StatePassed, ObservedAt: result.ObservedAt}}
	result.ParodusClients = &clients
	result.ParodusClientsTruncated = &truncated
	clean, err := Sanitize(result)
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	clients[0] = "changed"
	if clean.ParodusClients == nil || (*clean.ParodusClients)[0] != "apparmor-simulator" || clean.ParodusClientsTruncated == nil || *clean.ParodusClientsTruncated {
		t.Fatalf("clean result = %+v", clean)
	}
}

func TestSanitizeCopiesAndRedactsWebhookRegistrations(t *testing.T) {
	registration := validWebhookRegistration(strings.Repeat("a", 64))
	registration.EventFilters = []string{"devices/.*", "secret=private"}
	registrations := []WebhookRegistration{registration}
	result := validWebhookInventoryResult()
	result.WebhookRegistrations = &registrations
	clean, err := Sanitize(result)
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	registrations[0].EventFilters[0] = "changed"
	if clean.WebhookRegistrations == nil || strings.Contains(strings.Join((*clean.WebhookRegistrations)[0].EventFilters, ","), "private") || strings.Contains(strings.Join((*clean.WebhookRegistrations)[0].EventFilters, ","), "changed") {
		t.Fatalf("clean registrations = %+v", clean.WebhookRegistrations)
	}
}
