package types_test

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
	"gopkg.in/yaml.v3"
)

func TestBuiltInRendererArtifactInventory(t *testing.T) {
	types.Register()

	literalArtifacts := []string{
		"compose.env",
		"compose.yaml",
		"instances/1/compose.env",
		"instances/2/compose.env",
	}
	bngExtras := []string{
		"etc/dhcp/dhcpd.conf",
		"etc/dhcp/dhcpd6.conf",
		"etc/dnsmasq.conf",
		"etc/dnsmasq.dhcp-hosts.map",
		"etc/dnsmasq.dhcp-subnets.map",
		"etc/dnsmasq.dynamic.hosts",
		"etc/dnsmasq.hosts",
		"etc/iptables.rules.v4",
		"etc/iptables.rules.v6",
		"etc/network-startup.sh",
		"etc/ntp.conf",
		"etc/ports.conf",
		"etc/radvd.conf",
		"etc/service-interfaces.env",
		"etc/sysctl.conf",
		"var/www/html/DCMresponse.txt",
	}
	bngArtifacts := append([]string(nil), literalArtifacts...)
	for _, key := range bngExtras {
		bngArtifacts = append(bngArtifacts, key)
		bngArtifacts = append(bngArtifacts, "instances/1/"+key, "instances/2/"+key)
	}

	tests := []struct {
		typeName string
		renderer string
		wantKeys []string
	}{
		{typeName: "bng", renderer: "bng-renderer", wantKeys: bngArtifacts},
		{typeName: "event-sink", renderer: "event-sink-renderer", wantKeys: literalArtifacts},
		{typeName: "gateway", renderer: "gateway-renderer", wantKeys: literalArtifacts},
		{typeName: "generic-container", renderer: "generic-container-renderer", wantKeys: []string{"compose.env", "compose.yaml", "entrypoint.sh"}},
		{typeName: "oktopus", renderer: "oktopus-renderer", wantKeys: literalArtifacts},
		{typeName: "webpa", renderer: "webpa-renderer", wantKeys: literalArtifacts},
		{typeName: "xb10", renderer: "xb10-renderer", wantKeys: literalArtifacts},
	}

	for _, test := range tests {
		t.Run(test.typeName, func(t *testing.T) {
			registered, ok := typeregistry.Lookup(test.typeName)
			if !ok {
				t.Fatalf("%s is not registered", test.typeName)
			}
			result, err := registered.Renderer().Render(context.Background(), artifactInventoryInput(t, test.typeName))
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if result.Renderer != test.renderer {
				t.Fatalf("renderer = %q, want %q", result.Renderer, test.renderer)
			}

			gotKeys := make([]string, 0, len(result.Artifacts))
			for _, artifact := range result.Artifacts {
				gotKeys = append(gotKeys, artifact.Key)
			}
			sort.Strings(gotKeys)
			wantKeys := append([]string(nil), test.wantKeys...)
			sort.Strings(wantKeys)
			if !slices.Equal(gotKeys, wantKeys) {
				t.Fatalf("artifact keys:\n got: %q\nwant: %q", gotKeys, wantKeys)
			}
		})
	}
}

