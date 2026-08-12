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
}
