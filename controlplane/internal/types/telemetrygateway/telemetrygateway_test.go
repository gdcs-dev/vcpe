package telemetrygateway

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

func TestServiceTypeMetadata(t *testing.T) {
	serviceType := serviceType{}
	if serviceType.Type() != "telemetry-gateway" {
		t.Errorf("Type() = %q", serviceType.Type())
	}
	if serviceType.DefaultImage() != "ghcr.io/gdcs-dev/telemetry-gateway" {
		t.Errorf("DefaultImage() = %q", serviceType.DefaultImage())
	}
	if serviceType.DefaultImagePolicy() != "build" {
		t.Errorf("DefaultImagePolicy() = %q", serviceType.DefaultImagePolicy())
	}
	if health := serviceType.Health(); health != (typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 8080}) {
		t.Errorf("Health() = %+v", health)
	}
	roles := serviceType.ExpectedRoles()
	if len(roles) != 1 || roles[0].Role != "mgmt" || !roles[0].Required {
		t.Errorf("ExpectedRoles() = %+v", roles)
	}
	if serviceType.Renderer() == nil || serviceType.Renderer().Name() != "telemetry-gateway-renderer" {
		t.Errorf("Renderer() = %#v", serviceType.Renderer())
	}
}

func TestServiceTypeValidation(t *testing.T) {
	serviceType := serviceType{}
	for _, testCase := range []struct {
		name    string
		config  string
		wantErr bool
	}{
		{name: "omitted"},
		{name: "empty", config: "{}"},
		{name: "string environment", config: "env:\n  ARGUS_URL: http://argus:6600"},
		{name: "unknown field", config: "credentials: dev", wantErr: true},
		{name: "non-string environment", config: "env:\n  PORT: 8080", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var node yaml.Node
			if testCase.config != "" {
				if err := yaml.Unmarshal([]byte(testCase.config), &node); err != nil {
					t.Fatal(err)
				}
			}
			err := serviceType.ValidateConfig(node)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("ValidateConfig() error = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}

	if err := serviceType.ValidateReplicas(1); err != nil {
		t.Fatalf("ValidateReplicas(1) error = %v", err)
	}
	if err := serviceType.ValidateReplicas(2); err == nil || !strings.Contains(err.Error(), "singleton") {
		t.Fatalf("ValidateReplicas(2) error = %v", err)
	}
}

func TestRendererProducesEmbeddedGatewayArtifacts(t *testing.T) {
	result := renderTelemetryGateway(t)
	artifacts := map[string]string{}
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	for _, key := range []string{"compose.env", "instances/1/compose.env", "compose.yaml"} {
		if _, ok := artifacts[key]; !ok {
			t.Errorf("missing operation artifact %q: %v", key, artifactKeys(artifacts))
		}
	}
	environment := artifacts["compose.env"]
	if strings.Index(environment, "A_FIRST=first") > strings.Index(environment, "Z_LAST=last") {
		t.Fatalf("environment overrides are not sorted:\n%s", environment)
	}
	if !strings.HasSuffix(strings.TrimSpace(environment), "Z_LAST=last") {
		t.Fatalf("environment overrides do not follow interface defaults:\n%s", environment)
	}

	document := parseCompose(t, artifacts["compose.yaml"])
	services := composeMap(t, document["services"], "services")
	service := composeMap(t, services["telemetry-gateway-1"], "services.telemetry-gateway-1")
	if service["image"] != "ghcr.io/gdcs-dev/telemetry-gateway:test" || service["restart"] != "unless-stopped" {
		t.Errorf("service identity/policy = %#v", service)
	}
	for _, forbidden := range []string{"privileged", "cap_add", "entrypoint"} {
		if _, ok := service[forbidden]; ok {
			t.Errorf("service unexpectedly sets %s: %#v", forbidden, service[forbidden])
		}
	}
	ports := composeStrings(t, service["ports"], "ports")
	for _, want := range []string{"18080:8080", "127.0.0.1:47000:8080"} {
		if !contains(ports, want) {
			t.Errorf("ports = %v, want %q", ports, want)
		}
	}
	networks := composeMap(t, service["networks"], "service.networks")
	management := composeMap(t, networks["mgmt"], "service.networks.mgmt")
	if management["mac_address"] != "02:00:00:00:00:10" || management["ipv4_address"] != "10.0.0.10" {
		t.Errorf("management attachment = %#v", management)
	}
	aliases := composeStrings(t, management["aliases"], "management.aliases")
	if len(aliases) != 1 || aliases[0] != "telemetry-gateway" {
		t.Errorf("management aliases = %v", aliases)
	}
	dns := composeStrings(t, service["dns"], "dns")
	if len(dns) != 1 || dns[0] != "10.0.0.2" {
		t.Errorf("dns = %v, want BNG as sole custom upstream", dns)
	}
	if contains(dns, "10.0.0.1") {
		t.Errorf("dns = %v, must not include Podman gateway", dns)
	}
}

func TestRendererDeclaresEmbeddedDataVolumes(t *testing.T) {
	result := renderTelemetryGateway(t)
	document := parseCompose(t, artifactContent(t, result, "compose.yaml"))
	volumes := composeMap(t, document["volumes"], "volumes")
	wantVolumes := map[string]string{
		"postgres-data":   "/var/lib/postgresql/data",
		"prometheus-data": "/var/lib/prometheus/metrics2",
		"grafana-data":    "/var/lib/grafana",
	}
	if len(volumes) != len(wantVolumes) {
		t.Fatalf("volume declarations = %#v", volumes)
	}
	service := composeMap(t, composeMap(t, document["services"], "services")["telemetry-gateway-1"], "service")
	mounts := composeStrings(t, service["volumes"], "service.volumes")
	for name, path := range wantVolumes {
		if _, ok := volumes[name]; !ok {
			t.Errorf("missing volume declaration %q", name)
		}
		if !contains(mounts, name+":"+path) {
			t.Errorf("mounts = %v, want %s:%s", mounts, name, path)
		}
		if contains(mounts, path) {
			t.Errorf("mounts include anonymous data path %q: %v", path, mounts)
		}
	}
}

func renderTelemetryGateway(t *testing.T) render.Result {
	t.Helper()
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("env:\n  Z_LAST: last\n  A_FIRST: first"), &config); err != nil {
		t.Fatal(err)
	}
	management := plan.Network{Role: "mgmt", Bridge: "edge-mgmt", IPv4: &plan.Family{CIDR: "10.0.0.0/24", Gateway: "10.0.0.1"}}
	bng := plan.Service{Type: "bng", Instances: []plan.Instance{{Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", IPv4: "10.0.0.2"}}}}}
	service := plan.Service{
		Name: "telemetry-gateway", Type: "telemetry-gateway", Replicas: 1,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/telemetry-gateway", Tag: "test"},
		Ports: []string{"18080:8080"}, Config: config,
		Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{
			Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:10", IPv4: "10.0.0.10", ManagedNetwork: true,
		}}}},
	}
	result, err := (serviceType{}).Renderer().Render(context.Background(), render.Input{
		Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{management}, Services: []plan.Service{bng, service}},
		Service:    service, HealthPorts: map[int]int{0: 47000},
	})
	if err != nil {
		t.Fatalf("render telemetry-gateway: %v", err)
	}
	return result
}

func artifactContent(t *testing.T, result render.Result, key string) string {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Key == key {
			return artifact.Content
		}
	}
	t.Fatalf("missing artifact %q", key)
	return ""
}

func artifactKeys(artifacts map[string]string) []string {
	keys := make([]string, 0, len(artifacts))
	for key := range artifacts {
		keys = append(keys, key)
	}
	return keys
}

func parseCompose(t *testing.T, content string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse compose: %v", err)
	}
	return document
}

func composeMap(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	mapping, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want mapping", path, value)
	}
	return mapping
}

func composeStrings(t *testing.T, value any, path string) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want sequence", path, value)
	}
	strings := make([]string, len(values))
	for index, value := range values {
		stringValue, ok := value.(string)
		if !ok {
			t.Fatalf("%s[%d] = %#v, want string", path, index, value)
		}
		strings[index] = stringValue
	}
	return strings
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
