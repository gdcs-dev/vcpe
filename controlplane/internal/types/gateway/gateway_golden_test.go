package gateway_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/gateway"
	"gopkg.in/yaml.v3"
)

func TestGATEWAYGoldenComposeEnv(t *testing.T) {
	gateway.Register()
	st, ok := typeregistry.Lookup("gateway")
	if !ok {
		t.Fatal("gateway not registered")
	}

	var cfg yaml.Node
	if err := yaml.Unmarshal([]byte("lan: { ipv4: 192.168.0.1, ipv6: \"fd00::1\" }\nerouter: { vlan: 100 }\n"), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "lan", Bridge: "edge-lan"}}}
	svc := plan.Service{
		Name:   "gateway",
		Type:   "gateway",
		Image:  manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:01"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	got := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			got = a.Content
		}
	}
	for _, frag := range []string{
		"DEPLOYMENT_NAME=edge",
		"SERVICE_NAME=gateway",
		"IFACE_LAN_DEVICE=eth0",
		"LAN_IPV4=192.168.0.1",
		"LAN_IPV6=fd00::1",
		"EROUTER_VLAN=100",
	} {
		if !strings.Contains(got, frag) {
			t.Fatalf("gateway compose.env missing %q in:\n%s", frag, got)
		}
	}
}

func TestGATEWAYRejectsUnknownConfigField(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	var cfg yaml.Node
	_ = yaml.Unmarshal([]byte("bogus: true\n"), &cfg)
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestGatewayPublishesHealthDirectlyWithoutManagedTopology(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0", IPv4: "10.7.200.10", Addressing: "static"},
		}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	compose := composeArtifact(t, result)
	for _, want := range []string{"127.0.0.1:47000:9878", "aa-health:"} {
		if !strings.Contains(compose, want) {
			t.Fatalf("expected direct health publication to contain %q:\n%s", want, compose)
		}
	}
	if strings.Contains(compose, "vcpe-healthd") || strings.Contains(compose, "gateway-1-health") {
		t.Fatalf("expected no per-instance health proxy service:\n%s", compose)
	}
}

func TestGatewayPublishesHealthDirectlyWithManagedTopology(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0"},
		}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	compose := composeArtifact(t, result)
	if !strings.Contains(compose, "127.0.0.1:47000:9878") {
		t.Fatalf("expected the workload's own health mapping:\n%s", compose)
	}
	if strings.Contains(compose, "aa-health:") || strings.Contains(compose, "vcpe-healthd") {
		t.Fatalf("expected no private health network or proxy when the topology attachment is already Podman-managed:\n%s", compose)
	}
}

func composeArtifact(t *testing.T, result render.Result) string {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			return artifact.Content
		}
	}
	t.Fatal("no compose.yaml artifact produced")
	return ""
}

func TestGatewayAddressingReflectedInEnv(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan"}, {Role: "cm", Bridge: "edge-cm"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0", IPv4: "10.7.200.10", Gateway4: "10.7.200.1", Addressing: "static"},
			{Role: "cm", Network: "edge-cm", Device: "wan0"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render gateway: %v", err)
	}
	got := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			got = a.Content
		}
	}
	if !strings.Contains(got, "IFACE_WAN_ADDRESSING=static") {
		t.Fatalf("compose.env missing IFACE_WAN_ADDRESSING=static:\n%s", got)
	}
	if !strings.Contains(got, "IFACE_CM_ADDRESSING=dhcp") {
		t.Fatalf("compose.env missing IFACE_CM_ADDRESSING=dhcp (default):\n%s", got)
	}
}

// TestGatewayDHCPAddressingCoexistsWithHealthPublication verifies a dhcp-
// addressed interface and direct health publication coexist without
// conflict, since publication forwards through the managed aa-health
// attachment by loopback port rather than by this interface's resolved
// address.
func TestGatewayDHCPAddressingCoexistsWithHealthPublication(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway with dhcp addressing: %v", err)
	}
	compose := composeArtifact(t, result)
	if !strings.Contains(compose, "aa-health:") {
		t.Fatalf("expected direct health publication even with addressing: dhcp on the self-addressed interface:\n%s", compose)
	}
}
