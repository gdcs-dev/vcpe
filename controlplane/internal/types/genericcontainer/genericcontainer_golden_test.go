package genericcontainer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/genericcontainer"
	"gopkg.in/yaml.v3"
)

func TestGenericContainerGeneratesComposeAndEnv(t *testing.T) {
	genericcontainer.Register()
	st, ok := typeregistry.Lookup("generic-container")
	if !ok {
		t.Fatal("generic-container not registered")
	}

	var cfg yaml.Node
	src := "command: [/bin/sleep, infinity]\nenv: { FOO: bar }\nports: [\"8080:80\"]\n"
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "lan", Bridge: "edge-lan"}}}
	svc := plan.Service{
		Name:   "client",
		Type:   "generic-container",
		Image:  manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render generic-container: %v", err)
	}

	artifacts := map[string]string{}
	for _, a := range result.Artifacts {
		artifacts[a.Key] = a.Content
	}

	env := artifacts["compose.env"]
	compose := artifacts["compose.yaml"]
	entrypoint := artifacts["entrypoint.sh"]

	if env == "" || compose == "" {
		t.Fatalf("expected both compose.env and compose.yaml; got env=%q compose=%q", env, compose)
	}
	if entrypoint == "" {
		t.Fatal("expected entrypoint.sh artifact")
	}
	if _, ok := artifacts["resolv.conf"]; ok {
		t.Fatal("unexpected resolv.conf artifact: renderer should not emit resolv.conf")
	}

	for _, frag := range []string{"DEPLOYMENT_NAME=edge", "SERVICE_NAME=client", "FOO=bar"} {
		if !strings.Contains(env, frag) {
			t.Fatalf("compose.env missing %q:\n%s", frag, env)
		}
	}
	for _, frag := range []string{
		"client-1:",
		"image: docker.io/library/alpine:3.19",
		"8080:80",
		"entrypoint.sh:/run/vcpe/entrypoint.sh:ro",
		"entrypoint:",
	} {
		if !strings.Contains(compose, frag) {
			t.Fatalf("compose.yaml missing %q:\n%s", frag, compose)
		}
	}
	if strings.Contains(compose, "resolv.conf") {
		t.Fatalf("compose.yaml must not reference resolv.conf:\n%s", compose)
	}
	if !strings.Contains(entrypoint, "exec \"$@\"") {
		t.Fatalf("entrypoint.sh missing exec:\n%s", entrypoint)
	}
}

// TestGenericContainerStaticAddressing verifies that an interface with
// addressing: static is reflected in compose.env and the entrypoint applies it
// without running a DHCP client, replacing the former VCPE_INIT_STATIC_ROLE
// config.env convention.
func TestGenericContainerStaticAddressing(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")

	var cfg yaml.Node
	src := "command: [\"/bin/sh\"]\n"
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:   "client",
		Type:   "generic-container",
		Image:  manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a",
				IPv4: "192.168.1.10/24", Gateway4: "192.168.1.1", DefaultRoute: true, Addressing: "static"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	artifacts := map[string]string{}
	for _, a := range result.Artifacts {
		artifacts[a.Key] = a.Content
	}

	if artifacts["entrypoint.sh"] == "" {
		t.Fatal("expected entrypoint.sh")
	}
	if _, ok := artifacts["resolv.conf"]; ok {
		t.Fatal("unexpected resolv.conf artifact")
	}
	env := artifacts["compose.env"]
	if !strings.Contains(env, "IFACE_LAN_DEFAULT_ROUTE=1") {
		t.Fatalf("compose.env missing IFACE_LAN_DEFAULT_ROUTE=1:\n%s", env)
	}
	if !strings.Contains(env, "IFACE_LAN_ADDRESSING=static") {
		t.Fatalf("compose.env missing IFACE_LAN_ADDRESSING=static:\n%s", env)
	}
	if strings.Contains(env, "VCPE_INIT_STATIC_ROLE") {
		t.Fatalf("VCPE_INIT_STATIC_ROLE is superseded by addressing and must not appear:\n%s", env)
	}
}

// TestGenericContainerMultipleInterfacesEachInitialized verifies every
// declared interface is reflected with its own resolved addressing, not just
// a single default-route interface.
func TestGenericContainerMultipleInterfacesEachInitialized(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")

	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:  "client",
		Type:  "generic-container",
		Image: manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "wan", Device: "eth0", MAC: "02:00:00:00:00:0a", DefaultRoute: true},
			{Role: "lan-p1", Device: "eth1", MAC: "02:00:00:00:00:0b",
				IPv4: "10.0.0.5/24", Gateway4: "10.0.0.1", Addressing: "static"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	env := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			env = a.Content
		}
	}
	if !strings.Contains(env, "IFACE_WAN_ADDRESSING=dhcp") {
		t.Fatalf("compose.env missing IFACE_WAN_ADDRESSING=dhcp (default):\n%s", env)
	}
	if !strings.Contains(env, "IFACE_LAN_P1_ADDRESSING=static") {
		t.Fatalf("compose.env missing IFACE_LAN_P1_ADDRESSING=static:\n%s", env)
	}
}

