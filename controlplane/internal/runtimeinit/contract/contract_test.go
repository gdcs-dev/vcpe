package contract_test

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/contract"
)

// TestContractMatchesPlannerIdentities asserts that the runtime-init startup
// contract carries byte-for-byte the same interface identities (device, MAC,
// addresses, gateways) the planner resolved, and that those MACs match a fresh
// CanonicalMAC computation. This guards the invariant that the planner and the
// runtime-init contract never diverge.
func TestContractMatchesPlannerIdentities(t *testing.T) {
	doc := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{
				{Role: "wan", IPv4: &manifest.AddressFamily{CIDR: "10.200.0.0/24", Gateway: "10.200.0.1"}},
				{Role: "lan", IPv4: &manifest.AddressFamily{CIDR: "10.210.0.0/24", Gateway: "10.210.0.1"}},
			},
			Services: []manifest.Service{
				{
					Name:     "bng",
					Type:     "bng",
					Replicas: 1,
					Image:    manifest.Image{Repository: "ghcr.io/gdcs-dev/bng", Tag: "dev"},
					Interfaces: []manifest.Interface{
						{Role: "wan", IPv4: "10.200.0.2", DefaultRoute: true},
						{Role: "lan"},
					},
				},
			},
		},
	}

	resolved, err := planner.Build(doc)
	if err != nil {
		t.Fatalf("planner build: %v", err)
	}

	contracts := contract.BuildForDeployment("op-test", resolved)
	doc0, ok := contracts["bng"]
	if !ok {
		t.Fatal("expected a bng contract")
	}
	if doc0.Deployment != "edge" {
		t.Fatalf("expected deployment edge, got %q", doc0.Deployment)
	}

	plannedIface := resolved.Services[0].Instances[0].Interfaces
	if len(doc0.Interfaces) != len(plannedIface) {
		t.Fatalf("interface count mismatch: contract %d, planner %d", len(doc0.Interfaces), len(plannedIface))
	}

	for i, binding := range doc0.Interfaces {
		want := plannedIface[i]
		if binding.Role != want.Role {
			t.Errorf("iface %d role: contract %q, planner %q", i, binding.Role, want.Role)
		}
		if binding.Name != want.Device {
			t.Errorf("iface %d device: contract %q, planner %q", i, binding.Name, want.Device)
		}
		if binding.MAC != want.MAC {
			t.Errorf("iface %d mac: contract %q, planner %q", i, binding.MAC, want.MAC)
		}
		// The MAC must equal a fresh CanonicalMAC for this single-replica key.
		expectMAC := plan.CanonicalMAC("edge", "bng", want.Role, 0)
		if binding.MAC != expectMAC {
			t.Errorf("iface %d mac %q does not match CanonicalMAC %q", i, binding.MAC, expectMAC)
		}
		if binding.Gateway4 != want.Gateway4 {
			t.Errorf("iface %d gateway4: contract %q, planner %q", i, binding.Gateway4, want.Gateway4)
		}
	}
}

// TestBridgeNameDeterminismMatchesPlanner asserts the planner derives the same
// bridge name DeriveBridgeName produces for the same (deployment, role) key.
func TestBridgeNameDeterminismMatchesPlanner(t *testing.T) {
	doc := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{
				{Role: "wan", IPv4: &manifest.AddressFamily{CIDR: "10.200.0.0/24"}},
			},
			Services: []manifest.Service{
				{Name: "bng", Type: "bng", Replicas: 1, Image: manifest.Image{Repository: "x/bng"}, Interfaces: []manifest.Interface{{Role: "wan"}}},
			},
		},
	}
	resolved, err := planner.Build(doc)
	if err != nil {
		t.Fatalf("planner build: %v", err)
	}
	want, _ := plan.DeriveBridgeName("edge", "wan")
	if got := resolved.Networks[0].Bridge; got != want {
		t.Fatalf("bridge name mismatch: planner %q, DeriveBridgeName %q", got, want)
	}
}
