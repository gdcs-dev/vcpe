// Package genericcontainer implements the catch-all service type. It renders a
// generated compose file plus an environment file from a small typed config and
// the resolved interface identities. It absorbs the former bespoke "client"
// service, which is now just a generic container.
package genericcontainer

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for generic containers.
const TypeName = "generic-container"

// Config is the typed configuration for a generic container.
type Config struct {
	Command []string          `yaml:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Ports   []string          `yaml:"ports,omitempty"`
	Volumes []string          `yaml:"volumes,omitempty"`
	Health  *HealthConfig     `yaml:"health,omitempty"`
}

// HealthConfig is the explicit readiness policy for a generic workload.
type HealthConfig struct {
	HTTP           *HTTPHealthProbe    `yaml:"http,omitempty"`
	Command        *CommandHealthProbe `yaml:"command,omitempty"`
	TimeoutSeconds int                 `yaml:"timeoutSeconds,omitempty"`
}

type HTTPHealthProbe struct {
	URL            string `yaml:"url"`
	ExpectedStatus int    `yaml:"expectedStatus,omitempty"`
}

type CommandHealthProbe struct {
	Command string `yaml:"command"`
}

// entrypointSH is the init script embedded in every generic-container render.
// It reads VCPE_INIT_* vars to perform identity → network → dns → exec.
const entrypointSH = `#!/bin/sh
set -e

# ── Identity ─────────────────────────────────────────────────────────────────

_vcpe_hostname="${VCPE_INIT_HOSTNAME:-}"
if [ -n "$_vcpe_hostname" ]; then
    hostname "$_vcpe_hostname"
fi

if [ -n "${VCPE_INIT_MAC_ROLE:-}" ]; then
    _mac_key=$(printf '%s' "$VCPE_INIT_MAC_ROLE" | tr 'a-z-' 'A-Z_')
    _mac_dev=$(eval "printf '%s' \"\${IFACE_${_mac_key}_DEVICE:-}\"")
    _mac_val=$(eval "printf '%s' \"\${IFACE_${_mac_key}_MAC:-}\"")
    if [ -n "$_mac_dev" ] && [ -n "$_mac_val" ]; then
        ip link set "$_mac_dev" down
        ip link set "$_mac_dev" address "$_mac_val"
        ip link set "$_mac_dev" up
    fi
fi

# ── Network ───────────────────────────────────────────────────────────────────
# Every declared interface (any IFACE_<ROLE>_DEVICE present) is initialized
# according to its own IFACE_<ROLE>_ADDRESSING (dhcp or static), replacing the
# former single default-route-interface special case.

for _if_var in $(env | awk -F= '$1 ~ /^IFACE_.*_DEVICE$/ {print $1}'); do
    _if_prefix="${_if_var%_DEVICE}"
    _if_dev=$(eval "printf '%s' \"\${${_if_prefix}_DEVICE:-}\"")
    [ -n "$_if_dev" ] || continue
    _if_addressing=$(eval "printf '%s' \"\${${_if_prefix}_ADDRESSING:-dhcp}\"")
    _if_default_route=$(eval "printf '%s' \"\${${_if_prefix}_DEFAULT_ROUTE:-}\"")
    _if_managed=$(eval "printf '%s' \"\${${_if_prefix}_NETWORK_MANAGED:-}\"")

    ip link set "$_if_dev" up 2>/dev/null || true

    if [ "$_if_addressing" = "static" ]; then
        _if_ipv4=$(eval "printf '%s' \"\${${_if_prefix}_IPV4:-}\"")
        _if_gw4=$(eval  "printf '%s' \"\${${_if_prefix}_GATEWAY4:-}\"")
        [ -n "$_if_ipv4" ] && ip addr add "$_if_ipv4" dev "$_if_dev"
        if [ "$_if_default_route" = "1" ] && [ -n "$_if_gw4" ]; then
            ip route replace default via "$_if_gw4" dev "$_if_dev"
        fi
    elif [ -z "$_if_managed" ]; then
        _dhcp_host="${_vcpe_hostname:-$(hostname)}"
        _n=0
        until udhcpc -n -q -i "$_if_dev" -x "hostname:${_dhcp_host}" 2>/dev/null; do
            _n=$((_n + 1))
            [ "$_n" -ge 100 ] && break
            sleep 3
        done
        # Only the interface marked defaultRoute keeps a default route; a
        # DHCP offer's router option on any other interface must not race
        # with (and override) the intended default-route interface's.
        if [ "$_if_default_route" != "1" ]; then
            ip route del default dev "$_if_dev" 2>/dev/null || true
        fi
    fi
done

# ── DNS override ─────────────────────────────────────────────────────────────

if [ -n "${VCPE_INIT_NAMESERVER_FROM:-}" ]; then
    _ns_key=$(printf '%s' "$VCPE_INIT_NAMESERVER_FROM" | tr 'a-z-' 'A-Z_')
    _ns_val=$(eval "printf '%s' \"\${IFACE_${_ns_key}_GATEWAY4:-}\"")
    [ -n "$_ns_val" ] && printf 'nameserver %s\n' "$_ns_val" > /etc/resolv.conf
elif [ "${VCPE_INIT_NAMESERVER_FROM_ROUTE:-}" = "1" ]; then
    # Wait separately for the default route — udhcpc sets it just after the IP.
    _n=0
    _ns_val=""
    while [ "$_n" -lt 10 ]; do
        _ns_val=$(ip route show default 2>/dev/null | awk '/default via/{print $3; exit}')
        [ -n "$_ns_val" ] && break
        sleep 0.5
        _n=$((_n + 1))
    done
    [ -n "$_ns_val" ] && printf 'nameserver %s\n' "$_ns_val" > /etc/resolv.conf
elif [ -n "${VCPE_INIT_NAMESERVER:-}" ]; then
    printf 'nameserver %s\n' "$VCPE_INIT_NAMESERVER" > /etc/resolv.conf
fi

# ── Settle delay ─────────────────────────────────────────────────────────────

[ -n "${VCPE_INIT_SLEEP:-}" ] && sleep "$VCPE_INIT_SLEEP"

exec "$@"
`

type serviceType struct {
	typeregistry.BaseServiceType
}

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	_, err := decodeConfig(node)
	return err
}

func decodeConfig(node yaml.Node) (Config, error) {
	var config Config
	if err := typeregistry.StrictDecode(node, &config); err != nil {
		return Config{}, err
	}
	if err := validateHealth(config.Health); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateHealth(config *HealthConfig) error {
	if config == nil {
		return nil
	}
	if config.TimeoutSeconds < 1 || config.TimeoutSeconds > 30 {
		return fmt.Errorf("health.timeoutSeconds must be between 1 and 30")
	}
	if (config.HTTP == nil) == (config.Command == nil) {
		return fmt.Errorf("health requires exactly one of http or command")
	}
	if config.HTTP != nil {
		parsed, err := url.Parse(config.HTTP.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("health.http.url must be an absolute HTTP URL")
		}
		if config.HTTP.ExpectedStatus != 0 && (config.HTTP.ExpectedStatus < 100 || config.HTTP.ExpectedStatus > 599) {
			return fmt.Errorf("health.http.expectedStatus must be a valid HTTP status")
		}
	}
	if config.Command != nil && strings.TrimSpace(config.Command.Command) == "" {
		return fmt.Errorf("health.command.command is required")
	}
	return nil
}

// HasConfiguredHealth reports whether the generic service declares a valid
// probe and is therefore eligible for a published health endpoint.
func HasConfiguredHealth(node yaml.Node) (bool, error) {
	config, err := decodeConfig(node)
	if err != nil {
		return false, err
	}
	return config.Health != nil, nil
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "generic-container-renderer",
		Mode:           servicetemplate.Interpolated,
		DecodeConfig:   decodeConfig,
		RenderInstance: renderService,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement { return nil }

func (serviceType) Health() typeregistry.HealthBehavior {
	return typeregistry.HealthBehavior{Mode: typeregistry.HealthModeOptional, ContainerPort: 9878}
}

func (serviceType) Description() string {
	return "Catch-all generic container workload"
}

func (serviceType) DefaultImage() string { return "" }

func renderService(_ context.Context, input render.Input, cfg Config) (render.Result, error) {
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("generic-container %q has no instances", input.Service.Name)
	}

	// Determine which DNS servers to inject for LAN-connected services.
	// Podman's aardvark-dns always writes the network bridge IP (.254) into
	// the container's /etc/resolv.conf. For LAN networks, the gateway's
	// brlan0 dnsmasq (at .1) is the correct resolver — it forwards to BNG
	// which knows about all container hostnames. We collect unique PodmanDNS
	// values from every network role this service connects to.
	roleDNS := map[string]string{}
	for _, n := range input.Deployment.Networks {
		if n.PodmanDNS != "" {
			roleDNS[n.Role] = n.PodmanDNS
		}
	}
	dnsSet := map[string]struct{}{}
	var lanDNS []string
	for _, iface := range input.Service.Instances[0].Interfaces {
		if ip, ok := roleDNS[iface.Role]; ok {
			if _, seen := dnsSet[ip]; !seen {
				dnsSet[ip] = struct{}{}
				lanDNS = append(lanDNS, ip)
			}
		}
	}

	env := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])
	env = append(env, render.SortedEnv(cfg.Env)...)

	composeYAML, err := generateCompose(input, cfg)
	if err != nil {
		return render.Result{}, err
	}

	artifacts := []render.Artifact{
		{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		{Key: "compose.yaml", Content: composeYAML},
		{Key: "entrypoint.sh", Content: entrypointSH},
	}
	if cfg.Health != nil {
		artifacts = append(artifacts, render.Artifact{Key: "vcpe-healthd.required", Content: ""})
	}

	return render.Result{
		Renderer:  "generic-container-renderer",
		Artifacts: artifacts,
	}, nil
}

func generateCompose(input render.Input, cfg Config) (string, error) {
	inst := input.Service.Instances[0]
	replicas := input.Service.Replicas
	if replicas <= 0 {
		// Fall back to the number of resolved instances when Replicas is not
		// explicitly set (e.g., when the service is constructed without going
		// through the planner).
		replicas = len(input.Service.Instances)
		if replicas == 0 {
			replicas = 1
		}
	}

	// Build the top-level external network declarations from the first
	// instance's interface list (network names are the same across replicas).
	topNetworks := map[string]any{}
	for _, iface := range inst.Interfaces {
		key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
		topNetworks[iface.Role] = map[string]any{
			"external": true,
			"name":     "${IFACE_" + key + "_NETWORK}",
		}
	}

	// buildSvcEntry constructs a single compose service map.
	// pinMAC controls whether mac_address is included in the network
	// attachment; single-replica services pin the IPAM MAC, multi-replica
	// services let Podman assign a unique random MAC to each container.
	buildSvcEntry := func(index int, pinMAC bool) map[string]any {
		svcNetworks := map[string]any{}
		for _, iface := range inst.Interfaces {
			key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
			netEntry := map[string]any{}
			if pinMAC {
				netEntry["mac_address"] = "${IFACE_" + key + "_MAC}"
			}
			svcNetworks[iface.Role] = netEntry
		}
		svc := map[string]any{
			"image":    render.ImageRef(input.Service.Image),
			"env_file": []string{"compose.env"},
			"restart":  "unless-stopped",
			"cap_add":  []string{"NET_ADMIN", "NET_RAW"},
		}
		if len(svcNetworks) > 0 {
			svc["networks"] = svcNetworks
		}
		volumes := append(append([]string(nil), input.Service.Volumes...), cfg.Volumes...)
		volumes = append(volumes, "./entrypoint.sh:/run/vcpe/entrypoint.sh:ro")
		svc["volumes"] = volumes
		svc["entrypoint"] = []string{"/bin/sh", "/run/vcpe/entrypoint.sh"}
		if len(cfg.Command) > 0 {
			svc["command"] = cfg.Command
		}
		// Merge top-level manifest ports with any ports declared in config.
		allPorts := append(append([]string(nil), input.Service.Ports...), cfg.Ports...)
		if cfg.Health != nil && input.HealthPorts[index] != 0 {
			allPorts = append(allPorts, fmt.Sprintf("127.0.0.1:%d:9878", input.HealthPorts[index]))
		}
		if len(allPorts) > 0 {
			svc["ports"] = allPorts
		}
		if len(cfg.Env) > 0 {
			envMap := map[string]string{}
			for k, v := range cfg.Env {
				envMap[k] = v
			}
			svc["environment"] = envMap
		}
		return svc
	}

	services := map[string]any{}
	// Always use 1-based indexed names ({service}-{n}) regardless of replica
	// count so that names are stable when replicas changes. This enables
	// scale-up and scale-down without orphaning existing containers.
	for i := 0; i < replicas; i++ {
		// Pin the MAC address only for single-replica services, where the IPAM
		// MAC is stable. Multi-replica services let Podman assign unique MACs.
		pinMAC := replicas == 1
		entry := buildSvcEntry(i, pinMAC)
		// Set an explicit container_name and hostname, always indexed (e.g.
		// example-client-1) so names are stable and unambiguous regardless of
		// replica count.
		entry["container_name"] = fmt.Sprintf("${DEPLOYMENT_NAME}-${SERVICE_NAME}-%d", i+1)
		entry["hostname"] = fmt.Sprintf("${SERVICE_NAME}-%d", i+1)
		services[fmt.Sprintf("%s-%d", input.Service.Name, i+1)] = entry
		if cfg.Health != nil {
			healthService := map[string]any{
				"image":        render.ImageRef(input.Service.Image),
				"network_mode": fmt.Sprintf("service:%s-%d", input.Service.Name, i+1),
				"depends_on":   []string{fmt.Sprintf("%s-%d", input.Service.Name, i+1)},
				"restart":      "unless-stopped",
				"volumes":      []string{"./vcpe-healthd:/run/vcpe/vcpe-healthd:ro"},
				"entrypoint":   []string{"/run/vcpe/vcpe-healthd"},
			}
			healthService["command"] = genericHealthCommand(cfg.Health)
			services[fmt.Sprintf("%s-health-%d", input.Service.Name, i+1)] = healthService
		}
	}

	doc := map[string]any{"services": services}
	if len(topNetworks) > 0 {
		doc["networks"] = topNetworks
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal generated compose: %w", err)
	}
	return string(out), nil
}

func genericHealthCommand(config *HealthConfig) []string {
	if config.Command != nil {
		return []string{"--timeout", fmt.Sprintf("%ds", config.TimeoutSeconds), "--probe", "configured=" + config.Command.Command}
	}
	expected := config.HTTP.ExpectedStatus
	if expected == 0 {
		expected = 200
	}
	return []string{"--timeout", fmt.Sprintf("%ds", config.TimeoutSeconds), "--http-probe", fmt.Sprintf("configured=%s|%d", config.HTTP.URL, expected)}
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
