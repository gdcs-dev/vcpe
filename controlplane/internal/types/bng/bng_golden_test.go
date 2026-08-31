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
					{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:aa:bb:cc:dd:ee", IPv4: "10.200.0.2", Gateway4: "10.200.0.1", Addressing: "static"},
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
		"IFACE_WAN_ADDRESSING=static",
		"IFACE_WAN_BRIDGE=",
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

// TestBNGRendererUsesResolvedDevice asserts the renderer pulls addresses from
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

// TestBNGDnsmasqResolvesWebPAByInstanceAlias asserts that the WebPA peer
// hostname and virtual-host CNAMEs target the actual compose/aardvark
// instance alias (e.g. "webpa-1"), not the bare manifest service name, which
// curated renderers no longer register as a network alias.
func TestBNGDnsmasqResolvesWebPAByInstanceAlias(t *testing.T) {
	bng.Register()
	st, _ := typeregistry.Lookup("bng")
	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "mgmt", Bridge: "edge-mgmt", IPv4: &plan.Family{CIDR: "10.10.10.0/24", Gateway: "10.10.10.1"}},
		},
		Services: []plan.Service{
			{
				Name: "webpa",
				Type: "webpa",
				Instances: []plan.Instance{
					{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", IPv4: "10.10.10.11"}}},
				},
			},
		},
	}
	svc := plan.Service{
		Name:      "bng",
		Type:      "bng",
		Image:     manifest.Image{Repository: "x/bng"},
		Config:    bngConfigNode(t),
		Instances: []plan.Instance{{Interfaces: []plan.Interface{{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:00:00:00:00:01"}}}},
	}
	dep.Services = append(dep.Services, svc)
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	hosts, ok := artifact(result, "etc/dnsmasq.hosts")
	if !ok {
		t.Fatal("expected etc/dnsmasq.hosts")
	}
	if !strings.Contains(hosts, "10.10.10.11 webpa-1\n") {
		t.Fatalf("expected dnsmasq.hosts to key WebPA by instance alias webpa-1, got:\n%s", hosts)
	}
	if strings.Contains(hosts, " webpa\n") {
		t.Fatalf("dnsmasq.hosts should not use the bare service name, got:\n%s", hosts)
	}

	conf, ok := artifact(result, "etc/dnsmasq.conf")
	if !ok {
		t.Fatal("expected etc/dnsmasq.conf")
	}
	for _, want := range []string{"cname=talaria,webpa-1", "cname=webpa,webpa-1", "cname=talaria.dns.podman,webpa-1"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("expected dnsmasq.conf to contain %q, got:\n%s", want, conf)
		}
	}
	for _, want := range []string{
		"addn-hosts=/etc/dnsmasq.management.hosts",
		"addn-hosts=/etc/dnsmasq.dhcp.hosts",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("expected dnsmasq.conf to contain %q, got:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "addn-hosts=/etc/dnsmasq.hosts") || strings.Contains(conf, "dnsmasq.dynamic.hosts") {
		t.Fatalf("dnsmasq.conf must not serve the management seed or removed shared dynamic file:\n%s", conf)
	}
	for _, key := range []string{"etc/dnsmasq.management.hosts", "etc/dnsmasq.dhcp.hosts"} {
		content, ok := artifact(result, key)
		if !ok || content != "" {
			t.Fatalf("expected empty separately-owned artifact %q, got present=%t content=%q", key, ok, content)
		}
	}
	if _, ok := artifact(result, "etc/dnsmasq.dynamic.hosts"); ok {
		t.Fatal("removed shared dynamic hosts artifact must not be rendered")
	}
	if strings.Contains(conf, ",webpa\n") {
		t.Fatalf("dnsmasq.conf cname targets should not use the bare service name, got:\n%s", conf)
	}
}

// TestBNGRendersWithDHCPAddressingOnItsOwnServerRole documents that the
// control plane does not guard against setting addressing: dhcp on an
// interface BNG itself serves DHCP for — rendering succeeds regardless
// (a misconfiguration here is left to fail at runtime, not validation).
func TestBNGRendersWithDHCPAddressingOnItsOwnServerRole(t *testing.T) {
	bng.Register()
	st, _ := typeregistry.Lookup("bng")
	dep := plan.Deployment{
		Name:     "edge",
		Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan", IPv4: &plan.Family{CIDR: "10.200.0.0/24", Gateway: "10.200.0.1"}}},
	}
	svc := plan.Service{
		Name:   "bng",
		Type:   "bng",
		Image:  manifest.Image{Repository: "ghcr.io/gdcs-dev/bng", Tag: "dev"},
		Config: bngConfigNode(t),
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:aa:bb:cc:dd:ee", Addressing: "dhcp"},
		}}},
	}
	if _, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc}); err != nil {
		t.Fatalf("expected bng to render even with addressing: dhcp on its own DHCP-server role, got %v", err)
	}
}

