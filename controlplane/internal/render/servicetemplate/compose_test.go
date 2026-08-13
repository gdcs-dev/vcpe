package servicetemplate_test

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
)

func TestDefaultAttachmentPinsIPv4OnlyWhenManaged(t *testing.T) {
	iface := plan.Interface{MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}
	entry := servicetemplate.DefaultAttachment(iface, true)
	if entry["mac_address"] != iface.MAC {
		t.Fatalf("mac_address = %#v, want %q", entry["mac_address"], iface.MAC)
	}
	if entry["ipv4_address"] != iface.IPv4 {
		t.Fatalf("ipv4_address = %#v, want %q", entry["ipv4_address"], iface.IPv4)
	}

	entry = servicetemplate.DefaultAttachment(iface, false)
	if _, ok := entry["ipv4_address"]; ok {
		t.Fatalf("unmanaged network must not pin ipv4_address, got %#v", entry)
	}
	if entry["mac_address"] != iface.MAC {
		t.Fatalf("mac_address = %#v, want %q", entry["mac_address"], iface.MAC)
	}
}

func TestMACOnlyAttachmentNeverPinsIPv4(t *testing.T) {
	iface := plan.Interface{MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}
	for _, managed := range []bool{true, false} {
		entry := servicetemplate.MACOnlyAttachment(iface, managed)
		if _, ok := entry["ipv4_address"]; ok {
			t.Fatalf("MACOnlyAttachment must never pin ipv4_address (managed=%t), got %#v", managed, entry)
		}
		if entry["mac_address"] != iface.MAC {
			t.Fatalf("mac_address = %#v, want %q", entry["mac_address"], iface.MAC)
		}
	}
}

