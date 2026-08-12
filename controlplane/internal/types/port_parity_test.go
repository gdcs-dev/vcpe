package types_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
	"gopkg.in/yaml.v3"
)

func TestRenderersPreserveManifestPorts(t *testing.T) {
	types.Register()
	for _, typeName := range []string{"bng", "event-sink", "gateway", "generic-container", "oktopus", "webpa", "xb10"} {
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
