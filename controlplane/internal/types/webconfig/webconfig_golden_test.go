package webconfig_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/webconfig"
	"gopkg.in/yaml.v3"
)

func TestWebConfigGoldenCompose(t *testing.T) {
	webconfig.Register()
	serviceType, ok := typeregistry.Lookup(webconfig.TypeName)
	if !ok {
		t.Fatal("webconfig is not registered")
	}

	service := plan.Service{
		Name:  webconfig.TypeName,
		Type:  webconfig.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/webconfig", Tag: "dev"},
		Ports: []string{"9000:9000"},
		Instances: []plan.Instance{{
			Index: 0,
			Interfaces: []plan.Interface{{
				Role: "mgmt", Network: "edge-mgmt", Device: "eth0",
				MAC: "02:00:00:00:00:09", IPv4: "10.10.10.5", ManagedNetwork: true,
			}},
		}},
	}
	result, err := serviceType.Renderer().Render(context.Background(), render.Input{
		Deployment:  plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}},
		Service:     service,
		HealthPorts: map[int]int{0: 47000},
	})
	if err != nil {
		t.Fatalf("render webconfig: %v", err)
	}

	artifacts := map[string]string{}
	for _, artifact := range result.Artifacts {
		artifacts[artifact.Key] = artifact.Content
	}
	for _, key := range []string{"compose.env", "instances/1/compose.env", "compose.yaml"} {
		if _, ok := artifacts[key]; !ok {
			t.Errorf("missing artifact %q: %v", key, artifacts)
		}
	}
	compose := artifacts["compose.yaml"]
	for _, want := range []string{
		"webconfig-1:",
		"image: ghcr.io/gdcs-dev/webconfig:dev",
		"name: edge-mgmt",
		"- webconfig",
		"9000:9000",
		"127.0.0.1:47000:9878",
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml missing %q:\n%s", want, compose)
		}
	}
	if strings.Count(compose, "aliases:") != 1 {
		t.Errorf("compose.yaml aliases = %d, want one first-instance mgmt alias:\n%s", strings.Count(compose, "aliases:"), compose)
	}
}

func TestWebConfigRejectsUnknownConfigAndMultipleReplicas(t *testing.T) {
	webconfig.Register()
	serviceType, ok := typeregistry.Lookup(webconfig.TypeName)
	if !ok {
		t.Fatal("webconfig is not registered")
	}
	if err := serviceType.ValidateConfig(manifestNode(t, "fixture: custom")); err == nil {
		t.Fatal("expected WebConfig to reject unknown configuration")
	}
	validator, ok := serviceType.(typeregistry.ReplicaValidator)
	if !ok {
		t.Fatal("WebConfig does not implement replica validation")
	}
	if err := validator.ValidateReplicas(1); err != nil {
		t.Fatalf("ValidateReplicas(1) error = %v", err)
	}
	if err := validator.ValidateReplicas(2); err == nil || !strings.Contains(err.Error(), "singleton") {
		t.Fatalf("ValidateReplicas(2) error = %v, want singleton rejection", err)
	}
}

func manifestNode(t *testing.T, content string) yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(content), &node); err != nil {
		t.Fatal(err)
	}
	return node
}