// Package eventsink implements the event-sink service type. Like webpa, it has
// no typed config and renders only the interface environment consumed by the
// curated compose file at services/event-sink/compose.yaml.
package eventsink

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for the event-sink service.
const TypeName = "event-sink"

type serviceType struct{}

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

func (serviceType) Renderer() render.Renderer { return renderer{} }

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) Health() typeregistry.HealthBehavior {
	return typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878}
}

func (serviceType) DefaultImagePolicy() string { return "build" }

func (serviceType) ValidateInterfaces(_ []manifest.Interface) error { return nil }

func (serviceType) Description() string {
	return "Generic XMiDT webhook consumer and event logger"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/event-sink" }

type renderer struct{}

func (renderer) Name() string { return "event-sink-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("event-sink %q has no instances", input.Service.Name)
	}
	var cfg Config
	_ = typeregistry.StrictDecode(input.Service.Config, &cfg)
	artifacts := []render.Artifact{}
	services := map[string]any{}
	networks := map[string]any{}
	for _, inst := range input.Service.Instances {
		env := render.IfaceEnv(input.Deployment, input.Service, inst)
		if len(cfg.Env) > 0 {
			extra := make([]string, 0, len(cfg.Env))
			for k, v := range cfg.Env {
				extra = append(extra, k+"="+v)
			}
			sort.Strings(extra)
			env = append(env, extra...)
		}
		content := strings.Join(env, "\n") + "\n"
		if inst.Index == 0 {
			artifacts = append(artifacts, render.Artifact{Key: "compose.env", Content: content})
		}
		artifacts = append(artifacts, render.Artifact{Key: fmt.Sprintf("instances/%d/compose.env", inst.Index+1), Content: content})

		serviceNetworks := map[string]any{}
		for _, iface := range inst.Interfaces {
			serviceNetworks[iface.Role] = map[string]any{"mac_address": iface.MAC, "ipv4_address": iface.IPv4}
			networks[iface.Role] = map[string]any{"external": true, "name": iface.Network}
		}
		instanceName := fmt.Sprintf("%s-%d", input.Service.Name, inst.Index+1)
		svc := map[string]any{
			"image":          render.ImageRef(input.Service.Image),
			"container_name": input.Deployment.Name + "-" + instanceName,
			"hostname":       instanceName,
			"env_file":       []string{fmt.Sprintf("instances/%d/compose.env", inst.Index+1)},
			"networks":       serviceNetworks,
		}
		ports := append([]string(nil), input.Service.Ports...)
		if healthPort := input.HealthPorts[inst.Index]; healthPort != 0 {
			ports = append(ports, fmt.Sprintf("127.0.0.1:%d:9878", healthPort))
		}
		if len(ports) > 0 {
			svc["ports"] = ports
		}
		services[instanceName] = svc
	}
	compose, err := yaml.Marshal(map[string]any{"services": services, "networks": networks})
	if err != nil {
		return render.Result{}, fmt.Errorf("marshal event-sink compose: %w", err)
	}
	artifacts = append(artifacts, render.Artifact{Key: "compose.yaml", Content: string(compose)})
	return render.Result{
		Renderer:  "event-sink-renderer",
		Artifacts: artifacts,
	}, nil
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
