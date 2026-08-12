package xb10

import (
	"context"
	"reflect"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"gopkg.in/yaml.v3"
)

func TestRendererEnvironmentArtifacts(t *testing.T) {
	result, err := (serviceType{}).Renderer().Render(context.Background(), xb10RenderInput(t))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	want := map[string]string{
		"compose.env":             xb10Environment("02:00:00:00:00:01", "02:00:00:00:00:02", "10.0.0.2", "192.0.2.2"),
		"instances/1/compose.env": xb10Environment("02:00:00:00:00:01", "02:00:00:00:00:02", "10.0.0.2", "192.0.2.2"),
		"instances/2/compose.env": xb10Environment("02:00:00:00:00:03", "02:00:00:00:00:04", "10.0.0.3", "192.0.2.3"),
	}
	got := map[string]string{}
	for _, artifact := range result.Artifacts {
		if artifact.Key != "compose.yaml" {
			got[artifact.Key] = artifact.Content
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment artifacts:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRendererComposeContract(t *testing.T) {
	result, err := (serviceType{}).Renderer().Render(context.Background(), xb10RenderInput(t))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var compose struct {
		Services map[string]struct {
			Ports    []string                          `yaml:"ports"`
			Volumes  []string                          `yaml:"volumes"`
			Networks map[string]map[string]interface{} `yaml:"networks"`
		} `yaml:"services"`
		Networks map[string]struct {
			External bool   `yaml:"external"`
			Name     string `yaml:"name"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(xb10Artifact(t, result, "compose.yaml")), &compose); err != nil {
		t.Fatalf("unmarshal compose: %v", err)
	}

	service := compose.Services["xb10-1"]
	if want := []string{"18080:8080", "127.0.0.1:47000:9878"}; !reflect.DeepEqual(service.Ports, want) {
		t.Errorf("ports = %q, want %q", service.Ports, want)
	}
	if want := []string{"xb10-data:/data", "/tmp/xb10:/host"}; !reflect.DeepEqual(service.Volumes, want) {
		t.Errorf("volumes = %q, want %q", service.Volumes, want)
	}
	wantAttachments := map[string]map[string]interface{}{
		"cm":  {"mac_address": "02:00:00:00:00:02"},
		"wan": {"mac_address": "02:00:00:00:00:01"},
	}
	if !reflect.DeepEqual(service.Networks, wantAttachments) {
		t.Errorf("network attachments = %#v, want %#v", service.Networks, wantAttachments)
	}
	for role, attachment := range service.Networks {
		if _, ok := attachment["ipv4_address"]; ok {
			t.Errorf("%s attachment unexpectedly pins ipv4_address: %#v", role, attachment)
		}
	}
	wantNetworks := map[string]struct {
		External bool   `yaml:"external"`
		Name     string `yaml:"name"`
	}{
		"cm":  {External: true, Name: "edge-cm"},
		"wan": {External: true, Name: "edge-wan"},
	}
	if !reflect.DeepEqual(compose.Networks, wantNetworks) {
		t.Errorf("networks = %#v, want %#v", compose.Networks, wantNetworks)
	}
}

func xb10RenderInput(t *testing.T) render.Input {
	t.Helper()
	var config yaml.Node
	if err := yaml.Unmarshal([]byte("erouter:\n  vlan: 100\nenv:\n  ZETA: last\n  ALPHA: first\n"), &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	instances := []plan.Instance{
		{Index: 0, Interfaces: xb10Interfaces("01", "02", "2")},
		{Index: 1, Interfaces: xb10Interfaces("03", "04", "3")},
	}
	return render.Input{
		Deployment: plan.Deployment{
			Name: "edge",
			Networks: []plan.Network{
				{Role: "wan", IPv4: &plan.Family{CIDR: "192.0.2.0/24"}},
				{Role: "cm"},
			},
		},
		Service: plan.Service{
			Name:      "xb10",
			Type:      TypeName,
			Image:     manifest.Image{Repository: "example/xb10", Tag: "test"},
			Replicas:  2,
			Ports:     []string{"18080:8080"},
			Volumes:   []string{"xb10-data:/data", "/tmp/xb10:/host"},
			Config:    config,
			Instances: instances,
		},
		HealthPorts: map[int]int{0: 47000, 1: 47001},
	}
}

func xb10Interfaces(wanMAC, cmMAC, host string) []plan.Interface {
	return []plan.Interface{
		{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:00:00:00:00:" + wanMAC, IPv4: "192.0.2." + host, IPv6: "2001:db8::" + host, Gateway4: "192.0.2.1", Gateway6: "2001:db8::1", Addressing: manifest.AddressingStatic},
		{Role: "cm", Network: "edge-cm", Device: "eth1", MAC: "02:00:00:00:00:" + cmMAC, IPv4: "10.0.0." + host, IPv6: "2001:db8:1::" + host},
	}
}

func xb10Environment(wanMAC, cmMAC, cmIP, wanIP string) string {
	return "DEPLOYMENT_NAME=edge\n" +
		"SERVICE_NAME=xb10\n" +
		"IMAGE=example/xb10:test\n" +
		"IFACE_CM_ADDRESSING=dhcp\n" +
		"IFACE_CM_BRIDGE=\n" +
		"IFACE_CM_DEVICE=eth1\n" +
		"IFACE_CM_GATEWAY4=\n" +
		"IFACE_CM_GATEWAY6=\n" +
		"IFACE_CM_IPV4=" + cmIP + "\n" +
		"IFACE_CM_IPV6=2001:db8:1::" + cmIP[len(cmIP)-1:] + "\n" +
		"IFACE_CM_MAC=" + cmMAC + "\n" +
		"IFACE_CM_NETWORK=edge-cm\n" +
		"IFACE_WAN_ADDRESSING=static\n" +
		"IFACE_WAN_BRIDGE=\n" +
		"IFACE_WAN_DEVICE=eth0\n" +
		"IFACE_WAN_GATEWAY4=192.0.2.1\n" +
		"IFACE_WAN_GATEWAY6=2001:db8::1\n" +
		"IFACE_WAN_IPV4=" + wanIP + "\n" +
		"IFACE_WAN_IPV6=2001:db8::" + wanIP[len(wanIP)-1:] + "\n" +
		"IFACE_WAN_MAC=" + wanMAC + "\n" +
		"IFACE_WAN_NETWORK=edge-wan\n" +
		"EROUTER0_IPV4=" + wanIP + "/24\n" +
		"EROUTER0_IPV6=2001:db8::" + wanIP[len(wanIP)-1:] + "\n" +
		"EROUTER0_IPV4_GATEWAY=192.0.2.1\n" +
		"EROUTER0_IPV6_GATEWAY=2001:db8::1\n" +
		"EROUTER0_VLAN=100\n" +
		"ALPHA=first\n" +
		"ZETA=last\n" +
		"WAN0_IPV4=" + cmIP + "\n" +
		"WAN0_IPV6=2001:db8:1::" + cmIP[len(cmIP)-1:] + "\n"
}

func xb10Artifact(t *testing.T, result render.Result, key string) string {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Key == key {
			return artifact.Content
		}
	}
	t.Fatalf("artifact %q not found", key)
	return ""
}
