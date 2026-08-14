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
	if _, ok := registry.Lookup(JourneyCPEWebPA, "gateway", "webpa"); !ok {
		t.Fatal("gateway provider not found")
	}
	want := []string{"cpe-webpa/gateway/webpa", "cpe-webpa/xb10/webpa"}
	if got := registry.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
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

func TestApplicationEvidenceUnavailableIsUnknown(t *testing.T) {
	observation := ApplicationEvidenceUnavailable(time.Now().UTC(), "active")
	if observation.State != StateUnknown || observation.ReasonID != ReasonApplicationEvidenceUnavailable {
		t.Fatalf("observation = %+v", observation)
	}
}
