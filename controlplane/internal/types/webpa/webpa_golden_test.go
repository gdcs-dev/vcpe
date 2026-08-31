package webpa_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/webpa"
	"gopkg.in/yaml.v3"
)

func TestWebPAGoldenComposeEnv(t *testing.T) {
	webpa.Register()
	st, ok := typeregistry.Lookup("webpa")
	if !ok {
		t.Fatal("webpa not registered")
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "webpa",
		Type:  "webpa",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/webpa", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:09", IPv4: "10.10.10.5", Gateway4: "10.10.10.1", Addressing: "static"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render webpa: %v", err)
	}

	artifacts := map[string]string{}
	for _, a := range result.Artifacts {
		artifacts[a.Key] = a.Content
	}

	// compose.env must contain all IFACE_* vars
	wantEnv := strings.Join([]string{
		"DEPLOYMENT_NAME=edge",
		"SERVICE_NAME=webpa",
		"IMAGE=ghcr.io/gdcs-dev/webpa:dev",
		"IFACE_MGMT_ADDRESSING=static",
		"IFACE_MGMT_BRIDGE=",
		"IFACE_MGMT_DEVICE=eth0",
		"IFACE_MGMT_GATEWAY4=10.10.10.1",
		"IFACE_MGMT_GATEWAY6=",
		"IFACE_MGMT_IPV4=10.10.10.5",
		"IFACE_MGMT_IPV6=",
		"IFACE_MGMT_MAC=02:00:00:00:00:09",
		"IFACE_MGMT_NETWORK=edge-mgmt",
	}, "\n") + "\n"
	if artifacts["compose.env"] != wantEnv {
		t.Fatalf("webpa compose.env mismatch:\n--- got ---\n%s\n--- want ---\n%s", artifacts["compose.env"], wantEnv)
	}

	// compose.yaml must contain the resolved mgmt network for its instance.
	composeYAML := artifacts["compose.yaml"]
	if composeYAML == "" {
		t.Fatal("webpa renderer did not produce compose.yaml")
	}
	if !strings.Contains(composeYAML, "name: edge-mgmt") {
		t.Errorf("compose.yaml should contain the resolved mgmt network, got:\n%s", composeYAML)
	}
	for _, host := range []string{"webpa", "consul", "talaria", "scytale", "tr1d1um", "argus", "caduceus", "petasos", "themis"} {
		if !strings.Contains(composeYAML, "- "+host) {
			t.Errorf("compose.yaml should register a %q mgmt network alias, got:\n%s", host, composeYAML)
		}
	}
}

// TestWebPADefaultAddressingIsDHCP verifies an interface with no explicit
// addressing/ipv4 resolves to dhcp and carries no pinned address.
func TestWebPADefaultAddressingIsDHCP(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "webpa",
		Type:  "webpa",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/webpa", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:09", ManagedNetwork: true},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render webpa: %v", err)
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
	if strings.Contains(env, "IFACE_MGMT_IPV4=10") {
		t.Fatalf("expected no pinned ipv4 in dhcp mode:\n%s", env)
	}
}

func TestWebPARendersRoutesToBNGConnectedNetworks(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")

	mgmt := plan.Network{Role: "mgmt", Bridge: "edge-mgmt", IPv4: &plan.Family{CIDR: "10.10.10.0/24", Gateway: "10.10.10.1"}}
	wan := plan.Network{Role: "wan", Bridge: "edge-wan", IPAMDriver: "none", IPv4: &plan.Family{CIDR: "10.7.200.0/24"}}
	cm := plan.Network{Role: "cm", Bridge: "edge-cm", IPAMDriver: "none", IPv4: &plan.Family{CIDR: "10.7.201.0/24"}}
	bngService := plan.Service{
		Name: "bng", Type: "bng",
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", IPv4: "10.10.10.10"},
			{Role: "wan", Network: "edge-wan", IPv4: "10.7.200.1"},
			{Role: "cm", Network: "edge-cm", IPv4: "10.7.201.1"},
		}}},
	}
	service := plan.Service{
		Name: "webpa", Type: "webpa", Image: manifest.Image{Repository: "example/webpa"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", IPv4: "10.10.10.11", ManagedNetwork: true}}}},
	}
	input := render.Input{Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{mgmt, wan, cm}, Services: []plan.Service{bngService, service}}, Service: service}

	result, err := st.Renderer().Render(context.Background(), input)
	if err != nil {
		t.Fatalf("render webpa: %v", err)
	}
	env := ""
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.env" {
			env = artifact.Content
		}
	}
	for _, want := range []string{
		"BNG_ROUTER_IPV4=10.10.10.10",
		"BNG_ROUTED_IPV4_CIDRS=10.7.200.0/24 10.7.201.0/24",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("compose.env missing %q:\n%s", want, env)
		}
	}
}

func TestWebPAWithoutEligibleBNGRendersNoRoutes(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")
	service := plan.Service{
		Name: "webpa", Type: "webpa", Image: manifest.Image{Repository: "example/webpa"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", IPv4: "10.10.10.11", ManagedNetwork: true}}}},
	}
	input := render.Input{Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt", IPv4: &plan.Family{CIDR: "10.10.10.0/24"}}, {Role: "wan", Bridge: "edge-wan", IPv4: &plan.Family{CIDR: "10.7.200.0/24"}}}, Services: []plan.Service{service}}, Service: service}

	result, err := st.Renderer().Render(context.Background(), input)
	if err != nil {
		t.Fatalf("render webpa: %v", err)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.env" && strings.Contains(artifact.Content, "BNG_ROUTE") {
			t.Fatalf("deployment without an eligible BNG must not render routes:\n%s", artifact.Content)
		}
	}
}

func TestWebPARejectsConfig(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")
	var cfg yaml.Node
	_ = yaml.Unmarshal([]byte("anything: 1\n"), &cfg)
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected webpa to reject any config")
	}
}

func TestWebPAComposeReplicasHaveDistinctHealthPorts(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")
	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name: "webpa", Type: "webpa", Image: manifest.Image{Repository: "webpa", Tag: "test"},
		Ports: []string{"8080:8080"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}},
			{Index: 1, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:02", IPv4: "10.0.0.3"}}},
		},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000, 1: 47001}})
	if err != nil {
		t.Fatalf("render webpa replicas: %v", err)
	}
	var compose string
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			compose = artifact.Content
		}
	}
	for _, want := range []string{"webpa-1:", "webpa-2:", "127.0.0.1:47000:9878", "127.0.0.1:47001:9878", "instances/1/compose.env", "instances/2/compose.env"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml missing %q:\n%s", want, compose)
		}
	}
	// Only the first instance should claim the bare "webpa" mgmt alias, to
	// avoid an ambiguous alias across replicas.
	if strings.Count(compose, "aliases:") != 1 {
		t.Errorf("expected exactly one mgmt network alias block (first instance only), got:\n%s", compose)
	}
}
