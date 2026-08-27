package types_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
	"gopkg.in/yaml.v3"
)

func TestRendererArtifactInventory(t *testing.T) {
	types.Register()
	for _, typeName := range []string{"bng", "event-sink", "gateway", "generic-container", "oktopus", "routerd", "webpa", "xb10"} {
		t.Run(typeName, func(t *testing.T) {
			registered, ok := typeregistry.Lookup(typeName)
			if !ok {
				t.Fatalf("%s is not registered", typeName)
			}
			var config yaml.Node
			if err := yaml.Unmarshal([]byte("{}"), &config); err != nil {
				t.Fatalf("unmarshal config: %v", err)
			}
			service := plan.Service{
				Name:     "service",
				Type:     typeName,
				Replicas: 2,
				Image:    manifest.Image{Repository: "example/" + typeName, Tag: "test"},
				Config:   config,
				Instances: []plan.Instance{
					{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}},
					{Index: 1, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:02", IPv4: "10.0.0.3"}}},
				},
			}
			result, err := registered.Renderer().Render(context.Background(), render.Input{
				Deployment:  plan.Deployment{Name: "edge"},
				Service:     service,
				HealthPorts: map[int]int{0: 47000, 1: 47001},
			})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if result.Renderer != registered.Renderer().Name() {
				t.Errorf("renderer identity = %q, want %q", result.Renderer, registered.Renderer().Name())
			}

			artifacts := map[string]string{}
			for _, artifact := range result.Artifacts {
				if _, duplicate := artifacts[artifact.Key]; duplicate {
					t.Errorf("duplicate artifact key %q", artifact.Key)
				}
				artifacts[artifact.Key] = artifact.Content
			}
			for _, key := range []string{"compose.env", "compose.yaml"} {
				if _, ok := artifacts[key]; !ok {
					t.Errorf("missing root artifact %q; got %v", key, artifactKeys(artifacts))
				}
			}
			if typeName == "generic-container" {
				if _, ok := artifacts["entrypoint.sh"]; !ok {
					t.Errorf("missing generic-container entrypoint; got %v", artifactKeys(artifacts))
				}
				for key := range artifacts {
					if strings.HasPrefix(key, "instances/") {
						t.Errorf("generic-container unexpectedly emitted per-instance artifact %q", key)
					}
				}
			} else {
				for _, index := range []int{1, 2} {
					key := fmt.Sprintf("instances/%d/compose.env", index)
					if _, ok := artifacts[key]; !ok {
						t.Errorf("missing one-based replica artifact %q; got %v", key, artifactKeys(artifacts))
					}
				}
			}
			for key := range artifacts {
				if strings.HasPrefix(key, "instances/0/") {
					t.Errorf("zero-based replica artifact %q", key)
				}
			}
		})
	}
}

func artifactKeys(artifacts map[string]string) []string {
	keys := make([]string, 0, len(artifacts))
	for key := range artifacts {
		keys = append(keys, key)
	}
	return keys
}

func TestRenderersPreserveManifestPorts(t *testing.T) {
	types.Register()
	for _, typeName := range []string{"bng", "event-sink", "gateway", "generic-container", "oktopus", "routerd", "webpa", "xb10"} {
		t.Run(typeName, func(t *testing.T) {
			registered, ok := typeregistry.Lookup(typeName)
			if !ok {
				t.Fatalf("%s is not registered", typeName)
			}
			var config yaml.Node
			if err := yaml.Unmarshal([]byte("{}"), &config); err != nil {
				t.Fatalf("unmarshal config: %v", err)
			}
			service := plan.Service{
				Name:   "service",
				Type:   typeName,
				Image:  manifest.Image{Repository: "example/" + typeName, Tag: "test"},
				Ports:  []string{"18080:8080"},
				Config: config,
				Instances: []plan.Instance{{Index: 0, Interfaces: []plan.Interface{{
					Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2",
				}}}},
			}
			result, err := registered.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service, HealthPorts: map[int]int{0: 47000}})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			compose := ""
			for _, artifact := range result.Artifacts {
				if artifact.Key == "compose.yaml" {
					compose = artifact.Content
				}
			}
			if !strings.Contains(compose, "18080:8080") {
				t.Fatalf("compose.yaml does not preserve manifest port:\n%s", compose)
			}
		})
	}
}