func TestBNGRendersDHCPLeaseCallbacksAndPlannedAliases(t *testing.T) {
	bng.Register()
	st, _ := typeregistry.Lookup("bng")
	var config yaml.Node
	if err := yaml.Unmarshal([]byte(`
access:
  - role: wan
    dhcp4:
      subnet: 10.7.200.0/24
      ranges: [{start: 10.7.200.100, end: 10.7.200.200}]
  - role: cm
    dhcp4:
      subnet: 10.7.201.0/24
      ranges: [{start: 10.7.201.100, end: 10.7.201.200}]
`), &config); err != nil {
		t.Fatal(err)
	}
	config = *config.Content[0]

	bngService := plan.Service{
		Name:   "bng",
		Type:   "bng",
		Image:  manifest.Image{Repository: "x/bng"},
		Config: config,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.10.10.10"},
			{Role: "wan", Network: "edge-wan", MAC: "02:00:00:00:00:02", IPv4: "10.7.200.1"},
			{Role: "cm", Network: "edge-cm", MAC: "02:00:00:00:00:03", IPv4: "10.7.201.1"},
		}}},
	}
	xb10Service := plan.Service{
		Name: "xb10", Type: "xb10", Replicas: 2,
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{
				{Role: "wan", Network: "edge-wan", MAC: "02:00:00:00:10:01", DefaultRoute: true},
				{Role: "cm", Network: "edge-cm", MAC: "02:00:00:00:10:02"},
				{Role: "lan", Network: "edge-lan", MAC: "02:00:00:00:10:03"},
			}},
			{Index: 1, Interfaces: []plan.Interface{
				{Role: "wan", Network: "edge-wan", MAC: "02:00:00:00:20:01", DefaultRoute: true},
				{Role: "cm", Network: "edge-cm", MAC: "02:00:00:00:20:02"},
			}},
		},
	}
	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "mgmt", Bridge: "edge-mgmt"},
			{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none"},
			{Role: "cm", Bridge: "edge-cm", IPAMDriver: "none"},
			{Role: "lan", Bridge: "edge-lan", IPAMDriver: "none"},
		},
		Services: []plan.Service{bngService, xb10Service},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: bngService})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	dhcpd, _ := artifact(result, "etc/dhcp/dhcpd.conf")
	for _, want := range []string{"on commit", "on release", "on expiry", "/etc/dhcpd-notify.sh"} {
		if !strings.Contains(dhcpd, want) {
			t.Errorf("dhcpd.conf missing %q:\n%s", want, dhcpd)
		}
	}
	mapping, ok := artifact(result, "etc/dnsmasq.dhcp-hosts.map")
	if !ok {
		t.Fatal("expected DHCP identity map")
	}
	for _, want := range []string{
		"02:00:00:00:10:01 xb10 xb10-1 xb10-1-wan",
		"02:00:00:00:10:02 xb10-1-cm",
		"02:00:00:00:20:01 xb10-2 xb10-2-wan",
		"02:00:00:00:20:02 xb10-2-cm",
	} {
		if !strings.Contains(mapping, want+"\n") {
			t.Errorf("identity map missing %q:\n%s", want, mapping)
		}
	}
	if strings.Contains(mapping, "02:00:00:00:10:03") {
		t.Errorf("identity map published interface outside BNG DHCP access roles:\n%s", mapping)
	}
}

func TestBNGRejectsDuplicateGeneratedDHCPAlias(t *testing.T) {
	bng.Register()
	st, _ := typeregistry.Lookup("bng")
	bngService := plan.Service{
		Name: "bng", Type: "bng", Image: manifest.Image{Repository: "x/bng"}, Config: bngConfigNode(t),
		Instances: []plan.Instance{{Interfaces: []plan.Interface{{Role: "wan", Network: "edge-wan", Device: "wan0", MAC: "02:00:00:00:00:01", IPv4: "10.200.0.1"}}}},
	}
	dep := plan.Deployment{
		Name:     "edge",
		Networks: []plan.Network{{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none"}},
		Services: []plan.Service{
			bngService,
			{Name: "device", Type: "gateway", Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{Role: "wan", MAC: "02:00:00:00:10:01"}}}}},
			{Name: "device-1", Type: "gateway", Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{Role: "wan", MAC: "02:00:00:00:20:01"}}}}},
		},
	}

	_, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: bngService})
	if err == nil || !strings.Contains(err.Error(), "duplicate DHCP DNS alias") || !strings.Contains(err.Error(), "device-1") {
		t.Fatalf("expected contextual duplicate alias error, got %v", err)
	}
}
