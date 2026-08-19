package diagnostic

import (
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

func resolverStore(t *testing.T, services string) *persist.Store {
	t.Helper()
	store, err := persist.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	document := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata: {name: edge}\nspec:\n  networks:\n    - role: wan\n      ipv4: {cidr: 10.0.0.0/24}\n  services:\n" + services
	if err := store.SaveDesiredSnapshot("edge", []byte(document)); err != nil {
		t.Fatal(err)
	}
	return store
}

func resolverRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(NewCPEWebPAProvider("gateway"))
	registry.Register(NewCPEWebPAProvider("xb10"))
	registry.Register(NewCPEWebPACallbackProvider())
	registry.Register(NewParodusClientsProvider())
	registry.Register(NewArgusWebhooksProvider())
	registry.Register(NewWebhookProvider())
	return registry
}

func TestResolveSingleReplicaAndEndpoint(t *testing.T) {
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	endpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Resolve(store, resolverRegistry(), ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Endpoint != endpoint || selection.Instance.Index != 0 || selection.Target.Name != "webpa" {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestResolveParodusClientsWithoutWebPA(t *testing.T) {
	store := resolverStore(t, "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n")
	endpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Resolve(store, resolverRegistry(), ResolveRequest{Deployment: "edge", Source: "gateway", Target: "parodus"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Provider.Journey() != JourneyParodusClients || selection.Endpoint != endpoint || selection.Target.Name != "parodus" || selection.Target.Type != "parodus" || selection.TargetEndpoint != (persist.HealthEndpoint{}) {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestResolveArgusWebhooksFromWebPA(t *testing.T) {
	store := resolverStore(t, "    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n")
	endpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Resolve(store, resolverRegistry(), ResolveRequest{Deployment: "edge", Source: "webpa", Target: "webhooks"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Provider.Journey() != JourneyArgusWebhooks || selection.Endpoint != endpoint || selection.Target.Name != "argus" || selection.Target.Type != "argus" || selection.TargetEndpoint != (persist.HealthEndpoint{}) {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestResolveErrors(t *testing.T) {
	base := "    - name: gateway\n      type: gateway\n      replicas: 2\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n"
	tests := []struct {
		name     string
		services string
		request  ResolveRequest
		want     string
	}{
		{name: "unknown source", services: base, request: ResolveRequest{Deployment: "edge", Source: "missing", Target: "webpa"}, want: "no source service"},
		{name: "replica required", services: base, request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa"}, want: "--replica is required"},
		{name: "replica range", services: base, request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", Replica: intPointer(3)}, want: "out of range"},
		{name: "unsupported source", services: strings.Replace(base, "type: gateway", "type: generic-container", 1), request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", Replica: intPointer(0)}, want: "unsupported type"},
		{name: "missing target", services: strings.Replace(base, "type: webpa", "type: bng", 1), request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", Replica: intPointer(0)}, want: "no service of type webpa"},
		{name: "ambiguous target", services: base + "    - name: webpa-two\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n", request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "webpa", Replica: intPointer(0)}, want: "ambiguous"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := resolverStore(t, test.services)
			_, err := Resolve(store, resolverRegistry(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestResolveWebhookParticipantsAndEndpoints(t *testing.T) {
	services := "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n"
	store := resolverStore(t, services)
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Resolve(store, resolverRegistry(), ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Provider.Journey() != JourneyWebhook || selection.Endpoint != subscriberEndpoint || selection.TargetEndpoint != webpaEndpoint {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestResolveWebhookErrors(t *testing.T) {
	base := "    - name: event-sink\n      type: event-sink\n      replicas: 1\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n"
	tests := []struct {
		name         string
		services     string
		reserveWebPA bool
		want         string
	}{
		{name: "missing webpa endpoint", services: base, want: "webpa service \"webpa\" replica 0 has no persisted loopback endpoint"},
		{name: "unsupported subscriber", services: strings.Replace(base, "type: event-sink", "type: generic-container", 1), reserveWebPA: true, want: "unsupported type"},
		{name: "ambiguous webpa service", services: base + "    - name: webpa-two\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n", reserveWebPA: true, want: "ambiguous webpa targets"},
		{name: "ambiguous webpa replica", services: strings.Replace(base, "replicas: 1\n      image: {repository: webpa}", "replicas: 2\n      image: {repository: webpa}", 1), reserveWebPA: true, want: "exactly one WebPA participant"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := resolverStore(t, test.services)
			if _, err := store.ReserveHealthEndpoint("edge", "event-sink", 0); err != nil {
				t.Fatal(err)
			}
			if test.reserveWebPA {
				if _, err := store.ReserveHealthEndpoint("edge", "webpa", 0); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Resolve(store, resolverRegistry(), ResolveRequest{Deployment: "edge", Source: "event-sink", Target: "webhook"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestResolveCallbackParticipantsAndEndpoints(t *testing.T) {
	services := "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 2\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n"
	store := resolverStore(t, services)
	gatewayEndpoint, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	webpaEndpoint, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatal(err)
	}
	subscriberEndpoint, err := store.ReserveHealthEndpoint("edge", "event-sink", 1)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Resolve(store, resolverRegistry(), ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", Subscriber: "event-sink", SubscriberReplica: intPointer(1)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Provider.Journey() != JourneyCPEWebPACallback || selection.Endpoint != gatewayEndpoint || selection.TargetEndpoint != webpaEndpoint || selection.SubscriberEndpoint != subscriberEndpoint || selection.SubscriberInstance.Index != 1 {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestResolveCallbackErrors(t *testing.T) {
	base := "    - name: gateway\n      type: gateway\n      replicas: 1\n      interfaces: [{role: wan}]\n      image: {repository: gateway}\n    - name: event-sink\n      type: event-sink\n      replicas: 2\n      interfaces: [{role: mgmt}]\n      image: {repository: event-sink}\n    - name: webpa\n      type: webpa\n      replicas: 1\n      image: {repository: webpa}\n"
	tests := []struct {
		name     string
		services string
		request  ResolveRequest
		want     string
	}{
		{name: "missing subscriber", services: base, request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback"}, want: "requires a subscriber"},
		{name: "unknown subscriber", services: base, request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", Subscriber: "missing"}, want: "no subscriber service"},
		{name: "unsupported subscriber", services: strings.Replace(base, "type: event-sink", "type: generic-container", 1), request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", Subscriber: "event-sink"}, want: "unsupported type"},
		{name: "subscriber replica required", services: base, request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", Subscriber: "event-sink"}, want: "--subscriber-replica is required"},
		{name: "subscriber replica range", services: base, request: ResolveRequest{Deployment: "edge", Source: "gateway", Target: "callback", Subscriber: "event-sink", SubscriberReplica: intPointer(2)}, want: "out of range"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := resolverStore(t, test.services)
			if _, err := store.ReserveHealthEndpoint("edge", "gateway", 0); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ReserveHealthEndpoint("edge", "webpa", 0); err != nil {
				t.Fatal(err)
			}
			_, err := Resolve(store, resolverRegistry(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func intPointer(value int) *int { return &value }
