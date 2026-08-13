package eventsink_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/eventsink"
)

func TestEventSinkDefaultAddressingIsDHCP(t *testing.T) {
	eventsink.Register()
	st, ok := typeregistry.Lookup("event-sink")
	if !ok {
		t.Fatal("event-sink not registered")
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "event-sink",
		Type:  "event-sink",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/event-sink", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:0a", ManagedNetwork: true},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render event-sink: %v", err)
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

func TestEventSinkStaticAddressingPinsIPv4(t *testing.T) {
	eventsink.Register()
	st, _ := typeregistry.Lookup("event-sink")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "event-sink",
		Type:  "event-sink",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/event-sink", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:0a", IPv4: "10.10.10.6", Addressing: "static"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render event-sink: %v", err)
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
	if !strings.Contains(compose, "ipv4_address: 10.10.10.6") {
		t.Fatalf("compose.yaml missing pinned ipv4_address:\n%s", compose)
	}
	if !strings.Contains(compose, "aliases") || !strings.Contains(compose, "- event-sink") {
		t.Fatalf("compose.yaml should register a bare \"event-sink\" mgmt network alias, got:\n%s", compose)
	}
}

// TestEventSinkComposeAliasOnlyFirstInstance verifies the bare "event-sink"
// mgmt alias is registered once, on the first instance/replica only.
func TestEventSinkComposeAliasOnlyFirstInstance(t *testing.T) {
	eventsink.Register()
	st, _ := typeregistry.Lookup("event-sink")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name: "event-sink", Type: "event-sink", Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/event-sink", Tag: "dev"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}},
			{Index: 1, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:02", IPv4: "10.0.0.3"}}},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000, 1: 47001}})
	if err != nil {
		t.Fatalf("render event-sink replicas: %v", err)
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

// TestEventSinkComposeGrantsNetworkCapabilities verifies the rendered
// workload requests the privileges its entrypoint needs (ip link/dhclient),
// matching every other renderer that manages its own interfaces.
func TestEventSinkComposeGrantsNetworkCapabilities(t *testing.T) {
	eventsink.Register()
	st, _ := typeregistry.Lookup("event-sink")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "event-sink",
		Type:  "event-sink",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/event-sink", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:0a", ManagedNetwork: true},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render event-sink: %v", err)
	}
	compose := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.yaml" {
			compose = a.Content
		}
	}
	for _, want := range []string{"privileged: true", "NET_ADMIN", "NET_RAW"} {
		if !strings.Contains(compose, want) {
			t.Fatalf("compose.yaml missing %q:\n%s", want, compose)
		}
	}
}