func TestBuiltInRendererComposeSemantics(t *testing.T) {
	types.Register()
	tests := []struct {
		typeName        string
		containerName   string
		hostname        string
		networkName     string
		publishesHealth bool
		pinsMAC         bool
		pinsIPv4        bool
	}{
		{typeName: "bng", containerName: "edge-bng-1", hostname: "bng-1", networkName: "edge-mgmt", publishesHealth: true, pinsMAC: true, pinsIPv4: true},
		{typeName: "event-sink", containerName: "edge-event-sink-1", hostname: "event-sink-1", networkName: "edge-mgmt", publishesHealth: true, pinsMAC: true, pinsIPv4: true},
		{typeName: "gateway", containerName: "edge-gateway-1", hostname: "gateway-1", networkName: "edge-mgmt", pinsMAC: true},
		{typeName: "generic-container", containerName: "${DEPLOYMENT_NAME}-${SERVICE_NAME}-1", hostname: "${SERVICE_NAME}-1", networkName: "${IFACE_MGMT_NETWORK}"},
		{typeName: "oktopus", containerName: "edge-oktopus-1", hostname: "oktopus-1", networkName: "edge-mgmt", publishesHealth: true, pinsMAC: true, pinsIPv4: true},
		{typeName: "webpa", containerName: "edge-webpa-1", hostname: "webpa-1", networkName: "edge-mgmt", publishesHealth: true, pinsMAC: true, pinsIPv4: true},
		{typeName: "xb10", containerName: "edge-xb10-1", hostname: "xb10-1", networkName: "edge-mgmt", publishesHealth: true, pinsMAC: true},
	}

	for _, test := range tests {
		t.Run(test.typeName, func(t *testing.T) {
			registered, ok := typeregistry.Lookup(test.typeName)
			if !ok {
				t.Fatalf("%s is not registered", test.typeName)
			}
			result, err := registered.Renderer().Render(context.Background(), artifactInventoryInput(t, test.typeName))
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			var compose struct {
				Services map[string]map[string]any `yaml:"services"`
				Networks map[string]map[string]any `yaml:"networks"`
			}
			if err := yaml.Unmarshal([]byte(mustArtifactContent(t, result, "compose.yaml")), &compose); err != nil {
				t.Fatalf("unmarshal compose: %v", err)
			}

			wantServiceNames := []string{test.typeName + "-1", test.typeName + "-2"}
			gotServiceNames := sortedMapKeys(compose.Services)
			if !slices.Equal(gotServiceNames, wantServiceNames) {
				t.Fatalf("service names = %q, want %q", gotServiceNames, wantServiceNames)
			}
			service := compose.Services[test.typeName+"-1"]
			if service["container_name"] != test.containerName {
				t.Errorf("container_name = %#v, want %q", service["container_name"], test.containerName)
			}
			if service["hostname"] != test.hostname {
				t.Errorf("hostname = %#v, want %q", service["hostname"], test.hostname)
			}
			ports := stringSlice(service["ports"])
			if !slices.Contains(ports, "18080:8080") {
				t.Errorf("ports %q do not preserve manifest mapping", ports)
			}
			hasHealth := slices.Contains(ports, "127.0.0.1:47000:9878")
			if hasHealth != test.publishesHealth {
				t.Errorf("publishes direct health = %t, want %t; ports=%q", hasHealth, test.publishesHealth, ports)
			}

			network := compose.Networks["mgmt"]
			if network["external"] != true || network["name"] != test.networkName {
				t.Errorf("mgmt network = %#v, want external name %q", network, test.networkName)
			}
			serviceNetworks, ok := service["networks"].(map[string]any)
			if !ok {
				t.Fatalf("service networks = %#v", service["networks"])
			}
			attachment, ok := serviceNetworks["mgmt"].(map[string]any)
			if !ok {
				t.Fatalf("mgmt attachment = %#v", serviceNetworks["mgmt"])
			}
			_, hasMAC := attachment["mac_address"]
			_, hasIPv4 := attachment["ipv4_address"]
			if hasMAC != test.pinsMAC || hasIPv4 != test.pinsIPv4 {
				t.Errorf("attachment = %#v, want pinsMAC=%t pinsIPv4=%t", attachment, test.pinsMAC, test.pinsIPv4)
			}
		})
	}
}

func mustArtifactContent(t *testing.T, result render.Result, key string) string {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Key == key {
			return artifact.Content
		}
	}
	t.Fatalf("artifact %q not found", key)
	return ""
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringSlice(value any) []string {
	if value == nil {
		return nil
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice {
		return nil
	}
	values := make([]string, 0, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		if value, ok := reflected.Index(index).Interface().(string); ok {
			values = append(values, value)
		}
	}
	return values
}

func artifactInventoryInput(t *testing.T, typeName string) render.Input {
	t.Helper()
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("{}"), &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	instances := make([]plan.Instance, 0, 2)
	for index := range 2 {
		instances = append(instances, plan.Instance{
			Index: index,
			Interfaces: []plan.Interface{{
				Role:    "mgmt",
				Network: "edge-mgmt",
				Device:  "eth0",
				MAC:     fmt.Sprintf("02:00:00:00:00:%02x", index+1),
				IPv4:    fmt.Sprintf("10.0.0.%d", index+2),
			}},
		})
	}
	return render.Input{
		Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt"}}},
		Service: plan.Service{
			Name:      typeName,
			Type:      typeName,
			Image:     manifest.Image{Repository: "example/" + typeName, Tag: "test"},
			Replicas:  2,
			Ports:     []string{"18080:8080"},
			Config:    config,
			Instances: instances,
		},
		HealthPorts: map[int]int{0: 47000, 1: 47001},
	}
}
