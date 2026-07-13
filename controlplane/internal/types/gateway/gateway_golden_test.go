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
