package planner_test

import (
	"reflect"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
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
	first, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	for i := 0; i < 5; i++ {
		next, err := planner.Build(doc, nil)
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
	resolved, err := planner.Build(sampleDoc(), nil)
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
	resolved, err := planner.Build(sampleDoc(), nil)
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

// TestSingleReplicaUsesIndexedName asserts that a single-replica service uses
// the {service}-1 compose service name (1-based indexed) rather than the bare
// {service} name, so names are stable when replicas later increases.
func TestSingleReplicaUsesIndexedName(t *testing.T) {
	doc := sampleDoc()
	resolved, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, svc := range resolved.Services {
		if svc.Replicas != 1 {
			continue
		}
		if len(svc.Instances) != 1 {
			t.Fatalf("service %q: expected 1 instance, got %d", svc.Name, len(svc.Instances))
		}
		if svc.Instances[0].Index != 0 {
			t.Fatalf("service %q: expected instance index 0, got %d", svc.Name, svc.Instances[0].Index)
		}
		// Index is 0-based internally; external name uses Index+1.
		// Confirm the delta for a fresh deploy: PreviousReplicaCount=0, Replicas=1.
		if want := (plan.ReplicaDelta{ToAdd: []int{0}}); !reflect.DeepEqual(svc.Delta, want) {
			t.Fatalf("service %q: expected delta %v, got %v", svc.Name, want, svc.Delta)
		}
	}
}

// TestReplicaDeltaScaleUp asserts PreviousReplicaCount=1, Replicas=2 produces
// ToAdd=[1], ToRemove=[].
func TestReplicaDeltaScaleUp(t *testing.T) {
	doc := sampleDoc()
	// Override bng to 2 replicas (no explicit interfaces needed for delta test).
	for i, svc := range doc.Spec.Services {
		if svc.Name == "bng" {
			doc.Spec.Services[i].Replicas = 2
		}
	}
	prev := map[string]int{"bng": 1, "webpa": 1}
	resolved, err := planner.Build(doc, prev)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, svc := range resolved.Services {
		if svc.Name != "bng" {
			continue
		}
		want := plan.ReplicaDelta{ToAdd: []int{1}, ToRemove: nil}
		if !reflect.DeepEqual(svc.Delta, want) {
			t.Fatalf("scale-up delta: want %+v, got %+v", want, svc.Delta)
		}
	}
}

// TestReplicaDeltaScaleDown asserts PreviousReplicaCount=3, Replicas=1 produces
// ToAdd=[], ToRemove=[2,1] (highest index first).
func TestReplicaDeltaScaleDown(t *testing.T) {
	doc := sampleDoc()
	for i, svc := range doc.Spec.Services {
		if svc.Name == "bng" {
			doc.Spec.Services[i].Replicas = 1
		}
	}
	prev := map[string]int{"bng": 3, "webpa": 1}
	resolved, err := planner.Build(doc, prev)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, svc := range resolved.Services {
		if svc.Name != "bng" {
			continue
		}
		want := plan.ReplicaDelta{ToAdd: nil, ToRemove: []int{2, 1}}
		if !reflect.DeepEqual(svc.Delta, want) {
			t.Fatalf("scale-down delta: want %+v, got %+v", want, svc.Delta)
		}
	}
}

// TestReplicaDeltaNoChange asserts PreviousReplicaCount=2, Replicas=2 produces
// an empty delta.
func TestReplicaDeltaNoChange(t *testing.T) {
	doc := sampleDoc()
	for i, svc := range doc.Spec.Services {
		if svc.Name == "bng" {
			doc.Spec.Services[i].Replicas = 2
		}
	}
	prev := map[string]int{"bng": 2, "webpa": 1}
	resolved, err := planner.Build(doc, prev)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, svc := range resolved.Services {
		if svc.Name != "bng" {
			continue
		}
		want := plan.ReplicaDelta{}
		if !reflect.DeepEqual(svc.Delta, want) {
			t.Fatalf("no-change delta: want %+v, got %+v", want, svc.Delta)
		}
	}
}

// TestReplicaDeltaFirstDeploy asserts PreviousReplicaCount=0, Replicas=2
// produces ToAdd=[0,1], ToRemove=[].
func TestReplicaDeltaFirstDeploy(t *testing.T) {
	doc := sampleDoc()
	for i, svc := range doc.Spec.Services {
		if svc.Name == "bng" {
			doc.Spec.Services[i].Replicas = 2
		}
	}
	// No previous counts (nil) = all zeros.
	resolved, err := planner.Build(doc, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, svc := range resolved.Services {
		if svc.Name != "bng" {
			continue
		}
		want := plan.ReplicaDelta{ToAdd: []int{0, 1}, ToRemove: nil}
		if !reflect.DeepEqual(svc.Delta, want) {
			t.Fatalf("first-deploy delta: want %+v, got %+v", want, svc.Delta)
		}
	}
}
