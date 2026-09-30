// Package webconfig implements the development-only WebConfig service type.
package webconfig

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for WebConfig.
const TypeName = "webconfig"

type serviceType struct{ typeregistry.BaseServiceType }

var (
	_ typeregistry.ServiceType      = serviceType{}
	_ typeregistry.ReplicaValidator = serviceType{}
)

func (serviceType) Type() string { return TypeName }

// ValidateConfig rejects unrecognized WebConfig configuration.
func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil
	}
	return typeregistry.StrictDecode(node, &struct{}{})
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[struct{}]{
		Name:           "webconfig-renderer",
		DecodeConfig:   decodeConfig,
		RenderInstance: renderInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) Description() string {
	return "Development-only WebConfig server for Gateway webcfg clients"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/webconfig" }

func (serviceType) ValidateReplicas(replicas int) error {
	if replicas > 1 {
		return fmt.Errorf("WebConfig is singleton; got %d replicas", replicas)
	}
	return nil
}

func decodeConfig(node yaml.Node) (struct{}, error) {
	if node.Kind == 0 {
		return struct{}{}, nil
	}
	if err := typeregistry.StrictDecode(node, &struct{}{}); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}

func renderInstance(_ context.Context, input render.Input, _ struct{}) (render.Result, error) {
	instance := input.Service.Instances[0]
	environment := render.IfaceEnv(input.Deployment, input.Service, instance)

	service, networks := servicetemplate.BuildComposeService(input, instance, servicetemplate.DefaultAttachment)
	if len(input.Service.Ports) > 0 {
		service["ports"] = append([]string(nil), input.Service.Ports...)
	}
	serviceNetworks, _ := service["networks"].(map[string]any)
	servicetemplate.AddFirstInstanceNetworkAliases(instance, serviceNetworks, "mgmt", []string{TypeName})
	servicetemplate.AttachHealthPublication(input, instance, input.HealthPorts[instance.Index], 9878, networks, serviceNetworks, service)

	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, instance.Index+1)
	compose, err := yaml.Marshal(map[string]any{
		"services": map[string]any{instanceName: service},
		"networks": networks,
	})
	if err != nil {
		return render.Result{}, fmt.Errorf("marshal WebConfig compose: %w", err)
	}
	return render.Result{Artifacts: []render.Artifact{
		{Key: "compose.env", Content: strings.Join(environment, "\n") + "\n"},
		{Key: "compose.yaml", Content: string(compose)},
	}}, nil
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once