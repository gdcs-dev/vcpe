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
