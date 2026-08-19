package diagnostic

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

var updateDiagnosticGoldens = flag.Bool("update", false, "update diagnostic golden files")

func TestRenderASCIIHealthyFailedAndInconclusive(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Result)
		content []string
	}{
		{name: "healthy", mutate: func(*Result) {}, content: []string{"CPE to WebPA diagnostic: edge/gateway[0] -> webpa", "--PASSED-->", "Result: passed"}},
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

func TestRenderParodusClientList(t *testing.T) {
	clients := []string{"apparmor-simulator", "config"}
	truncated := false
	result := validResult()
	result.Journey = JourneyParodusClients
	result.Target = EndpointIdentity{Deployment: "edge", Service: "parodus", Type: "parodus", Replica: 0}
	result.Nodes = []Node{{ID: "gateway", Label: "Gateway", Kind: "service"}, {ID: "parodus", Label: "Parodus", Kind: "service"}}
	result.Edges = []Edge{{ID: "parodus-client-list", From: "gateway", To: "parodus", Label: "list registered clients", BlocksFollowing: true}}
	result.Observations = []Observation{{EdgeID: "parodus-client-list", State: StatePassed, ObservedAt: result.ObservedAt}}
	result.ParodusClients = &clients
	result.ParodusClientsTruncated = &truncated
	output, err := RenderASCII(result)
	if err != nil {
		t.Fatalf("RenderASCII: %v", err)
	}
	for _, content := range []string{"Parodus client enumeration: edge/gateway[0] -> parodus", "Registered clients:", "apparmor-simulator", "config", "Truncated: false"} {
		if !strings.Contains(output, content) {
			t.Errorf("output missing %q:\n%s", content, output)
		}
	}
	jsonOutput, err := RenderJSON(result)
	if err != nil || !strings.Contains(jsonOutput, `"parodusClients": [`) || !strings.Contains(jsonOutput, `"parodusClientsTruncated": false`) {
		t.Fatalf("RenderJSON() = %q, %v", jsonOutput, err)
	}
}

func TestRenderXB10ParodusClientList(t *testing.T) {
	clients := []string{"apparmor-simulator"}
	truncated := false
	result := validResult()
	result.Journey = JourneyParodusClients
	result.Source = EndpointIdentity{Deployment: "edge", Service: "xb10", Type: "xb10", Replica: 0}
	result.Target = EndpointIdentity{Deployment: "edge", Service: "parodus", Type: "parodus", Replica: 0}
	result.Nodes = []Node{{ID: "xb10", Label: "XB10", Kind: "service"}, {ID: "parodus", Label: "Parodus", Kind: "service"}}
	result.Edges = []Edge{{ID: "parodus-client-list", From: "xb10", To: "parodus", Label: "list registered clients", BlocksFollowing: true}}
	result.Observations = []Observation{{EdgeID: "parodus-client-list", State: StatePassed, ObservedAt: result.ObservedAt}}
	result.ParodusClients = &clients
	result.ParodusClientsTruncated = &truncated
	output, err := RenderASCII(result)
	if err != nil {
		t.Fatalf("RenderASCII: %v", err)
	}
	if !strings.Contains(output, "Parodus client enumeration: edge/xb10[0] -> parodus") || !strings.Contains(output, "apparmor-simulator") {
		t.Fatalf("output = %q", output)
	}
}

func TestRenderWebhookInventory(t *testing.T) {
	first := validWebhookRegistration(strings.Repeat("a", 64))
	second := validWebhookRegistration(strings.Repeat("b", 64))
	second.CallbackURL = "http://other/webhook"
	registrations := []WebhookRegistration{first, second}
	result := validWebhookInventoryResult()
	result.WebhookRegistrations = &registrations
	output, err := RenderASCII(result)
	if err != nil {
		t.Fatalf("RenderASCII: %v", err)
	}
	for _, content := range []string{"Argus webhook inventory: edge/webpa[0] -> argus", "Registered webhooks:", first.Fingerprint, second.Fingerprint, "Callback URL: http://event-sink/webhook", "Secret configured: true"} {
		if !strings.Contains(output, content) {
			t.Errorf("output missing %q:\n%s", content, output)
		}
	}
	jsonOutput, err := RenderJSON(result)
	if err != nil || !strings.Contains(jsonOutput, `"webhookRegistrations": [`) {
		t.Fatalf("RenderJSON() = %q, %v", jsonOutput, err)
	}
}

func TestRenderTalariaDeviceInventory(t *testing.T) {
	first := validTalariaDevice("mac:001122334455")
	second := validTalariaDevice("mac:001122334456")
	devices := []TalariaDevice{first, second}
	result := validTalariaDeviceInventoryResult()
	result.TalariaDevices = &devices
	output, err := RenderASCII(result)
	if err != nil {
		t.Fatalf("RenderASCII: %v", err)
	}
	for _, content := range []string{"Talaria connected-device inventory: edge/webpa[0] -> talaria", "Connected devices:", first.ID, "Pending: 1", "Bytes sent: 2", "Uptime: 1m2s"} {
		if !strings.Contains(output, content) {
			t.Errorf("output missing %q:\n%s", content, output)
		}
	}
	jsonOutput, err := RenderJSON(result)
	if err != nil || !strings.Contains(jsonOutput, `"talariaDevices": [`) || !strings.Contains(jsonOutput, `"bytesSent": 2`) {
		t.Fatalf("RenderJSON() = %q, %v", jsonOutput, err)
	}

	empty := validTalariaDeviceInventoryResult()
	emptyDevices := []TalariaDevice{}
	empty.TalariaDevices = &emptyDevices
	emptyOutput, err := RenderASCII(empty)
	if err != nil || !strings.Contains(emptyOutput, "Connected devices:\n  (none)") {
		t.Fatalf("empty RenderASCII() = %q, %v", emptyOutput, err)
	}
}

func TestRenderWebhookEvidence(t *testing.T) {
	result := validResult()
	result.Observations[0].State = StateFailed
	result.Observations[0].Evidence = []Evidence{{Key: "http-status", Value: "401"}, {Key: "correlation-state", Value: "rejected"}}
	result.Observations[1].State = StateSkipped
	result.FirstFailure = result.Edges[0].ID
	output, err := RenderASCII(result)
	if err != nil {
		t.Fatalf("RenderASCII: %v", err)
	}
	for _, content := range []string{"Evidence: http-status=401", "Evidence: correlation-state=rejected"} {
		if !strings.Contains(output, content) {
			t.Errorf("output missing %q:\n%s", content, output)
		}
	}
}

func TestRenderWebhookGoldens(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{
			name: "passive-inconclusive",
			mutate: func(result *Result) {
				result.Observations[6].State = StateUnknown
				result.Observations[6].ReasonID = ReasonActiveCallbackNotRequested
				result.Observations[6].RemediationID = RemediationAllowActiveCallback
				result.Observations[6].Message = "active callback was not requested"
			},
		},
		{
			name:   "healthy-active",
			mutate: func(*Result) {},
		},
		{
			name: "registration-failure",
			mutate: func(result *Result) {
				result.Observations[3].State = StateFailed
				result.Observations[3].ReasonID = ReasonRegistrationMissing
				result.Observations[3].RemediationID = RemediationRegisterWebhook
				result.Observations[3].Message = "no matching webhook registration was found"
			},
		},
		{
			name: "direct-callback-failure",
			mutate: func(result *Result) {
				result.Observations[8].State = StateFailed
				result.Observations[8].ReasonID = ReasonCallbackRejected
				result.Observations[8].RemediationID = RemediationCheckCallbackSignature
				result.Observations[8].Message = "callback rejected the diagnostic marker"
				result.Observations[8].Evidence = []Evidence{{Key: "http-status", Value: "401"}, {Key: "correlation-state", Value: "rejected"}}
			},
		},
		{
			name: "caduceus-delivery-failure",
			mutate: func(result *Result) {
				result.Observations[10].State = StateFailed
				result.Observations[10].ReasonID = ReasonCaduceusReceiptMissing
				result.Observations[10].RemediationID = RemediationCheckCaduceusDelivery
				result.Observations[10].Message = "Caduceus accepted the event but no callback receipt was recorded"
				result.Observations[10].Evidence = []Evidence{{Key: "correlation-state", Value: "missing"}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := webhookGoldenResult(t)
			test.mutate(&result)
			observations, firstFailure, err := ApplyCausality(result.Edges, result.Observations, result.ObservedAt)
			if err != nil {
				t.Fatalf("ApplyCausality: %v", err)
			}
			result.Observations = observations
			result.FirstFailure = firstFailure
			if err := result.Validate(); err != nil {
				t.Fatalf("invalid webhook fixture: %v", err)
			}

			ascii, err := RenderASCII(result)
			if err != nil {
				t.Fatalf("RenderASCII: %v", err)
			}
			checkDiagnosticGolden(t, test.name+".ascii", ascii)

			json, err := RenderJSON(result)
			if err != nil {
				t.Fatalf("RenderJSON: %v", err)
			}
			checkDiagnosticGolden(t, test.name+".json", json)
		})
	}
}

func TestRenderCallbackGoldens(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{name: "callback-success", mutate: func(*Result) {}},
		{name: "callback-cpe-failure", mutate: func(result *Result) {
			result.Observations[0] = Observation{EdgeID: "application-parodus", State: StateFailed, ReasonID: "parodus-client-offline", RemediationID: "start-parodus", Message: "Parodus client is offline", ObservedAt: result.ObservedAt}
		}},
		{name: "callback-routing-inconclusive", mutate: func(result *Result) {
			result.Observations[12] = Observation{EdgeID: "routing-observation", State: StateUnknown, ReasonID: ReasonRoutingObservationUnavailable, RemediationID: RemediationCheckRoutingObservation, Message: "Caduceus routing observation was unavailable", ObservedAt: result.ObservedAt}
		}},
		{name: "callback-receipt-timeout", mutate: func(result *Result) {
			result.Observations[13] = Observation{EdgeID: "callback-receipt", State: StateUnknown, ReasonID: ReasonCaduceusReceiptMissing, RemediationID: RemediationCheckCaduceusDelivery, Message: "subscriber callback receipt was unavailable before the deadline", ObservedAt: result.ObservedAt}
		}},
		{name: "callback-receipt-restarted", mutate: func(result *Result) {
			result.Observations[13] = Observation{EdgeID: "callback-receipt", State: StateUnknown, ReasonID: ReasonReceiptRestarted, RemediationID: RemediationRestartSubscriber, Message: "subscriber receipt state was lost during restart", ObservedAt: result.ObservedAt}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := callbackGoldenResult(t)
			test.mutate(&result)
			observations, firstFailure, err := ApplyCausality(result.Edges, result.Observations, result.ObservedAt)
			if err != nil {
				t.Fatalf("ApplyCausality: %v", err)
			}
			result.Observations = observations
			result.FirstFailure = firstFailure
			ascii, err := RenderASCII(result)
			if err != nil {
				t.Fatalf("RenderASCII: %v", err)
			}
			checkDiagnosticGolden(t, test.name+".ascii", ascii)
			json, err := RenderJSON(result)
			if err != nil {
				t.Fatalf("RenderJSON: %v", err)
			}
			checkDiagnosticGolden(t, test.name+".json", json)
		})
	}
}

