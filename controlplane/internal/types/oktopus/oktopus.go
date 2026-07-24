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

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
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

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil // config block is optional
	}
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer { return renderer{} }

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) DefaultImagePolicy() string { return "build" }

func (serviceType) ValidateInterfaces(_ []manifest.Interface) error { return nil }

func (serviceType) Description() string {
	return "Oktopus USP controller — cloud-native device management platform"
}

func (serviceType) DefaultImage() string { return "" }

type renderer struct{}

func (renderer) Name() string { return "oktopus-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("oktopus %q has no instances", input.Service.Name)
	}

	var cfg Config
	if input.Service.Config.Kind != 0 {
		if err := typeregistry.StrictDecode(input.Service.Config, &cfg); err != nil {
			return render.Result{}, fmt.Errorf("oktopus %q: %w", input.Service.Name, err)
		}
	}

	// Base interface env vars (DEPLOYMENT_NAME, SERVICE_NAME, IMAGE, IFACE_*).
	lines := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])

	// Append Oktopus-specific overrides derived from the manifest config.
	lines = appendIfSet(lines, "ADMIN_EMAIL", cfg.AdminEmail)
	lines = appendIfSet(lines, "ADMIN_NAME", cfg.AdminName)
	lines = appendIfSet(lines, "ADMIN_PASSWORD", cfg.AdminPassword)
	lines = appendIfSet(lines, "NATS_USER", cfg.NATSUser)
	lines = appendIfSet(lines, "NATS_PW", cfg.NATSPassword)
	lines = appendIfSet(lines, "STOMP_USER", cfg.STOMPUser)
	lines = appendIfSet(lines, "STOMP_PASSWD", cfg.STOMPPassword)
	lines = appendIfSet(lines, "SECRET_API_KEY", cfg.TaaSAPIKey)

	// Derive NATS_URL from credentials if either is overridden.
	if cfg.NATSUser != "" || cfg.NATSPassword != "" {
		user := cfg.NATSUser
		if user == "" {
			user = "oktopususer"
		}
		pw := cfg.NATSPassword
		if pw == "" {
			pw = "oktopuspw"
		}
		lines = append(lines, fmt.Sprintf("NATS_URL=nats://%s:%s@localhost:4222", user, pw))
	}

	// Pass through arbitrary extra env vars from the manifest.
	for k, v := range cfg.Env {
		lines = append(lines, k+"="+v)
	}

	composeYAML := generateCompose(input)

	return render.Result{
		Renderer: "oktopus-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(lines, "\n") + "\n"},
			{Key: "compose.yaml", Content: composeYAML},
		},
	}, nil
}

// generateCompose builds the compose.yaml for the oktopus container, wiring
// the mgmt network attachment and any port mappings from the manifest.
func generateCompose(input render.Input) string {
	inst := input.Service.Instances[0]

	// Build network entries from interfaces.
	topNets := &strings.Builder{}
	svcNets := &strings.Builder{}
	for _, iface := range inst.Interfaces {
		key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
		fmt.Fprintf(topNets, "  %s:\n    external: true\n    name: ${IFACE_%s_NETWORK}\n", iface.Role, key)
		fmt.Fprintf(svcNets, "      %s:\n        mac_address: ${IFACE_%s_MAC}\n        ipv4_address: ${IFACE_%s_IPV4}\n", iface.Role, key, key)
	}

	// Build ports section.
	portLines := &strings.Builder{}
	for _, p := range input.Service.Ports {
		fmt.Fprintf(portLines, "      - %q\n", p)
	}

	var b strings.Builder
	b.WriteString("# Generated by oktopus-renderer — do not edit by hand.\n")
	b.WriteString("services:\n")
	fmt.Fprintf(&b, "  %s:\n", input.Service.Name)
	fmt.Fprintf(&b, "    image: %s\n", render.ImageRef(input.Service.Image))
	fmt.Fprintf(&b, "    container_name: ${DEPLOYMENT_NAME}-%s\n", input.Service.Name)
	fmt.Fprintf(&b, "    hostname: %s\n", input.Service.Name)
	b.WriteString("    privileged: true\n")
	b.WriteString("    cap_add:\n      - NET_ADMIN\n      - NET_RAW\n")
	b.WriteString("    env_file:\n      - compose.env\n")
	if portLines.Len() > 0 {
		b.WriteString("    ports:\n")
		b.WriteString(portLines.String())
	}
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./runtime/mongo:/var/lib/mongodb\n")
	b.WriteString("      - ./runtime/nats:/var/lib/nats/jetstream\n")
	for _, v := range input.Service.Volumes {
		fmt.Fprintf(&b, "      - %s\n", v)
	}
	if svcNets.Len() > 0 {
		b.WriteString("    networks:\n")
		b.WriteString(svcNets.String())
	}
	if topNets.Len() > 0 {
		b.WriteString("\nnetworks:\n")
		b.WriteString(topNets.String())
	}
	return b.String()
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
