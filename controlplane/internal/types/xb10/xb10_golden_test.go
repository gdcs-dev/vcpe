package xb10_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/xb10"
	"gopkg.in/yaml.v3"
)

func TestXB10RendererPreservesArtifactsAndNetworkPolicy(t *testing.T) {
	xb10.Register()
	registered, ok := typeregistry.Lookup(xb10.TypeName)
	if !ok {
		t.Fatal("xb10 is not registered")
	}
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("erouter: {vlan: 100}\nenv: {ZED: z, ALPHA: a}\n"), &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	service := plan.Service{
		Name:     "xb10",
		Type:     xb10.TypeName,
		Replicas: 2,
		Image:    manifest.Image{Repository: "example/xb10", Tag: "test"},
		Ports:    []string{"18080:8080"},
		Volumes:  []string{"./data:/data"},
		Config:   config,
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{
				{Role: "wan", Network: "edge-wan", Device: "erouter0", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2", Gateway4: "10.0.0.1", Addressing: "static"},
				{Role: "cm", Network: "edge-cm", Device: "wan0", MAC: "02:00:00:00:00:02", IPv4: "10.1.0.2", Addressing: "static"},
			}},
			{Index: 1, Interfaces: []plan.Interface{
				{Role: "wan", Network: "edge-wan", Device: "erouter0", MAC: "02:00:00:00:00:03", IPv4: "10.0.0.3", Gateway4: "10.0.0.1", Addressing: "static"},
				{Role: "cm", Network: "edge-cm", Device: "wan0", MAC: "02:00:00:00:00:04", IPv4: "10.1.0.3", Addressing: "static"},
			}},
		},
	}
	result, err := registered.Renderer().Render(context.Background(), render.Input{
		Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{
			{Role: "wan", Bridge: "edge-wan", IPv4: &plan.Family{CIDR: "10.0.0.0/24"}},
			{Role: "cm", Bridge: "edge-cm", IPv4: &plan.Family{CIDR: "10.1.0.0/24"}},
		}},
		Service:     service,
		HealthPorts: map[int]int{0: 47000, 1: 47001},
	})
	if err != nil {
		t.Fatalf("render xb10: %v", err)
	}
	if result.Renderer != "xb10-renderer" {
		t.Errorf("renderer = %q, want xb10-renderer", result.Renderer)
	}
	artifacts := xb10Artifacts(result)
	for _, key := range []string{"compose.env", "compose.yaml", "instances/1/compose.env", "instances/2/compose.env"} {
		if _, ok := artifacts[key]; !ok {
			t.Errorf("missing artifact %q", key)
		}
	}
	if artifacts["compose.env"] != artifacts["instances/1/compose.env"] {
		t.Error("root compose.env does not mirror the first instance")
	}
	for _, want := range []string{"EROUTER0_IPV4=10.0.0.2/24", "EROUTER0_VLAN=100", "WAN0_IPV4=10.1.0.2", "ALPHA=a", "ZED=z"} {
		if !strings.Contains(artifacts["compose.env"], want) {
			t.Errorf("first compose.env missing %q:\n%s", want, artifacts["compose.env"])
		}
	}
	if strings.Index(artifacts["compose.env"], "ALPHA=a") > strings.Index(artifacts["compose.env"], "ZED=z") {
		t.Errorf("configured environment is not sorted:\n%s", artifacts["compose.env"])
	}
	if !strings.Contains(artifacts["instances/2/compose.env"], "EROUTER0_IPV4=10.0.0.3/24") {
		t.Errorf("second compose.env does not use the second instance:\n%s", artifacts["instances/2/compose.env"])
	}

	compose := xb10Compose(t, artifacts["compose.yaml"])
	services := xb10Map(t, compose["services"], "services")
	for index, healthPort := range map[int]string{1: "47000", 2: "47001"} {
		name := "xb10-" + string(rune('0'+index))
		workload := xb10Map(t, services[name], "services."+name)
		if workload["container_name"] != "edge-"+name || workload["hostname"] != name {
			t.Errorf("service %q identity = container %#v hostname %#v", name, workload["container_name"], workload["hostname"])
		}
		if !xb10Contains(workload["ports"], "18080:8080") || !xb10Contains(workload["ports"], "127.0.0.1:"+healthPort+":9878") {
			t.Errorf("service %q ports = %#v", name, workload["ports"])
		}
		if !xb10Contains(workload["volumes"], "./data:/data") {
			t.Errorf("service %q volumes = %#v", name, workload["volumes"])
		}
		attachments := xb10Map(t, workload["networks"], "services."+name+".networks")
		for _, role := range []string{"wan", "cm"} {
			attachment := xb10Map(t, attachments[role], "services."+name+".networks."+role)
			if attachment["mac_address"] == "" {
				t.Errorf("service %q %s attachment lacks mac_address", name, role)
			}
			if _, pinned := attachment["ipv4_address"]; pinned {
				t.Errorf("service %q %s attachment unexpectedly pins ipv4_address: %#v", name, role, attachment)
			}
		}
	}
	for _, role := range []string{"wan", "cm"} {
		network := xb10Map(t, xb10Map(t, compose["networks"], "networks")[role], "networks."+role)
		if network["external"] != true || network["name"] != "edge-"+role {
			t.Errorf("network %q = %#v", role, network)
		}
	}
}

func xb10Artifacts(result render.Result) map[string]string {
	artifacts := make(map[string]string, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	return artifacts
}

func xb10Compose(t *testing.T, content string) map[string]any {
	t.Helper()
	var compose map[string]any
	if err := yaml.Unmarshal([]byte(content), &compose); err != nil {
		t.Fatalf("parse compose.yaml: %v", err)
	}
	return compose
}

func xb10Map(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	mapping, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want mapping", path, value)
	}
	return mapping
}

func xb10Contains(value any, want string) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
