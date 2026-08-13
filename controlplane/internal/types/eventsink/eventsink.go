// Package eventsink implements the event-sink service type. Like webpa, it has
// no typed config and renders only the interface environment consumed by the
// curated compose file at services/event-sink/compose.yaml.
package eventsink

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

// alwaysPinAttachment always pins mac_address and ipv4_address, regardless of
// the network's managed status. This is event-sink's existing behavior,
// distinct from the shared DefaultAttachment's managed-conditional ipv4
// pinning; preserved here rather than folded into a shared helper.
func alwaysPinAttachment(iface plan.Interface, _ bool) map[string]any {
	return map[string]any{"mac_address": iface.MAC, "ipv4_address": iface.IPv4}
}

func renderInstance(_ context.Context, input render.Input, cfg Config) (render.Result, error) {
	instance := input.Service.Instances[0]
	env := render.IfaceEnv(input.Deployment, input.Service, instance)
	env = append(env, render.SortedEnv(cfg.Env)...)

	svc, networks := servicetemplate.BuildComposeService(input, instance, alwaysPinAttachment)
	svc["privileged"] = true
	svc["cap_add"] = []string{"NET_ADMIN", "NET_RAW"}
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, instance.Index+1)
	ports := append([]string(nil), input.Service.Ports...)
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	svcNets, _ := svc["networks"].(map[string]any)
	// Register the bare "event-sink" hostname as an aardvark-dns network
	// alias on mgmt so devices attached directly to the mgmt network (not
	// just BNG DHCP clients relaying through BNG's dnsmasq CNAME) — including
	// Caduceus delivering webhook callbacks to WEBHOOK_URL — can resolve it.
	// Only the first instance claims the alias, matching the
	// firstInstanceAlias convention used elsewhere (bng.go) to avoid
	// ambiguity across replicas.
	if instance.Index == 0 {
		if mgmt, ok := svcNets["mgmt"].(map[string]any); ok {
			mgmt["aliases"] = []string{"event-sink"}
		}
	}
	servicetemplate.AttachHealthPublication(input, instance, input.HealthPorts[instance.Index], networks, svcNets, svc)
	services := map[string]any{instanceName: svc}
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
