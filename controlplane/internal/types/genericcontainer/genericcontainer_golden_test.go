package genericcontainer_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
		"instances/1/compose.env",
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

func TestGenericContainerRendersRadioEnvPerReplica(t *testing.T) {
	genericcontainer.Register()
	serviceType, _ := typeregistry.Lookup("generic-container")
	deployment := plan.Deployment{
		Name: "edge",
		WirelessNetworks: []plan.WirelessNetwork{{
			Name: "home", SSID: "vcpe-lab", Channel: 6, Security: "open",
		}},
	}
	service := plan.Service{
		Name: "station", Type: "generic-container", Replicas: 2,
		Image: manifest.Image{Repository: "example/station", Tag: "test"},
		Instances: []plan.Instance{
			{Index: 0, Radios: []plan.Radio{{Name: "wifi", Network: "home", Device: "wlan0", MAC: "02:00:00:00:00:01", Mode: "station", Addressing: "dhcp"}}},
			{Index: 1, Radios: []plan.Radio{{Name: "wifi", Network: "home", Device: "wlan0", MAC: "02:00:00:00:00:02", Mode: "station", Addressing: "dhcp"}}},
		},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{
		Deployment: deployment,
		Service:    service,
		WirelessCredentialFiles: map[string]render.SecretFileHandle{
			"home": {HostPath: "/private/home", ContainerPath: "/run/vcpe/credentials/home"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]string{}
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	if !strings.Contains(artifacts["instances/1/compose.env"], "RADIO_WIFI_MAC=02:00:00:00:00:01") {
		t.Fatalf("instance 1 radio env is incorrect:\n%s", artifacts["instances/1/compose.env"])
	}
	if !strings.Contains(artifacts["instances/2/compose.env"], "RADIO_WIFI_MAC=02:00:00:00:00:02") {
		t.Fatalf("instance 2 radio env is incorrect:\n%s", artifacts["instances/2/compose.env"])
	}
	for _, path := range []string{"instances/1/compose.env", "instances/2/compose.env"} {
		if !strings.Contains(artifacts["compose.yaml"], path) {
			t.Fatalf("compose.yaml missing %q:\n%s", path, artifacts["compose.yaml"])
		}
	}
	if !strings.Contains(artifacts["compose.yaml"], "/private/home:/run/vcpe/credentials/home:ro") {
		t.Fatalf("compose.yaml does not mount the scoped wireless credential:\n%s", artifacts["compose.yaml"])
	}
}

func TestGenericContainerPersonalSecurityComposeConfig(t *testing.T) {
	podmanCompose, err := exec.LookPath("podman-compose")
	if err != nil {
		t.Skip("podman-compose is required for generated Compose validation")
	}
	genericcontainer.Register()
	serviceType, _ := typeregistry.Lookup("generic-container")
	credential := filepath.Join(t.TempDir(), "passphrase")
	if err := os.WriteFile(credential, []byte("compose-config-sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	deployment := plan.Deployment{Name: "edge", WirelessNetworks: []plan.WirelessNetwork{{
		Name: "home", SSID: "vcpe-lab", Channel: 1, Security: manifest.WirelessWPA3Personal, PassphraseSecretRef: "home-wifi",
	}}}
	service := plan.Service{
		Name: "station", Type: "generic-container", Image: manifest.Image{Repository: "example/station", Tag: "test"},
		Instances: []plan.Instance{{Index: 0, Radios: []plan.Radio{{Name: "wifi", Network: "home", Device: "wlan0", Mode: "station", Addressing: "dhcp"}}}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{
		Deployment: deployment,
		Service:    service,
		WirelessCredentialFiles: map[string]render.SecretFileHandle{
			"home": {HostPath: credential, ContainerPath: "/run/vcpe/credentials/home/passphrase"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifactDir := t.TempDir()
	for _, artifact := range result.Artifacts {
		path := filepath.Join(artifactDir, artifact.Key)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(artifact.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(podmanCompose, "-f", filepath.Join(artifactDir, "compose.yaml"), "config")
	cmd.Dir = artifactDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("podman-compose config: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "/run/vcpe/credentials/home/passphrase") {
		t.Fatalf("expanded Compose omitted credential file path:\n%s", output)
	}
	if strings.Contains(string(output), "compose-config-sentinel") {
		t.Fatalf("expanded Compose exposed credential bytes:\n%s", output)
	}
}

func TestGenericContainerEntrypointWaitsForRadioArrival(t *testing.T) {
	entrypoint := renderEntrypoint(t)
	output, err := runEntrypoint(t, entrypoint, []string{
		"RADIO_WIFI_DEVICE=wlan0",
		"VCPE_RADIO_READY_TIMEOUT_SECONDS=2",
		"VCPE_TEST_IP_READY_AFTER=2",
	})
	if err != nil {
		t.Fatalf("entrypoint failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "workload-started") {
		t.Fatalf("entrypoint did not execute workload: %s", output)
	}
}

func TestGenericContainerEntrypointReportsRadioTimeout(t *testing.T) {
	entrypoint := renderEntrypoint(t)
	output, err := runEntrypoint(t, entrypoint, []string{
		"RADIO_WIFI_DEVICE=wlan0",
		"VCPE_RADIO_READY_TIMEOUT_SECONDS=1",
		"VCPE_TEST_IP_READY_AFTER=99",
	})
	if err == nil || !strings.Contains(output, "missing: wlan0") {
		t.Fatalf("error = %v, output = %s", err, output)
	}
}

func TestGenericContainerEntrypointWithoutRadiosDoesNotWait(t *testing.T) {
	entrypoint := renderEntrypoint(t)
	output, err := runEntrypoint(t, entrypoint, nil)
	if err != nil {
		t.Fatalf("entrypoint failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "ip-called") || !strings.Contains(output, "workload-started") {
		t.Fatalf("unexpected no-radio output: %s", output)
	}
}

func TestGenericContainerRadioOnlyUsesNoNetworkMode(t *testing.T) {
	compose := renderGenericCompose(t, plan.Instance{
		Radios: []plan.Radio{{Name: "wifi", Network: "home", Device: "wlan0", Mode: "station"}},
	})
	service := composeService(t, compose, "station-1")
	if service["network_mode"] != "none" {
		t.Fatalf("network_mode = %#v, want none", service["network_mode"])
	}
	if _, exists := service["networks"]; exists {
		t.Fatalf("radio-only service has networks: %#v", service["networks"])
	}
	if _, exists := compose["networks"]; exists {
		t.Fatalf("radio-only compose has top-level networks: %#v", compose["networks"])
	}
}

func TestGenericContainerMixedTransportPreservesWiredNetwork(t *testing.T) {
	compose := renderGenericCompose(t, plan.Instance{
		Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:01"}},
		Radios:     []plan.Radio{{Name: "wifi", Network: "home", Device: "wlan0", Mode: "station"}},
	})
	service := composeService(t, compose, "station-1")
	if _, exists := service["network_mode"]; exists {
		t.Fatalf("mixed service has network_mode: %#v", service["network_mode"])
	}
	if _, exists := service["networks"]; !exists {
		t.Fatal("mixed service is missing wired networks")
	}
	if _, exists := compose["networks"]; !exists {
		t.Fatal("mixed compose is missing top-level wired networks")
	}
}

func renderGenericCompose(t *testing.T, instance plan.Instance) map[string]any {
	t.Helper()
	genericcontainer.Register()
	serviceType, _ := typeregistry.Lookup("generic-container")
	instance.Index = 0
	service := plan.Service{
		Name: "station", Type: "generic-container", Replicas: 1,
		Image:     manifest.Image{Repository: "example/station", Tag: "test"},
		Instances: []plan.Instance{instance},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			var document map[string]any
			if err := yaml.Unmarshal([]byte(artifact.Content), &document); err != nil {
				t.Fatal(err)
			}
			return document
		}
	}
	t.Fatal("compose artifact not found")
	return nil
}

func composeService(t *testing.T, document map[string]any, name string) map[string]any {
	t.Helper()
	services, ok := document["services"].(map[string]any)
	if !ok {
		t.Fatalf("services = %#v", document["services"])
	}
	service, ok := services[name].(map[string]any)
	if !ok {
		t.Fatalf("service %q = %#v", name, services[name])
	}
	return service
}

func renderEntrypoint(t *testing.T) string {
	t.Helper()
	genericcontainer.Register()
	serviceType, _ := typeregistry.Lookup("generic-container")
	service := plan.Service{
		Name: "station", Type: "generic-container", Replicas: 1,
		Image:     manifest.Image{Repository: "example/station", Tag: "test"},
		Instances: []plan.Instance{{Index: 0}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Key == "entrypoint.sh" {
			return artifact.Content
		}
	}
	t.Fatal("entrypoint artifact not found")
	return ""
}

func runEntrypoint(t *testing.T, entrypoint string, environment []string) (string, error) {
	t.Helper()
	directory := t.TempDir()
	entrypointPath := filepath.Join(directory, "entrypoint.sh")
	if err := os.WriteFile(entrypointPath, []byte(entrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	ipScript := `#!/bin/sh
echo ip-called
echo call >> "$VCPE_TEST_IP_CALLS"
count=$(wc -l < "$VCPE_TEST_IP_CALLS")
[ "$count" -ge "${VCPE_TEST_IP_READY_AFTER:-99}" ]
`
	if err := os.WriteFile(filepath.Join(directory, "ip"), []byte(ipScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "sleep"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", entrypointPath, "/bin/sh", "-c", "echo workload-started")
	command.Env = append([]string{
		"PATH=" + directory + ":" + os.Getenv("PATH"),
		"VCPE_TEST_IP_CALLS=" + filepath.Join(directory, "ip-calls"),
	}, environment...)
	output, err := command.CombinedOutput()
	return string(output), err
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
