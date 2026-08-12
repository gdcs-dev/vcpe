// Package webpa implements the WebPA service type. WebPA has no typed config;
// it generates a compose.yaml and compose.env from the resolved interfaces so
// it can connect to any network role, not just the hardcoded mgmt network.
package webpa

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for WebPA.
const TypeName = "webpa"

type serviceType struct{ typeregistry.BaseServiceType }

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

// Config is the optional configuration for a webpa service.
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
		Name:           "webpa-renderer",
		DecodeConfig:   decodeConfig,
		RenderInstance: renderInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: false}}
}

func (serviceType) Description() string {
	return "USP/WebPA device-management server"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/webpa" }

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
	ipamNone := map[string]bool{}
	for _, n := range input.Deployment.Networks {
		if n.IPAMDriver == "none" {
			ipamNone[n.Role] = true
		}
	}
	instance := input.Service.Instances[0]
	env := render.IfaceEnv(input.Deployment, input.Service, instance)
	env = append(env, render.SortedEnv(cfg.Env)...)
	return render.Result{
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
			{Key: "compose.yaml", Content: renderWebPACompose(input, instance, ipamNone)},
		},
	}, nil
}

// renderWebPACompose generates a compose.yaml that wires up every interface
// from the resolved instance, regardless of role name. This replaces the
// curated services/webpa/compose.yaml so WebPA can connect to any network.
func renderWebPACompose(input render.Input, inst plan.Instance, ipamNone map[string]bool) string {
	topNets := map[string]any{}
	services := map[string]any{}
	svcNets := map[string]any{}
	for _, iface := range inst.Interfaces {
		entry := map[string]any{"mac_address": iface.MAC}
		if !ipamNone[iface.Role] {
			entry["ipv4_address"] = iface.IPv4
		}
		svcNets[iface.Role] = entry
		topNets[iface.Role] = map[string]any{"external": true, "name": iface.Network}
	}
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, inst.Index+1)
	svc := map[string]any{
		"image":          render.ImageRef(input.Service.Image),
		"container_name": input.Deployment.Name + "-" + instanceName,
		"hostname":       instanceName,
		"privileged":     true,
		"cap_add":        []string{"NET_ADMIN", "NET_RAW"},
		"env_file":       []string{fmt.Sprintf("instances/%d/compose.env", inst.Index+1)},
		"networks":       svcNets,
	}
	ports := append([]string(nil), input.Service.Ports...)
	if healthPort := input.HealthPorts[inst.Index]; healthPort != 0 {
		ports = append(ports, fmt.Sprintf("127.0.0.1:%d:9878", healthPort))
	}
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	services[instanceName] = svc
	doc := map[string]any{"services": services, "networks": topNets}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
