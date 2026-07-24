package bng_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/bng"
	"gopkg.in/yaml.v3"
)

func bngConfigNode(t *testing.T) yaml.Node {
	t.Helper()
	const cfg = `
access:
  - role: wan
    dhcp4:
      subnet: 10.200.0.0/24
      ranges:
        - { start: 10.200.0.100, end: 10.200.0.200 }
      options:
        routers: 10.200.0.1
      leaseSeconds: 3600
    dhcp6:
      subnet: 2001:dae:7:1::/64
      ranges:
        - { start: "2001:dae:7:1::1000", end: "2001:dae:7:1::2000" }
    radvd:
      prefix: 2001:dae:7:1::/64
      advManagedFlag: true
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(cfg), &node); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	// yaml.Unmarshal wraps in a document node; descend to the mapping.
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		return *node.Content[0]
	}
	return node
}

func renderBNG(t *testing.T) render.Result {
	t.Helper()
	bng.Register()

	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "wan", Bridge: "edge-wan", IPv4: &plan.Family{CIDR: "10.200.0.0/24", Gateway: "10.200.0.1"}},
		},
	}
	svc := plan.Service{
		Name:   "bng",
		Type:   "bng",
		Image:  manifest.Image{Repository: "ghcr.io/gdcs-dev/bng", Tag: "dev"},
		Config: bngConfigNode(t),
		Instances: []plan.Instance{
			{
				Index: 0,
				Interfaces: []plan.Interface{
					{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:aa:bb:cc:dd:ee", IPv4: "10.200.0.2", Gateway4: "10.200.0.1"},
				},
			},
		},
	}

	st, ok := typeregistry.Lookup("bng")
	if !ok {
		t.Fatal("bng type not registered")
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render bng: %v", err)
	}
	return result
}

func artifact(result render.Result, key string) (string, bool) {
	for _, a := range result.Artifacts {
		if a.Key == key {
			return a.Content, true
		}
	}
	return "", false
}

func TestBNGGoldenComposeEnv(t *testing.T) {
	result := renderBNG(t)
	got, ok := artifact(result, "compose.env")
	if !ok {
		t.Fatal("expected compose.env artifact")
	}
	want := strings.Join([]string{
		"DEPLOYMENT_NAME=edge",
		"SERVICE_NAME=bng",
		"IMAGE=ghcr.io/gdcs-dev/bng:dev",
		"IFACE_WAN_DEVICE=eth0",
		"IFACE_WAN_GATEWAY4=10.200.0.1",
		"IFACE_WAN_GATEWAY6=",
		"IFACE_WAN_IPV4=10.200.0.2",
		"IFACE_WAN_IPV6=",
		"IFACE_WAN_MAC=02:aa:bb:cc:dd:ee",
		"IFACE_WAN_NETWORK=edge-wan",
		"WAN_IPV4_CIDR=10.200.0.2/24",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("compose.env mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBNGGoldenDHCPAndRADVD(t *testing.T) {
	result := renderBNG(t)

	dhcpd, ok := artifact(result, "etc/dhcp/dhcpd.conf")
	if !ok {
		t.Fatal("expected etc/dhcp/dhcpd.conf")
	}
	for _, frag := range []string{
		"subnet 10.200.0.0 netmask 255.255.255.0 {",
		"range 10.200.0.100 10.200.0.200;",
		"option routers 10.200.0.1;",
		"default-lease-time 3600;",
	} {
		if !strings.Contains(dhcpd, frag) {
			t.Fatalf("dhcpd.conf missing %q in:\n%s", frag, dhcpd)
		}
	}

	dhcpd6, ok := artifact(result, "etc/dhcp/dhcpd6.conf")
	if !ok {
		t.Fatal("expected etc/dhcp/dhcpd6.conf")
	}
	if !strings.Contains(dhcpd6, "subnet6 2001:dae:7:1::/64 {") {
		t.Fatalf("dhcpd6.conf missing subnet6 block:\n%s", dhcpd6)
	}

	radvd, ok := artifact(result, "etc/radvd.conf")
	if !ok {
		t.Fatal("expected etc/radvd.conf")
	}
	for _, frag := range []string{"interface eth0 {", "AdvSendAdvert on;", "AdvManagedFlag on;", "prefix 2001:dae:7:1::/64 {"} {
		if !strings.Contains(radvd, frag) {
			t.Fatalf("radvd.conf missing %q in:\n%s", frag, radvd)
		}
	}
}

// TestBNGRendererHasNoEmbeddedLiterals asserts the renderer pulls addresses from
// the typed config and interfaces, not from hardcoded customer literals. We
// assert the device comes from the resolved interface (changing it changes the
// radvd output).
func TestBNGRendererUsesResolvedDevice(t *testing.T) {
	bng.Register()
	st, _ := typeregistry.Lookup("bng")
	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan"}}}
	svc := plan.Service{
		Name:      "bng",
		Type:      "bng",
		Image:     manifest.Image{Repository: "x/bng"},
		Config:    bngConfigNode(t),
		Instances: []plan.Instance{{Interfaces: []plan.Interface{{Role: "wan", Network: "edge-wan", Device: "wan99", MAC: "02:00:00:00:00:01"}}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	radvd, _ := artifact(result, "etc/radvd.conf")
	if !strings.Contains(radvd, "interface wan99 {") {
		t.Fatalf("expected radvd to use resolved device wan99, got:\n%s", radvd)
	}
}
