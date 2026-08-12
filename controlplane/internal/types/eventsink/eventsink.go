// Package eventsink implements the event-sink service type. Like webpa, it has
// no typed config and renders only the interface environment consumed by the
// curated compose file at services/event-sink/compose.yaml.
package eventsink

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

// TypeName is the manifest discriminator for the event-sink service.
const TypeName = "event-sink"

type serviceType struct{ typeregistry.BaseServiceType }

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

// Config is the optional configuration for an event-sink service.
type Config struct {
	Env map[string]string `yaml:"env,omitempty"`
}

// ValidateConfig accepts an optional env map; all other config is rejected.
func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil
	}
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "event-sink-renderer",
		DecodeConfig:   decodeConfig,
		RenderInstance: renderInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) Description() string {
	return "Generic XMiDT webhook consumer and event logger"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/event-sink" }

func decodeConfig(node yaml.Node) (Config, error) {
	if node.Kind == 0 {
		return Config{}, nil
	}
	var cfg Config
	if err := typeregistry.StrictDecode(node, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func renderInstance(_ context.Context, input render.Input, cfg Config) (render.Result, error) {
	instance := input.Service.Instances[0]
	env := render.IfaceEnv(input.Deployment, input.Service, instance)
	env = append(env, render.SortedEnv(cfg.Env)...)
	services := map[string]any{}
	networks := map[string]any{}
	serviceNetworks := map[string]any{}
	for _, iface := range instance.Interfaces {
		serviceNetworks[iface.Role] = map[string]any{"mac_address": iface.MAC, "ipv4_address": iface.IPv4}
		networks[iface.Role] = map[string]any{"external": true, "name": iface.Network}
	}
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, instance.Index+1)
	svc := map[string]any{
		"image":          render.ImageRef(input.Service.Image),
		"container_name": input.Deployment.Name + "-" + instanceName,
		"hostname":       instanceName,
		"env_file":       []string{fmt.Sprintf("instances/%d/compose.env", instance.Index+1)},
		"networks":       serviceNetworks,
	}
	ports := append([]string(nil), input.Service.Ports...)
	if healthPort := input.HealthPorts[instance.Index]; healthPort != 0 {
		ports = append(ports, fmt.Sprintf("127.0.0.1:%d:9878", healthPort))
	}
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	services[instanceName] = svc
	compose, err := yaml.Marshal(map[string]any{"services": services, "networks": networks})
	if err != nil {
		return render.Result{}, fmt.Errorf("marshal event-sink compose: %w", err)
	}
	return render.Result{
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
			{Key: "compose.yaml", Content: string(compose)},
		},
	}, nil
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