func callbackGoldenResult(t *testing.T) Result {
	t.Helper()
	graph, err := NewCPEWebPACallbackProvider().Expected(ExpectedInput{
		Deployment: plan.Deployment{Name: "edge"},
		Source:     plan.Service{Name: "gateway", Type: "gateway"},
		Instance:   plan.Instance{Index: 0},
		Target:     plan.Service{Name: "webpa", Type: "webpa"},
		Subscriber: plan.Service{Name: "event-sink", Type: "event-sink"},
	})
	if err != nil {
		t.Fatalf("callback provider graph: %v", err)
	}
	observedAt := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	result := Result{SchemaVersion: SchemaVersion, Journey: graph.Journey, Source: graph.Source, Target: graph.Target, Metadata: graph.Metadata, Nodes: graph.Nodes, Edges: graph.Edges, ObservedAt: observedAt, Observations: make([]Observation, len(graph.Edges))}
	for index, edge := range graph.Edges {
		result.Observations[index] = Observation{EdgeID: edge.ID, State: StatePassed, ObservedAt: observedAt}
	}
	return result
}

func webhookGoldenResult(t *testing.T) Result {
	t.Helper()
	graph, err := NewWebhookProvider().Expected(ExpectedInput{
		Deployment: plan.Deployment{Name: "edge"},
		Source:     plan.Service{Name: "event-sink", Type: "event-sink"},
		Instance:   plan.Instance{Index: 0},
		Target:     plan.Service{Name: "webpa", Type: "webpa"},
	})
	if err != nil {
		t.Fatalf("webhook provider graph: %v", err)
	}
	observedAt := time.Date(2026, 8, 14, 23, 45, 0, 0, time.UTC)
	result := Result{
		SchemaVersion: SchemaVersion,
		Journey:       graph.Journey,
		Source:        graph.Source,
		Target:        graph.Target,
		Metadata:      graph.Metadata,
		Nodes:         graph.Nodes,
		Edges:         graph.Edges,
		ObservedAt:    observedAt,
		Observations:  make([]Observation, len(graph.Edges)),
	}
	for index, edge := range graph.Edges {
		result.Observations[index] = Observation{EdgeID: edge.ID, State: StatePassed, ObservedAt: observedAt}
	}
	return result
}

func checkDiagnosticGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateDiagnosticGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Fatalf("golden file %s not found; run: go test ./internal/diagnostic -run TestRenderWebhookGoldens -update", path)
	}
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s:\ngot:\n%s\nwant:\n%s", name, got, string(want))
	}
}
