// Package oktopus implements the Oktopus USP controller service type.
// It renders a compose.env that provides the IFACE_* contract env vars plus
// any deployment-specific overrides from the manifest config block, and a
// compose.yaml that wires the container to the mgmt network and includes any
// port mappings declared in the manifest service ports field.
package oktopus

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

// TypeName is the manifest discriminator for the Oktopus USP controller.
const TypeName = "oktopus"

// Config holds the optional manifest-level overrides for the Oktopus container.
// All fields are optional; unset fields fall back to the defaults baked into
// services/oktopus/oktopus.env inside the image.
type Config struct {
	// Admin account seeded on first boot.
	AdminEmail    string `yaml:"adminEmail,omitempty"`
	AdminName     string `yaml:"adminName,omitempty"`
	AdminPassword string `yaml:"adminPassword,omitempty"`

	// NATS credentials — must match the nats.cfg baked into the image.
	NATSUser     string `yaml:"natsUser,omitempty"`
	NATSPassword string `yaml:"natsPassword,omitempty"`

	// STOMP credentials (blank = no auth).
	STOMPUser     string `yaml:"stompUser,omitempty"`
	STOMPPassword string `yaml:"stompPassword,omitempty"`

	// TaaS (USP conformance runner).
	TaaSAPIKey string `yaml:"taasAPIKey,omitempty"`

	// Extra arbitrary env vars passed through to /etc/oktopus/oktopus.env.
	Env map[string]string `yaml:"env,omitempty"`
}

type serviceType struct{ typeregistry.BaseServiceType }

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil // config block is optional
	}
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "oktopus-renderer",
		DecodeConfig:   decodeConfig,
		RenderInstance: renderInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) Description() string {
	return "Oktopus USP controller — cloud-native device management platform"
}

func (serviceType) DefaultImage() string { return "" }

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
	return render.Result{
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(oktopusEnv(input, instance, cfg), "\n") + "\n"},
			{Key: "compose.yaml", Content: generateCompose(input, instance)},
		},
	}, nil
}

// generateCompose builds the compose.yaml for the oktopus container, wiring
// the mgmt network attachment and any port mappings from the manifest.
func generateCompose(input render.Input, inst plan.Instance) string {
	svc, networks := servicetemplate.BuildComposeService(input, inst, alwaysPinAttachment)
	svc["privileged"] = true
	svc["cap_add"] = []string{"NET_ADMIN", "NET_RAW"}
	if len(input.Service.Volumes) > 0 {
		svc["volumes"] = input.Service.Volumes
	}
	ports := append([]string(nil), input.Service.Ports...)
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	svcNets, _ := svc["networks"].(map[string]any)
	// Register the bare "oktopus" hostname as an aardvark-dns network alias
	// on mgmt so devices attached directly to the mgmt network (not just BNG
	// DHCP clients relaying through BNG's dnsmasq CNAME) can resolve it. Only
	// the first instance claims the alias, matching the firstInstanceAlias
	// convention used elsewhere (bng.go) to avoid ambiguity across replicas.
	if inst.Index == 0 {
		if mgmt, ok := svcNets["mgmt"].(map[string]any); ok {
			mgmt["aliases"] = []string{"oktopus"}
		}
	}
	servicetemplate.AttachHealthPublication(input, inst, input.HealthPorts[inst.Index], networks, svcNets, svc)
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, inst.Index+1)
	out, _ := yaml.Marshal(map[string]any{"services": map[string]any{instanceName: svc}, "networks": networks})
	return string(out)
}

// alwaysPinAttachment always pins mac_address and ipv4_address, regardless of
// the network's managed status. This is oktopus's existing behavior, distinct
// from the shared DefaultAttachment's managed-conditional ipv4 pinning;
// preserved here rather than folded into a shared helper.
func alwaysPinAttachment(iface plan.Interface, _ bool) map[string]any {
	return map[string]any{"mac_address": iface.MAC, "ipv4_address": iface.IPv4}
}

func oktopusEnv(input render.Input, instance plan.Instance, cfg Config) []string {
	lines := render.IfaceEnv(input.Deployment, input.Service, instance)
	for _, pair := range [][2]string{{"ADMIN_EMAIL", cfg.AdminEmail}, {"ADMIN_NAME", cfg.AdminName}, {"ADMIN_PASSWORD", cfg.AdminPassword}, {"NATS_USER", cfg.NATSUser}, {"NATS_PW", cfg.NATSPassword}, {"STOMP_USER", cfg.STOMPUser}, {"STOMP_PASSWD", cfg.STOMPPassword}, {"SECRET_API_KEY", cfg.TaaSAPIKey}} {
		lines = appendIfSet(lines, pair[0], pair[1])
	}
	if cfg.NATSUser != "" || cfg.NATSPassword != "" {
		user, pw := cfg.NATSUser, cfg.NATSPassword
		if user == "" {
			user = "oktopususer"
		}
		if pw == "" {
			pw = "oktopuspw"
		}
		lines = append(lines, fmt.Sprintf("NATS_URL=nats://%s:%s@localhost:4222", user, pw))
	}
	for k, v := range cfg.Env {
		lines = append(lines, k+"="+v)
	}
	return lines
}

func appendIfSet(lines []string, key, value string) []string {
	if value == "" {
		return lines
	}
	return append(lines, key+"="+value)
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
