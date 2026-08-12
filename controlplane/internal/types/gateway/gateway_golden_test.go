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

func TestGatewayRendersHealthSidecarOnlyWhenHealthUpstreamDeclared(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan"}}}

	base := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
	}

	withoutUpstream := base
	withoutUpstream.Instances = []plan.Instance{{Interfaces: []plan.Interface{
		{Role: "wan", Network: "edge-wan", Device: "erouter0", IPv4: "10.7.200.10", Addressing: "static"},
	}}}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: withoutUpstream, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render without healthUpstream: %v", err)
	}
	compose := composeArtifact(t, result)
	if strings.Contains(compose, "vcpe-healthd") {
		t.Fatalf("expected no health sidecar without a declared healthUpstream interface:\n%s", compose)
	}

	withUpstream := base
	withUpstream.Instances = []plan.Instance{{Interfaces: []plan.Interface{
		{Role: "wan", Network: "edge-wan", Device: "erouter0", IPv4: "10.7.200.10", Addressing: "static", HealthUpstream: true},
	}}}
	result, err = st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: withUpstream, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render with healthUpstream: %v", err)
	}
	compose = composeArtifact(t, result)
	for _, want := range []string{"vcpe-healthd", "--proxy-url", "http://gateway-1:9878/health", "--timeout", "10s", "127.0.0.1:47000:9878", "aa-health:"} {
		if !strings.Contains(compose, want) {
			t.Fatalf("expected health sidecar to contain %q:\n%s", want, compose)
		}
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

// TestGatewayHealthUpstreamWithDHCPAddressing verifies healthUpstream and a
// dhcp-addressed interface coexist without conflict, since the health sidecar
// dials the workload by service name over a dedicated network rather than by
// this interface's resolved address.
func TestGatewayHealthUpstreamWithDHCPAddressing(t *testing.T) {
	gateway.Register()
	st, _ := typeregistry.Lookup("gateway")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan"}}}

	svc := plan.Service{
		Name:  "gateway",
		Type:  "gateway",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/gateway", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "erouter0", HealthUpstream: true},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render gateway with dhcp + healthUpstream: %v", err)
	}
	compose := composeArtifact(t, result)
	if !strings.Contains(compose, "vcpe-healthd") {
		t.Fatalf("expected health sidecar even with addressing: dhcp on the healthUpstream interface:\n%s", compose)
	}
}
