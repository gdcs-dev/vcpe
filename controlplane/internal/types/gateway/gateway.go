// Package gateway implements the gateway service type. It renders the interface
// environment plus a small set of gateway-specific variables derived from its typed
// config, consumed by the curated compose file at services/gateway/compose.yaml.
package gateway

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for gateway.
const TypeName = "gateway"

// Config is the typed configuration for an gateway service.
type Config struct {
	LAN     LANConfig     `yaml:"lan,omitempty"`
	Erouter ErouterConfig `yaml:"erouter,omitempty"`
}

type LANConfig struct {
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

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer { return renderer{} }

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{
		{Role: "lan", Required: false},
		{Role: "erouter", Required: false},
	}
}

func (serviceType) DefaultImagePolicy() string { return "build" }

type renderer struct{}

func (renderer) Name() string { return "gateway-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	var cfg Config
	if err := typeregistry.StrictDecode(input.Service.Config, &cfg); err != nil {
		return render.Result{}, fmt.Errorf("gateway %q: %w", input.Service.Name, err)
	}
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("gateway %q has no instances", input.Service.Name)
	}

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

	// LAN port MACs: roles matching lanPrefix+{1-4} → LAN{1-4}_MAC
	for i := 1; i <= 4; i++ {
		mac := ""
		if iface, ok := ifaceByRole[fmt.Sprintf("%s%d", lanPrefix, i)]; ok {
			mac = iface.MAC
		}
		env = append(env, fmt.Sprintf("LAN%d_MAC=%s", i, mac))
	}

	// cmRole → wan0 (physical cable-modem line interface)
	env = append(env, "WAN0_MAC="+ifaceByRole[cmRole].MAC)

	// wanRole → erouter0 (the erouter/WAN IP interface)
	wanIface := ifaceByRole[wanRole]
	env = append(env, "EROUTER0_MAC="+wanIface.MAC)

	// EROUTER0_IPV4 must be in CIDR notation for `ip addr add`
	wanCIDR := ""
	if n := input.Deployment.Network(wanRole); n != nil && n.IPv4 != nil {
		wanCIDR = n.IPv4.CIDR
	}
	env = append(env, "EROUTER0_IPV4="+ipWithPrefix(wanIface.IPv4, wanCIDR))
	env = append(env, "EROUTER0_IPV6="+wanIface.IPv6)
	env = append(env, "EROUTER0_IPV4_GATEWAY="+wanIface.Gateway4)
	env = append(env, "EROUTER0_IPV6_GATEWAY="+wanIface.Gateway6)

	if cfg.Erouter.VLAN != 0 {
		env = append(env, "EROUTER0_VLAN="+strconv.Itoa(cfg.Erouter.VLAN))
	}

	// BRLAN0 gets the LAN IP/prefix for `ip addr add` on the bridge
	env = append(env, "BRLAN0_IPV4="+cfg.LAN.IPv4)
	env = append(env, "BRLAN0_IPV6="+cfg.LAN.IPv6)
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

	composeYAML := renderGatewayCompose(input.Service.Name, inst.Interfaces, input.Service.Volumes, input.Service.Ports)

	return render.Result{
		Renderer: "gateway-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.yaml", Content: composeYAML},
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		},
	}, nil
}

// ipWithPrefix returns ip with the prefix length extracted from cidr
// (e.g. ip="10.1.2.3", cidr="10.1.2.0/24" → "10.1.2.3/24").
// Returns ip unchanged if either argument is empty or cidr is malformed.
func ipWithPrefix(ip, cidr string) string {
	if ip == "" || cidr == "" {
		return ip
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ip
	}
	ones, _ := ipNet.Mask.Size()
	return fmt.Sprintf("%s/%d", ip, ones)
}

// renderGatewayCompose generates a compose.yaml for the gateway service wired
// to the exact interfaces from the resolved instance. This replaces the curated
// services/gateway/compose.yaml when the gateway connects to non-standard roles
// (e.g. lan-7-p1 instead of lan-p1).
func renderGatewayCompose(svcName string, ifaces []plan.Interface, extraVolumes []string, ports []string) string {
	svcNets := map[string]any{}
	topNets := map[string]any{}
	for _, iface := range ifaces {
		key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
		svcNets[iface.Role] = map[string]any{
			"mac_address": "${IFACE_" + key + "_MAC}",
		}
		topNets[iface.Role] = map[string]any{
			"external": true,
			"name":     "${IFACE_" + key + "_NETWORK}",
		}
	}
	svc := map[string]any{
		"image":          "${IMAGE}",
		"container_name": "${DEPLOYMENT_NAME}-${SERVICE_NAME}",
		"hostname":       "${SERVICE_NAME}",
		"privileged":     true,
		"cap_add":        []string{"NET_ADMIN", "NET_RAW"},
		"env_file":       []string{"compose.env"},
		"networks":       svcNets,
	}
	if len(extraVolumes) > 0 {
		svc["volumes"] = extraVolumes
	}
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	doc := map[string]any{
		"services": map[string]any{svcName: svc},
		"networks": topNets,
	}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
