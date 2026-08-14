package diagnostic

import (
	"strings"
	"testing"
)

func TestRenderASCIIHealthyFailedAndInconclusive(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Result)
		content []string
	}{
		{name: "healthy", mutate: func(*Result) {}, content: []string{"--PASSED-->", "Result: passed"}},
		{name: "failed", mutate: func(result *Result) {
			result.Observations[1].State = StateFailed
			result.Observations[1].ReasonID = "talaria-dns-failed"
			result.Observations[1].RemediationID = "check-talaria-dns"
			result.Observations[1].Message = "Talaria hostname did not resolve"
			result.FirstFailure = result.Edges[1].ID
		}, content: []string{"--FAILED-->", "First failure: talaria-dns", "Remediation: check-talaria-dns"}},
		{name: "inconclusive", mutate: func(result *Result) {
			result.Edges[0].BlocksFollowing = false
			result.Observations[0].State = StateUnknown
			result.Observations[0].ReasonID = ReasonApplicationEvidenceUnavailable
		}, content: []string{"--UNKNOWN-->", "Inconclusive: app-parodus"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validResult()
			test.mutate(&result)
			output, err := RenderASCII(result)
			if err != nil {
				t.Fatalf("RenderASCII: %v", err)
			}
			for _, content := range test.content {
				if !strings.Contains(output, content) {
					t.Errorf("output missing %q:\n%s", content, output)
				}
			}
		})
	}
}

func TestRenderJSONUsesStableSchema(t *testing.T) {
	output, err := RenderJSON(validResult())
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	for _, content := range []string{`"schemaVersion": "vcpe.dev/diagnostic/v1"`, `"journey": "cpe-webpa"`, `"blocksFollowing": true`} {
		if !strings.Contains(output, content) {
			t.Errorf("JSON missing %q:\n%s", content, output)
		}
	}
}
