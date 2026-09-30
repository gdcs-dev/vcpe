// Package telemetrygateway implements the embedded telemetry-gateway service type.
package telemetrygateway

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

const TypeName = "telemetry-gateway"

type serviceType struct{ typeregistry.BaseServiceType }

var (
	_ typeregistry.ServiceType      = serviceType{}
	_ typeregistry.ReplicaValidator = serviceType{}
)

type Config struct {
	Env map[string]string `yaml:"env,omitempty"`
}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	_, err := decodeConfig(node)
	return err
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "telemetry-gateway-renderer",
		DecodeConfig:   decodeConfig,
		RenderInstance: renderInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) Health() typeregistry.HealthBehavior {
	return typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 8080}
}

func (serviceType) Description() string {
	return "Embedded telemetry gateway with local PostgreSQL and observability services"
}

func (serviceType) DefaultImage() string {
	return "ghcr.io/gdcs-dev/telemetry-gateway"
}

func (serviceType) ValidateReplicas(replicas int) error {
	if replicas > 1 {
		return fmt.Errorf("embedded topology is singleton; got %d replicas", replicas)
	}
	return nil
}

func decodeConfig(node yaml.Node) (Config, error) {
	if node.Kind == 0 {
		return Config{}, nil
	}
	var config Config
	if err := typeregistry.StrictDecode(node, &config); err != nil {
		return Config{}, err
	}
	if err := validateStringMap(node, "env"); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateStringMap(node yaml.Node, field string) error {
	root := &node
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != field {
			continue
		}
		mapping := root.Content[index+1]
		if mapping.Kind != yaml.MappingNode {
			return fmt.Errorf("%s must be a mapping", field)
		}
		for valueIndex := 1; valueIndex < len(mapping.Content); valueIndex += 2 {
			if mapping.Content[valueIndex].Tag != "!!str" {
				return fmt.Errorf("%s.%s must be a string", field, mapping.Content[valueIndex-1].Value)
			}
		}
	}
	return nil
}

func renderInstance(_ context.Context, input render.Input, config Config) (render.Result, error) {
	instance := input.Service.Instances[0]
	environment := render.IfaceEnv(input.Deployment, input.Service, instance)
	environment = append(environment, render.SortedEnv(config.Env)...)

	service, networks := servicetemplate.BuildComposeService(input, instance, servicetemplate.DefaultAttachment)
	service["restart"] = "unless-stopped"
	if len(input.Service.Ports) > 0 {
		service["ports"] = append([]string(nil), input.Service.Ports...)
	}
	service["volumes"] = []string{
		"postgres-data:/var/lib/postgresql/data",
		"prometheus-data:/var/lib/prometheus/metrics2",
		"grafana-data:/var/lib/grafana",
	}
	serviceNetworks, _ := service["networks"].(map[string]any)
	servicetemplate.AddFirstInstanceNetworkAliases(instance, serviceNetworks, "mgmt", []string{TypeName})
	servicetemplate.AttachHealthPublication(input, instance, input.HealthPorts[instance.Index], 8080, networks, serviceNetworks, service)

	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, instance.Index+1)
	compose, err := yaml.Marshal(map[string]any{
		"services": map[string]any{instanceName: service},
		"networks": networks,
		"volumes": map[string]any{
			"postgres-data":   map[string]any{},
			"prometheus-data": map[string]any{},
			"grafana-data":    map[string]any{},
		},
	})
	if err != nil {
		return render.Result{}, fmt.Errorf("marshal telemetry-gateway compose: %w", err)
	}
	return render.Result{Artifacts: []render.Artifact{
		{Key: "compose.env", Content: strings.Join(environment, "\n") + "\n"},
		{Key: "compose.yaml", Content: string(compose)},
	}}, nil
}

func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
