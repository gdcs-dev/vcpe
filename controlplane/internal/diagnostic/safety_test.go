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

func TestSanitizeRejectsExcessiveGraph(t *testing.T) {
	result := validResult()
	result.Nodes = make([]Node, MaxNodes+1)
	if _, err := Sanitize(result); err == nil || !strings.Contains(err.Error(), "structural limits") {
		t.Fatalf("expected structural limit error, got %v", err)
	}
}
