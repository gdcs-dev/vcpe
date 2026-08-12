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
)

func TestReplicaHealthMappings(t *testing.T) {
	types.Register()
	for _, serviceType := range []string{"xb10", "oktopus"} {
		for _, replicaCount := range []int{1, 2} {
			t.Run(serviceType+"-replicas-"+string(rune('0'+replicaCount)), func(t *testing.T) {
				registered, ok := typeregistry.Lookup(serviceType)
				if !ok {
					t.Fatalf("%s type is not registered", serviceType)
				}
				service := plan.Service{
					Name:  serviceType,
					Type:  serviceType,
					Image: manifest.Image{Repository: "example/" + serviceType, Tag: "test"},
					Ports: []string{"8080:8080"},
				}
				healthPorts := map[int]int{}
				for index := 0; index < replicaCount; index++ {
					service.Instances = append(service.Instances, plan.Instance{Index: index, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}})
					healthPorts[index] = 47000 + index
				}
				result, err := registered.Renderer().Render(context.Background(), render.Input{Deployment: plan.Deployment{Name: "edge"}, Service: service, HealthPorts: healthPorts})
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				var compose string
				for _, artifact := range result.Artifacts {
					if artifact.Key == "compose.yaml" {
						compose = artifact.Content
					}
				}
				expected := []string{serviceType + "-1:", "8080:8080", "127.0.0.1:47000:9878"}
				if replicaCount == 2 {
					expected = append(expected, serviceType+"-2:", "127.0.0.1:47001:9878")
				}
				for _, expected := range expected {
					if !strings.Contains(compose, expected) {
						t.Errorf("compose.yaml missing %q:\n%s", expected, compose)
					}
				}
				for _, forbidden := range []string{"0.0.0.0:47000:9878", "edge-mgmt:47000:9878"} {
					if strings.Contains(compose, forbidden) {
						t.Errorf("compose.yaml exposes health endpoint as %q:\n%s", forbidden, compose)
					}
				}
			})
		}
	}
}