func TestBuildComposeServiceStandardFields(t *testing.T) {
	input := render.Input{
		Deployment: plan.Deployment{Name: "edge"},
		Service: plan.Service{
			Name:  "svc",
			Image: manifest.Image{Repository: "example/svc", Tag: "test"},
		},
	}
	instance := plan.Instance{Index: 1, Interfaces: []plan.Interface{
		{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"},
	}}

	svc, externalNetworks := servicetemplate.BuildComposeService(input, instance, servicetemplate.DefaultAttachment)

	if svc["image"] != "example/svc:test" {
		t.Errorf("image = %#v, want %q", svc["image"], "example/svc:test")
	}
	if svc["container_name"] != "edge-svc-2" {
		t.Errorf("container_name = %#v, want %q", svc["container_name"], "edge-svc-2")
	}
	if svc["hostname"] != "svc-2" {
		t.Errorf("hostname = %#v, want %q", svc["hostname"], "svc-2")
	}
	envFile, ok := svc["env_file"].([]string)
	if !ok || len(envFile) != 1 || envFile[0] != "instances/2/compose.env" {
		t.Errorf("env_file = %#v, want [instances/2/compose.env]", svc["env_file"])
	}
	networks, ok := svc["networks"].(map[string]any)
	if !ok {
		t.Fatalf("networks = %#v, want map[string]any", svc["networks"])
	}
	mgmt, ok := networks["mgmt"].(map[string]any)
	if !ok || mgmt["mac_address"] != "02:00:00:00:00:01" || mgmt["ipv4_address"] != "10.0.0.2" {
		t.Errorf("networks[mgmt] = %#v", networks["mgmt"])
	}
	extNet, ok := externalNetworks["mgmt"].(map[string]any)
	if !ok || extNet["external"] != true || extNet["name"] != "edge-mgmt" {
		t.Errorf("externalNetworks[mgmt] = %#v", externalNetworks["mgmt"])
	}
}

func TestAttachHealthPublicationNoOpWhenHealthPortZero(t *testing.T) {
	topNets, svcNets, svc := map[string]any{}, map[string]any{}, map[string]any{}
	input := render.Input{Deployment: plan.Deployment{Name: "edge"}}
	instance := plan.Instance{Interfaces: []plan.Interface{{Role: "wan"}}}
	servicetemplate.AttachHealthPublication(input, instance, 0, topNets, svcNets, svc)
	if len(topNets) != 0 || len(svcNets) != 0 || len(svc) != 0 {
		t.Fatalf("expected no-op when healthPort is 0, got topNets=%v svcNets=%v svc=%v", topNets, svcNets, svc)
	}
}

func TestAttachHealthPublicationManagedTopologySkipsPrivateNetwork(t *testing.T) {
	input := render.Input{Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt"}}}}
	instance := plan.Instance{Interfaces: []plan.Interface{{Role: "mgmt"}}}
	topNets, svcNets, svc := map[string]any{}, map[string]any{}, map[string]any{}

	servicetemplate.AttachHealthPublication(input, instance, 47000, topNets, svcNets, svc)

	if _, ok := svcNets["aa-health"]; ok {
		t.Errorf("expected no aa-health attachment for a managed topology interface, got %#v", svcNets)
	}
	if len(topNets) != 0 {
		t.Errorf("expected no aa-health network declared, got %#v", topNets)
	}
	ports, ok := svc["ports"].([]string)
	if !ok || len(ports) != 1 || ports[0] != "127.0.0.1:47000:9878" {
		t.Errorf("svc ports = %#v", svc["ports"])
	}
}

func TestAttachHealthPublicationSelfAddressedUsesPrivateNetwork(t *testing.T) {
	input := render.Input{Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", IPAMDriver: "none"}}}}
	instance := plan.Instance{Interfaces: []plan.Interface{{Role: "wan"}}}
	topNets, svcNets, svc := map[string]any{}, map[string]any{}, map[string]any{}

	servicetemplate.AttachHealthPublication(input, instance, 47000, topNets, svcNets, svc)

	if _, ok := svcNets["aa-health"]; !ok {
		t.Errorf("workload networks missing aa-health entry: %#v", svcNets)
	}
	healthNet, ok := topNets["aa-health"].(map[string]any)
	if !ok || healthNet["external"] != true || healthNet["name"] != "edge-00-health" {
		t.Errorf("topNets[aa-health] = %#v", topNets["aa-health"])
	}
	ports, ok := svc["ports"].([]string)
	if !ok || len(ports) != 1 || ports[0] != "127.0.0.1:47000:9878" {
		t.Errorf("svc ports = %#v", svc["ports"])
	}
}

func TestAttachHealthPublicationPreservesExistingApplicationPorts(t *testing.T) {
	input := render.Input{Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt"}}}}
	instance := plan.Instance{Interfaces: []plan.Interface{{Role: "mgmt"}}}
	topNets, svcNets := map[string]any{}, map[string]any{}
	svc := map[string]any{"ports": []string{"8080:8080"}}

	servicetemplate.AttachHealthPublication(input, instance, 47000, topNets, svcNets, svc)

	ports, ok := svc["ports"].([]string)
	if !ok || len(ports) != 2 || ports[0] != "8080:8080" || ports[1] != "127.0.0.1:47000:9878" {
		t.Errorf("svc ports = %#v, want existing port preserved plus health mapping", svc["ports"])
	}
}

func TestAttachHealthPublicationNoServicesMapMutated(t *testing.T) {
	// The helper mutates only the selected workload's own maps; it never adds
	// another service (no transport proxy).
	input := render.Input{Deployment: plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "wan", IPAMDriver: "none"}}}}
	instance := plan.Instance{Interfaces: []plan.Interface{{Role: "wan"}}}
	topNets, svcNets, svc := map[string]any{}, map[string]any{}, map[string]any{}

	servicetemplate.AttachHealthPublication(input, instance, 47000, topNets, svcNets, svc)

	if _, ok := svc["depends_on"]; ok {
		t.Errorf("expected no depends_on entry, got %#v", svc["depends_on"])
	}
	if len(topNets) != 1 {
		t.Errorf("expected exactly one declared network (aa-health), got %#v", topNets)
	}
}

func TestAttachProbeSidecarShape(t *testing.T) {
	services := map[string]any{}
	servicetemplate.AttachProbeSidecar(services, "client", 0, "example/client:test", []string{"--timeout", "3s", "--probe", "configured=true"})

	sidecar, ok := services["client-health-1"].(map[string]any)
	if !ok {
		t.Fatalf("missing sidecar service, got %#v", services)
	}
	if sidecar["image"] != "example/client:test" {
		t.Errorf("sidecar image = %#v", sidecar["image"])
	}
	if sidecar["network_mode"] != "service:client-1" {
		t.Errorf("sidecar network_mode = %#v", sidecar["network_mode"])
	}
	dependsOn, ok := sidecar["depends_on"].([]string)
	if !ok || len(dependsOn) != 1 || dependsOn[0] != "client-1" {
		t.Errorf("sidecar depends_on = %#v", sidecar["depends_on"])
	}
	volumes, ok := sidecar["volumes"].([]string)
	if !ok || len(volumes) != 1 || volumes[0] != "./vcpe-healthd:/run/vcpe/vcpe-healthd:ro" {
		t.Errorf("sidecar volumes = %#v", sidecar["volumes"])
	}
	entrypoint, ok := sidecar["entrypoint"].([]string)
	if !ok || len(entrypoint) != 1 || entrypoint[0] != "/run/vcpe/vcpe-healthd" {
		t.Errorf("sidecar entrypoint = %#v", sidecar["entrypoint"])
	}
	command, ok := sidecar["command"].([]string)
	if !ok || len(command) != 4 || command[3] != "configured=true" {
		t.Errorf("sidecar command = %#v", sidecar["command"])
	}
	if sidecar["restart"] != "unless-stopped" {
		t.Errorf("sidecar restart = %#v", sidecar["restart"])
	}
}
