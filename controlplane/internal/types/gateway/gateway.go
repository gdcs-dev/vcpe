// Package gateway implements the gateway service type. It renders the interface
// environment plus a small set of gateway-specific variables derived from its typed
// config, consumed by the curated compose file at services/gateway/compose.yaml.
package gateway

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for gateway.
const TypeName = "gateway"

// Config is the typed configuration for an gateway service.
type Config struct {
	LAN     LANConfig         `yaml:"lan,omitempty"`
	Erouter ErouterConfig     `yaml:"erouter,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

type LANConfig struct {
	// Bridge is the name of the LAN bridge created inside the container.
	// Defaults to "brlan0" when empty.
	Bridge    string `yaml:"bridge,omitempty"`
	IPv4      string `yaml:"ipv4,omitempty"`
	IPv6      string `yaml:"ipv6,omitempty"`
	DHCPStart string `yaml:"dhcpStart,omitempty"`
	DHCPEnd   string `yaml:"dhcpEnd,omitempty"`
}

type ErouterConfig struct {
	// WanRole is the manifest network role that maps to the erouter0 (WAN)
	// interface. Defaults to "wan" when empty.
	WanRole string `yaml:"wanRole,omitempty"`
	// CMRole is the manifest network role that maps to the wan0 (CM)
	// interface. Defaults to "cm" when empty.
	CMRole string `yaml:"cmRole,omitempty"`
	// LanPrefix is the role name prefix for LAN ports (lan-p1 … lan-p4).
	// Defaults to "lan-p" when empty.
	LanPrefix string `yaml:"lanPrefix,omitempty"`
	VLAN      int    `yaml:"vlan,omitempty"`
}

type serviceType struct {
	typeregistry.BaseServiceType
}

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "gateway-renderer",
		Mode:           servicetemplate.PerInstance,
		DecodeConfig:   decodeConfig,
		RenderInstance: renderGatewayInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{
		{Role: "wan", Required: false},
		{Role: "cm", Required: false},
		{Role: "lan-p1", Required: false},
	}
}

func (serviceType) Description() string {
	return "Cable-modem / CPE simulator with LAN bridging"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/gateway" }

func decodeConfig(node yaml.Node) (Config, error) {
	var config Config
	if err := typeregistry.StrictDecode(node, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func renderGatewayInstance(_ context.Context, input render.Input, cfg Config) (render.Result, error) {
	env := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])
	if cfg.LAN.IPv4 != "" {
		env = append(env, "LAN_IPV4="+cfg.LAN.IPv4)
	}
	if cfg.LAN.IPv6 != "" {
		env = append(env, "LAN_IPV6="+cfg.LAN.IPv6)
	}
	if cfg.Erouter.VLAN != 0 {
		env = append(env, "EROUTER_VLAN="+strconv.Itoa(cfg.Erouter.VLAN))
	}

	// Resolve configurable role names with defaults.
	wanRole := cfg.Erouter.WanRole
	if wanRole == "" {
		wanRole = "wan"
	}
	cmRole := cfg.Erouter.CMRole
	if cmRole == "" {
		cmRole = "cm"
	}
	lanPrefix := cfg.Erouter.LanPrefix
	if lanPrefix == "" {
		lanPrefix = "lan-p"
	}

	// Legacy aliases expected by gateway-legacy-entrypoint.sh.
	// The entrypoint renames container interfaces by MAC to canonical names.
	inst := input.Service.Instances[0]
	ifaceByRole := make(map[string]plan.Interface, len(inst.Interfaces))
	for _, iface := range inst.Interfaces {
		ifaceByRole[iface.Role] = iface
	}

	// Manifest-driven bridge name and LAN device list.
	// LAN_BRIDGE: prefer the first bridge declared in the manifest's bridges
	// section (the one attached by LAN interfaces). Fall back to config.lan.bridge
	// then "brlan0" so the DHCP/dnsmasq config still has a bridge name to bind to.
	lanBridge := ""
	for _, b := range input.Service.Bridges {
		lanBridge = b.Name
		break
	}
	if lanBridge == "" {
		lanBridge = cfg.LAN.Bridge
	}
	if lanBridge == "" {
		lanBridge = "brlan0"
	}
	env = append(env, "LAN_BRIDGE="+lanBridge)

	// LAN_DEVICES: space-delimited device names for all interfaces whose role
	// matches lanPrefix, in port order. The entrypoint iterates this list to
	// determine which interfaces to bridge.
	var lanDevices []string
	for i := 1; i <= 4; i++ {
		role := fmt.Sprintf("%s%d", lanPrefix, i)
		if iface, ok := ifaceByRole[role]; ok && iface.Device != "" {
			lanDevices = append(lanDevices, iface.Device)
		}
	}
	env = append(env, "LAN_DEVICES="+strings.Join(lanDevices, " "))

	// Emit generic BRIDGE_* env vars from the manifest's bridges section.
	env = append(env, render.BridgeEnv(input.Service.Bridges)...)

	// WAN/erouter interface vars (manifest-driven via IFACE_* — no legacy aliases).
	wanIface := ifaceByRole[wanRole]
	wanCIDR := ""
	if n := input.Deployment.Network(wanRole); n != nil && n.IPv4 != nil {
		wanCIDR = n.IPv4.CIDR
	}
	env = append(env, "EROUTER0_IPV4="+render.IPWithPrefix(wanIface.IPv4, wanCIDR))
	env = append(env, "EROUTER0_IPV6="+wanIface.IPv6)
	env = append(env, "EROUTER0_IPV4_GATEWAY="+wanIface.Gateway4)
	env = append(env, "EROUTER0_IPV6_GATEWAY="+wanIface.Gateway6)

	if cfg.Erouter.VLAN != 0 {
		env = append(env, "EROUTER0_VLAN="+strconv.Itoa(cfg.Erouter.VLAN))
	}

	// LAN bridge DHCP config (gateway-type-specific; bridge IP comes from BRIDGE_*_IPV4).
	// BRLAN0_IPV4 is emitted for dnsmasq: prefer the first bridge spec's IPv4,
	// fall back to cfg.LAN.IPv4 (old manifests without a bridges: section).
	lanBridgeIPv4 := cfg.LAN.IPv4
	if len(input.Service.Bridges) > 0 && input.Service.Bridges[0].IPv4 != "" {
		lanBridgeIPv4 = input.Service.Bridges[0].IPv4
	}
	env = append(env, "BRLAN0_IPV4="+lanBridgeIPv4)
	env = append(env, "BRLAN0_DHCP_START="+cfg.LAN.DHCPStart)
	env = append(env, "BRLAN0_DHCP_END="+cfg.LAN.DHCPEnd)

	// BNG_DNS_SERVER: find the BNG peer's IP on this gateway's CM or WAN
	// network so the entrypoint can route DNS through BNG dnsmasq.
	bngDNS := ""
	for _, svc := range input.Deployment.Services {
		if svc.Type != "bng" || len(svc.Instances) == 0 {
			continue
		}
		for _, iface := range svc.Instances[0].Interfaces {
			if iface.Role == cmRole {
				bngDNS = iface.IPv4
				break
			}
		}
		if bngDNS == "" {
			for _, iface := range svc.Instances[0].Interfaces {
				if iface.Role == wanRole {
					bngDNS = iface.IPv4
					break
				}
			}
		}
		break
	}
	env = append(env, "BNG_DNS_SERVER="+bngDNS)

	env = append(env, render.SortedEnv(cfg.Env)...)

	composeYAML := renderGatewayCompose(input, inst)

	return render.Result{
		Renderer: "gateway-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.yaml", Content: composeYAML},
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		},
	}, nil
}

// renderGatewayCompose generates a compose.yaml for the gateway service wired
// to the exact interfaces from the resolved instance. This replaces the curated
// services/gateway/compose.yaml when the gateway connects to non-standard roles
// (e.g. lan-7-p1 instead of lan-p1).
func renderGatewayCompose(input render.Input, inst plan.Instance) string {
	svcNets := map[string]any{}
	topNets := map[string]any{}
	for _, iface := range inst.Interfaces {
		svcNets[iface.Role] = map[string]any{
			"mac_address": iface.MAC,
		}
		topNets[iface.Role] = map[string]any{
			"external": true,
			"name":     iface.Network,
		}
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
	if len(input.Service.Volumes) > 0 {
		svc["volumes"] = input.Service.Volumes
	}
	ports := append([]string(nil), input.Service.Ports...)
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	services := map[string]any{instanceName: svc}
	// The manifest opts a self-addressed gateway into a health transport
	// sidecar by marking one interface healthUpstream; which role it names
	// does not matter here since the workload and its sidecar reach each
	// other over the shared, Podman-managed health network by service name.
	if healthPort := input.HealthPorts[inst.Index]; healthPort != 0 && hasHealthUpstream(inst.Interfaces) {
		healthNetworkName := input.Deployment.Name + "-00-health"
		topNets["aa-health"] = map[string]any{"external": true, "name": healthNetworkName}
		svcNets["aa-health"] = map[string]any{}
		healthServiceName := instanceName + "-health"
		svc["depends_on"] = []string{healthServiceName}
		services[healthServiceName] = map[string]any{
			"image":          render.ImageRef(input.Service.Image),
			"container_name": input.Deployment.Name + "-" + instanceName + "-health",
			"entrypoint":     []string{"/usr/local/bin/vcpe-healthd"},
			// The upstream's own checks run sequentially and can each take up
			// to its own probe timeout, so the proxy waits longer than any
			// single check to avoid timing out on a slow-but-valid response.
			"command":  []string{"--proxy-url", "http://" + instanceName + ":9878/health", "--timeout", "10s"},
			"networks": map[string]any{"aa-health": map[string]any{}},
			"ports":    []string{fmt.Sprintf("127.0.0.1:%d:9878", healthPort)},
			"restart":  "unless-stopped",
		}
	}
	doc := map[string]any{
		"services": services,
		"networks": topNets,
	}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// hasHealthUpstream reports whether the manifest opted this instance into a
// health transport sidecar by marking one of its interfaces healthUpstream.
func hasHealthUpstream(interfaces []plan.Interface) bool {
	for _, iface := range interfaces {
		if iface.HealthUpstream {
			return true
		}
	}
	return false
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
