package planner_test

import (
	"reflect"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
)

func sampleDoc() manifest.Document {
	return manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{
				{Role: "wan", NAT: true, IPv4: &manifest.AddressFamily{CIDR: "10.200.0.0/24", Gateway: "10.200.0.1", Pool: &manifest.Pool{Start: "10.200.0.10", End: "10.200.0.20"}}},
				{Role: "lan", IPv4: &manifest.AddressFamily{CIDR: "10.210.0.0/24", Gateway: "10.210.0.1", Pool: &manifest.Pool{Start: "10.210.0.10", End: "10.210.0.20"}}},
			},
			Services: []manifest.Service{
				{Name: "webpa", Type: "webpa", Replicas: 1, Image: manifest.Image{Repository: "x/webpa"}, DependsOn: []string{"bng"}, Interfaces: []manifest.Interface{{Role: "lan"}}},
				{Name: "bng", Type: "bng", Replicas: 1, Image: manifest.Image{Repository: "x/bng"}, Interfaces: []manifest.Interface{{Role: "wan", DefaultRoute: true}, {Role: "lan"}}},
			},
		},
	}
}

// TestBuildIsDeterministic asserts repeated planning of the same manifest yields
// byte-for-byte identical resolved deployments (names, ordering, identities).
func TestBuildIsDeterministic(t *testing.T) {
	doc := sampleDoc()
	first, err := planner.Build(doc)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	for i := 0; i < 5; i++ {
		next, err := planner.Build(doc)
		if err != nil {
			t.Fatalf("build %d: %v", i, err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("planner output is non-deterministic on build %d", i)
		}
	}
}

// TestDependsOnOrdering asserts dependencies start before dependents.
func TestDependsOnOrdering(t *testing.T) {
	resolved, err := planner.Build(sampleDoc())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pos := map[string]int{}
	for i, svc := range resolved.Services {
		pos[svc.Name] = i
	}
	if pos["bng"] > pos["webpa"] {
		t.Fatalf("expected bng (dependency) before webpa (dependent); got bng=%d webpa=%d", pos["bng"], pos["webpa"])
	}
}

// TestDeviceAndGatewayAssignment asserts ordered eth<n> devices and inherited
// gateways from the network.
func TestDeviceAndGatewayAssignment(t *testing.T) {
	resolved, err := planner.Build(sampleDoc())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var ifaces []struct {
		Device   string
		Gateway4 string
	}
	for _, svc := range resolved.Services {
		if svc.Name != "bng" {
			continue
		}
		for _, in := range svc.Instances[0].Interfaces {
			ifaces = append(ifaces, struct {
				Device   string
				Gateway4 string
			}{in.Device, in.Gateway4})
		}
	}
	if len(ifaces) < 2 {
		t.Fatalf("expected 2 bng interfaces, got %d", len(ifaces))
	}
	if ifaces[0].Device != "eth0" || ifaces[1].Device != "eth1" {
		t.Fatalf("expected ordered eth devices, got %q,%q", ifaces[0].Device, ifaces[1].Device)
	}
	if ifaces[0].Gateway4 != "10.200.0.1" {
		t.Fatalf("expected wan gateway inherited, got %q", ifaces[0].Gateway4)
	}
}
