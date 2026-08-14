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

func intPointer(value int) *int { return &value }