// TestGenericContainerInitVarsPassThrough verifies that VCPE_INIT_* identity vars
// in config.env appear in compose.env and are not consumed at render time.
func TestGenericContainerInitVarsPassThrough(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")

	var cfg yaml.Node
	src := "env:\n  VCPE_INIT_HOSTNAME: phone-01\n  VCPE_INIT_MAC_ROLE: lan\n  VCPE_INIT_SLEEP: \"2\"\n  DEVICE_SERIAL: XB-001\n"
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:   "client",
		Type:   "generic-container",
		Image:  manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Device: "eth0", MAC: "02:00:00:00:00:0a"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	env := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			env = a.Content
		}
	}
	for _, want := range []string{
		"VCPE_INIT_HOSTNAME=phone-01",
		"VCPE_INIT_MAC_ROLE=lan",
		"VCPE_INIT_SLEEP=2",
		"DEVICE_SERIAL=XB-001",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("compose.env missing %q:\n%s", want, env)
		}
	}
}

func TestGenericContainerRejectsUnknownConfig(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")
	var cfg yaml.Node
	_ = yaml.Unmarshal([]byte("notafield: 1\n"), &cfg)
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestGenericContainerHealthConfigValidation(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")
	for _, testCase := range []struct {
		name    string
		config  string
		wantErr bool
	}{
		{name: "HTTP probe", config: "health: { http: { url: http://service:8080/ready }, timeoutSeconds: 2 }"},
		{name: "command probe", config: "health: { command: { command: test -f /ready }, timeoutSeconds: 2 }"},
		{name: "missing probe", config: "health: { timeoutSeconds: 2 }", wantErr: true},
		{name: "ambiguous probe", config: "health: { http: { url: http://service/ }, command: { command: true }, timeoutSeconds: 2 }", wantErr: true},
		{name: "unbounded timeout", config: "health: { command: { command: true }, timeoutSeconds: 31 }", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var config yaml.Node
			if err := yaml.Unmarshal([]byte(testCase.config), &config); err != nil {
				t.Fatal(err)
			}
			node := config
			if config.Kind == yaml.DocumentNode {
				node = *config.Content[0]
			}
			err := st.ValidateConfig(node)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("ValidateConfig() error = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}

func TestGenericContainerRendersConfiguredHealthSidecar(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("health: { http: { url: http://127.0.0.1:8080/ready, expectedStatus: 204 }, timeoutSeconds: 3 }"), &config); err != nil {
		t.Fatal(err)
	}
	service := plan.Service{
		Name: "client", Type: "generic-container", Image: manifest.Image{Repository: "example/client", Tag: "test"}, Config: config,
		Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a"}}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	artifacts := map[string]string{}
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	if _, ok := artifacts["vcpe-healthd.required"]; !ok {
		t.Fatal("expected vcpe-healthd staging marker")
	}
	for _, expected := range []string{
		"client-health-1:",
		"network_mode: service:client-1",
		"127.0.0.1:47000:9878",
		"configured=http://127.0.0.1:8080/ready|204",
	} {
		if !strings.Contains(artifacts["compose.yaml"], expected) {
			t.Fatalf("compose.yaml missing %q:\n%s", expected, artifacts["compose.yaml"])
		}
	}
}

func TestGenericContainerWithoutHealthHasNoEndpoint(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("command: [sleep, infinity]"), &config); err != nil {
		t.Fatal(err)
	}
	service := plan.Service{
		Name: "client", Type: "generic-container", Image: manifest.Image{Repository: "example/client", Tag: "test"}, Config: config,
		Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a"}}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Key == "vcpe-healthd.required" {
			t.Fatal("unconfigured service must not stage vcpe-healthd")
		}
		if artifact.Key == "compose.yaml" && strings.Contains(artifact.Content, "47000:9878") {
			t.Fatalf("unconfigured service must not publish a health endpoint:\n%s", artifact.Content)
		}
	}
}

// TestGenericContainerProbeHelperInheritsWorkloadTransport verifies the
// namespace-sharing probe helper carries only network_mode: it must not
// declare its own networks or ports, since it inherits both the managed
// health attachment and published endpoint from the workload it shares a
// namespace with.
func TestGenericContainerProbeHelperInheritsWorkloadTransport(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("health: { http: { url: http://127.0.0.1:8080/ready, expectedStatus: 204 }, timeoutSeconds: 3 }"), &config); err != nil {
		t.Fatal(err)
	}
	service := plan.Service{
		Name: "client", Type: "generic-container", Image: manifest.Image{Repository: "example/client", Tag: "test"}, Config: config,
		Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a"}}}},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service, HealthPorts: map[int]int{0: 47000}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var compose string
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			compose = artifact.Content
		}
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("parse compose.yaml: %v", err)
	}
	services, ok := document["services"].(map[string]any)
	if !ok {
		t.Fatalf("compose.yaml services must be a mapping, got %#v", document["services"])
	}
	probe, ok := services["client-health-1"].(map[string]any)
	if !ok {
		t.Fatalf("missing probe helper service, got %#v", services)
	}
	if probe["network_mode"] != "service:client-1" {
		t.Errorf("probe network_mode = %#v, want service:client-1", probe["network_mode"])
	}
	if _, ok := probe["networks"]; ok {
		t.Errorf("probe helper must not declare its own networks entry, got %#v", probe["networks"])
	}
	if _, ok := probe["ports"]; ok {
		t.Errorf("probe helper must not declare its own ports entry, got %#v", probe["ports"])
	}
	workload, ok := services["client-1"].(map[string]any)
	if !ok {
		t.Fatalf("missing workload service, got %#v", services)
	}
	if _, ok := workload["networks"]; !ok {
		t.Errorf("workload service missing its own networks entry: %#v", workload)
	}
	if _, ok := workload["ports"]; !ok {
		t.Errorf("workload service missing its own published ports: %#v", workload)
	}
}
