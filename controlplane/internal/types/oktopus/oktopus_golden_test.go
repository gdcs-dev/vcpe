package oktopus_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/oktopus"
)

func TestOktopusDefaultAddressingIsDHCP(t *testing.T) {
	oktopus.Register()
	st, ok := typeregistry.Lookup("oktopus")
	if !ok {
		t.Fatal("oktopus not registered")
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "oktopus",
		Type:  "oktopus",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/oktopus", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:0b", ManagedNetwork: true},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render oktopus: %v", err)
	}
	env := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			env = a.Content
		}
	}
	if !strings.Contains(env, "IFACE_MGMT_ADDRESSING=dhcp") {
		t.Fatalf("compose.env missing IFACE_MGMT_ADDRESSING=dhcp (default):\n%s", env)
	}
	if !strings.Contains(env, "IFACE_MGMT_NETWORK_MANAGED=1") {
		t.Fatalf("compose.env missing IFACE_MGMT_NETWORK_MANAGED=1 for a Podman-managed network:\n%s", env)
	}
}

func TestOktopusStaticAddressingPinsIPv4(t *testing.T) {
	oktopus.Register()
	st, _ := typeregistry.Lookup("oktopus")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "oktopus",
		Type:  "oktopus",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/oktopus", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:0b", IPv4: "10.10.10.7", Addressing: "static"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render oktopus: %v", err)
	}
	env, compose := "", ""
	for _, a := range result.Artifacts {
		switch a.Key {
		case "compose.env":
			env = a.Content
		case "compose.yaml":
			compose = a.Content
		}
	}
	if !strings.Contains(env, "IFACE_MGMT_ADDRESSING=static") {
		t.Fatalf("compose.env missing IFACE_MGMT_ADDRESSING=static:\n%s", env)
	}
	if !strings.Contains(compose, "ipv4_address: 10.10.10.7") {
		t.Fatalf("compose.yaml missing pinned ipv4_address:\n%s", compose)
	}
	if !strings.Contains(compose, "aliases") || !strings.Contains(compose, "- oktopus") {
		t.Fatalf("compose.yaml should register a bare \"oktopus\" mgmt network alias, got:\n%s", compose)
	}
}

func TestOktopusComposeVolumesAreManifestDriven(t *testing.T) {
	oktopus.Register()
	st, _ := typeregistry.Lookup("oktopus")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name: "oktopus", Type: "oktopus", Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/oktopus", Tag: "dev"},
		Volumes: []string{"./state:/var/lib/oktopus"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render oktopus: %v", err)
	}
	var compose string
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			compose = artifact.Content
		}
	}
	if !strings.Contains(compose, "- ./state:/var/lib/oktopus") {
		t.Fatalf("compose.yaml missing manifest volume:\n%s", compose)
	}
	for _, implicitVolume := range []string{"./runtime/mongo:/var/lib/mongodb", "./runtime/nats:/var/lib/nats/jetstream"} {
		if strings.Contains(compose, implicitVolume) {
			t.Errorf("compose.yaml contains implicit volume %q:\n%s", implicitVolume, compose)
		}
	}
}

// TestOktopusComposeAliasOnlyFirstInstance verifies the bare "oktopus" mgmt
// alias is registered once, on the first instance/replica only.
func TestOktopusComposeAliasOnlyFirstInstance(t *testing.T) {
	oktopus.Register()
	st, _ := typeregistry.Lookup("oktopus")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name: "oktopus", Type: "oktopus", Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/oktopus", Tag: "dev"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}},
			{Index: 1, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:02", IPv4: "10.0.0.3"}}},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000, 1: 47001}})
	if err != nil {
		t.Fatalf("render oktopus replicas: %v", err)
	}
	var compose string
	for _, a := range result.Artifacts {
		if a.Key == "compose.yaml" {
			compose = a.Content
		}
	}
	if strings.Count(compose, "aliases:") != 1 {
		t.Errorf("expected exactly one mgmt network alias block (first instance only), got:\n%s", compose)
	}
}