func TestRendererComposeSemantics(t *testing.T) {
	types.Register()
	tests := []struct {
		typeName        string
		healthPublished bool
		pinsIPv4        bool
		pinsMAC         bool
		externalName    string
		containerName   string
		hostname        string
	}{
		{typeName: "bng", healthPublished: true, pinsIPv4: true, pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
		{typeName: "event-sink", healthPublished: true, pinsIPv4: true, pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
		{typeName: "gateway", pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
		{typeName: "generic-container", externalName: "${IFACE_MGMT_NETWORK}", containerName: "${DEPLOYMENT_NAME}-${SERVICE_NAME}-%d", hostname: "${SERVICE_NAME}-%d"},
		{typeName: "oktopus", healthPublished: true, pinsIPv4: true, pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
		{typeName: "routerd", healthPublished: true, pinsIPv4: true, pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
		{typeName: "webpa", healthPublished: true, pinsIPv4: true, pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
		{typeName: "xb10", healthPublished: true, pinsMAC: true, externalName: "edge-mgmt", containerName: "edge-service-%d", hostname: "service-%d"},
	}
	for _, testCase := range tests {
		t.Run(testCase.typeName, func(t *testing.T) {
			registered, ok := typeregistry.Lookup(testCase.typeName)
			if !ok {
				t.Fatalf("%s is not registered", testCase.typeName)
			}
			var config yaml.Node
			if err := yaml.Unmarshal([]byte("{}"), &config); err != nil {
				t.Fatalf("unmarshal config: %v", err)
			}
			service := plan.Service{
				Name:     "service",
				Type:     testCase.typeName,
				Replicas: 2,
				Image:    manifest.Image{Repository: "example/" + testCase.typeName, Tag: "test"},
				Ports:    []string{"18080:8080"},
				Config:   config,
				Instances: []plan.Instance{
					{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}},
					{Index: 1, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:02", IPv4: "10.0.0.3"}}},
				},
			}
			result, err := registered.Renderer().Render(context.Background(), render.Input{
				Deployment:  plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}},
				Service:     service,
				HealthPorts: map[int]int{0: 47000, 1: 47001},
			})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			compose := composeDocument(t, result)
			services := composeMap(t, compose["services"], "services")
			for index := 1; index <= 2; index++ {
				name := fmt.Sprintf("service-%d", index)
				workload := composeMap(t, services[name], "services."+name)
				if want := fmt.Sprintf(testCase.containerName, index); workload["container_name"] != want {
					t.Errorf("container_name = %#v, want %q", workload["container_name"], want)
				}
				if want := fmt.Sprintf(testCase.hostname, index); workload["hostname"] != want {
					t.Errorf("hostname = %#v, want %q", workload["hostname"], want)
				}
				if !containsComposeValue(workload["ports"], "18080:8080") {
					t.Errorf("ports = %#v, want manifest port", workload["ports"])
				}
				attachment := composeMap(t, composeMap(t, workload["networks"], "services."+name+".networks")["mgmt"], "services."+name+".networks.mgmt")
				if _, ok := attachment["mac_address"]; ok != testCase.pinsMAC {
					t.Errorf("mac_address present = %t, want %t", ok, testCase.pinsMAC)
				}
				if _, ok := attachment["ipv4_address"]; ok != testCase.pinsIPv4 {
					t.Errorf("ipv4_address present = %t, want %t", ok, testCase.pinsIPv4)
				}
				if testCase.healthPublished && !containsComposeValue(workload["ports"], fmt.Sprintf("127.0.0.1:%d:9878", 46999+index)) {
					t.Errorf("ports = %#v, want health publication for replica %d", workload["ports"], index)
				}
			}
			networks := composeMap(t, compose["networks"], "networks")
			mgmt := composeMap(t, networks["mgmt"], "networks.mgmt")
			if got := mgmt["external"]; got != true {
				t.Errorf("networks.mgmt.external = %#v, want true", got)
			}
			if got := mgmt["name"]; got != testCase.externalName {
				t.Errorf("networks.mgmt.name = %#v, want %q", got, testCase.externalName)
			}
		})
	}
}

func composeDocument(t *testing.T, result render.Result) map[string]any {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Key != "compose.yaml" {
			continue
		}
		var document map[string]any
		if err := yaml.Unmarshal([]byte(artifact.Content), &document); err != nil {
			t.Fatalf("parse compose.yaml: %v", err)
		}
		return document
	}
	t.Fatal("missing compose.yaml artifact")
	return nil
}

func composeMap(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	mapping, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want mapping", path, value)
	}
	return mapping
}

func containsComposeValue(value any, want string) bool {
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
