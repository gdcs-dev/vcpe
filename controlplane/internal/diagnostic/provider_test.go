package diagnostic

import (
	"reflect"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

func TestRegistryLookupAndOrdering(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewCPEWebPAProvider("xb10"))
	registry.Register(NewCPEWebPAProvider("gateway"))
	registry.Register(NewCPEWebPACallbackProvider())
	registry.Register(NewParodusClientsProvider())
	registry.Register(NewWebhookProvider())
	if _, ok := registry.Lookup(JourneyCPEWebPA, "gateway", "webpa"); !ok {
		t.Fatal("gateway provider not found")
	}
	if _, ok := registry.Lookup(JourneyWebhook, "event-sink", "webpa"); !ok {
		t.Fatal("webhook provider not found")
	}
	if _, ok := registry.Lookup(JourneyCPEWebPACallback, "gateway", "webpa"); !ok {
		t.Fatal("callback provider not found")
	}
	if _, ok := registry.Lookup(JourneyParodusClients, "gateway", "parodus"); !ok {
		t.Fatal("Parodus client-list provider not found")
	}
	want := []string{"cpe-webpa-callback/gateway/webpa", "cpe-webpa/gateway/webpa", "cpe-webpa/xb10/webpa", "parodus-clients/gateway/parodus", "webhook/event-sink/webpa"}
	if got := registry.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
}

func TestParodusClientsProviderContract(t *testing.T) {
	provider := NewParodusClientsProvider()
	graph, err := provider.Expected(ExpectedInput{
		Deployment: plan.Deployment{Name: "edge"},
		Source:     plan.Service{Name: "gateway", Type: "gateway"},
		Instance:   plan.Instance{Index: 0},
		Target:     plan.Service{Name: "parodus", Type: "parodus"},
	})
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	if graph.Journey != JourneyParodusClients || graph.Target.Service != "parodus" || len(graph.Edges) != 1 || graph.Edges[0].ID != "parodus-client-list" {
		t.Fatalf("graph = %+v", graph)
	}
}

func TestWebhookProviderContract(t *testing.T) {
	provider := NewWebhookProvider()
	graph, err := provider.Expected(ExpectedInput{
		Deployment: plan.Deployment{Name: "edge"},
		Source:     plan.Service{Name: "events", Type: "event-sink"},
		Instance:   plan.Instance{Index: 1},
		Target:     plan.Service{Name: "webpa", Type: "webpa"},
	})
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	wantIDs := []string{
		"subscriber-intent", "argus-reachability", "argus-authentication", "registration-present", "registration-fresh", "registration-conformant", "callback-dns", "callback-transport", "callback-acceptance", "caduceus-ingestion", "caduceus-receipt",
	}
	if len(graph.Edges) != len(wantIDs) {
		t.Fatalf("edge count = %d, want %d", len(graph.Edges), len(wantIDs))
	}
	for index, edge := range graph.Edges {
		if edge.ID != wantIDs[index] || !edge.BlocksFollowing {
			t.Fatalf("edge %d = %+v, want ID %q and blocking", index, edge, wantIDs[index])
		}
	}
	if graph.Journey != JourneyWebhook || graph.Source.Replica != 1 || graph.Target.Type != "webpa" {
		t.Fatalf("unexpected graph identities: %+v", graph)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewCPEWebPAProvider("gateway"))
	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate registration panic")
		}
	}()
	registry.Register(NewCPEWebPAProvider("gateway"))
}

func TestCPEWebPAProviderContract(t *testing.T) {
	for _, sourceType := range []string{"gateway", "xb10"} {
		t.Run(sourceType, func(t *testing.T) {
			provider := NewCPEWebPAProvider(sourceType)
			graph, err := provider.Expected(ExpectedInput{
				Deployment: plan.Deployment{Name: "edge"},
				Source:     plan.Service{Name: "device", Type: sourceType},
				Instance: plan.Instance{Index: 0, Interfaces: []plan.Interface{
					{Role: "wan", Device: "erouter0", MAC: "02:00:00:00:00:01"},
				}},
				Target: plan.Service{Name: "webpa", Type: "webpa"},
			})
			if err != nil {
				t.Fatalf("Expected: %v", err)
			}
			if len(graph.Edges) != 5 || graph.Edges[0].ID != "application-parodus" || graph.Edges[4].ID != "device-registration" {
				t.Fatalf("unexpected ordered edges: %#v", graph.Edges)
			}
			if graph.Source.Type != sourceType || graph.Target.Type != "webpa" {
				t.Fatalf("unexpected endpoint identities: %+v -> %+v", graph.Source, graph.Target)
			}
			if got := graph.Metadata[2]; got.Key != "device-id" || got.Value != "mac:020000000001" {
				t.Fatalf("device metadata = %+v", got)
			}
		})
	}
}

func TestCPEWebPACallbackProviderContract(t *testing.T) {
	provider := NewCPEWebPACallbackProvider()
	graph, err := provider.Expected(ExpectedInput{
		Deployment: plan.Deployment{Name: "edge"},
		Source:     plan.Service{Name: "gateway", Type: "gateway"},
		Instance:   plan.Instance{Index: 0},
		Target:     plan.Service{Name: "webpa", Type: "webpa"},
		Subscriber: plan.Service{Name: "event-sink", Type: "event-sink"},
	})
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	wantIDs := []string{
		"application-parodus", "talaria-dns", "talaria-transport", "talaria-authentication", "device-registration", "subscriber-intent", "argus-reachability", "argus-authentication", "registration-present", "registration-fresh", "registration-conformant", "active-event-acceptance", "routing-observation", "callback-receipt",
	}
	if graph.Journey != JourneyCPEWebPACallback || len(graph.Edges) != len(wantIDs) {
		t.Fatalf("unexpected callback graph: %+v", graph)
	}
	for index, edge := range graph.Edges {
		if edge.ID != wantIDs[index] || !edge.BlocksFollowing {
			t.Fatalf("edge %d = %+v, want %q and blocking", index, edge, wantIDs[index])
		}
	}
	if _, err := provider.Expected(ExpectedInput{Source: plan.Service{Type: "xb10"}, Target: plan.Service{Type: "webpa"}, Subscriber: plan.Service{Type: "event-sink"}}); err == nil {
		t.Fatal("XB10 callback provider unexpectedly supported")
	}
}

func TestApplicationEvidenceUnavailableIsUnknown(t *testing.T) {
	observation := ApplicationEvidenceUnavailable(time.Now().UTC(), "active")
	if observation.State != StateUnknown || observation.ReasonID != ReasonApplicationEvidenceUnavailable {
		t.Fatalf("observation = %+v", observation)
	}
}
