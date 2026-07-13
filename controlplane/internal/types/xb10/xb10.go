// Package xb10 implements the xb10 service type for the RDK-B gateway
// simulator. It mirrors the gateway renderer pattern: it emits a compose.yaml
// that wires the container interfaces by MAC address, and a compose.env that
// maps the IFACE_* contract variables to the legacy names expected by the xb10
// entrypoint script (LAN1_MAC … LAN4_MAC, WAN0_MAC, EROUTER0_MAC, etc.).
package xb10

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for xb10.
const TypeName = "xb10"

// Config is the typed configuration for an xb10 service.
type Config struct {
	Erouter ErouterConfig `yaml:"erouter,omitempty"`
}

// ErouterConfig controls the role-to-interface mapping. Defaults match the
// standard single-gateway topology: wan → erouter0, cm → wan0, lan-p{1-4}.
type ErouterConfig struct {
	// WanRole is the manifest network role that maps to erouter0 (WAN IP).
	// Defaults to "wan".
	WanRole string `yaml:"wanRole,omitempty"`
	// CMRole is the manifest network role that maps to wan0 (cable-modem line).
	// Defaults to "cm".
	CMRole string `yaml:"cmRole,omitempty"`
	// LanPrefix is the role name prefix for LAN ports (lan-p1 … lan-p4).
	// Defaults to "lan-p".
	LanPrefix string `yaml:"lanPrefix,omitempty"`
	// VLAN sets EROUTER0_VLAN for tagged WAN interfaces.
	VLAN int `yaml:"vlan,omitempty"`
}

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil // config is optional; defaults cover the common topology
	}
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer { return renderer{} }

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{
		{Role: "wan", Required: false},
		{Role: "cm", Required: false},
	}
}

func (serviceType) DefaultImagePolicy() string { return "build" }

type renderer struct{}

func (renderer) Name() string { return "xb10-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("xb10 %q has no instances", input.Service.Name)
	}

	var cfg Config
	if input.Service.Config.Kind != 0 {
		if err := typeregistry.StrictDecode(input.Service.Config, &cfg); err != nil {
			return render.Result{}, fmt.Errorf("xb10 %q: %w", input.Service.Name, err)
		}
	}

	// Resolve role names with defaults.
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

	inst := input.Service.Instances[0]

	// Standard IFACE_* env vars.
	env := render.IfaceEnv(input.Deployment, input.Service, inst)

	// Build a role → Interface lookup for legacy alias derivation.
	ifaceByRole := make(map[string]plan.Interface, len(inst.Interfaces))
	for _, iface := range inst.Interfaces {
		ifaceByRole[iface.Role] = iface
	}

	// LAN port MACs: roles matching lanPrefix+{1-4} → LAN{1-4}_MAC
	// These are consumed by rename_interfaces_by_mac() in entrypoint.sh.
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

	// EROUTER0_IPV4 in CIDR notation for ip addr add.
	wanCIDR := ""
	if n := input.Deployment.Network(wanRole); n != nil && n.IPv4 != nil {
		wanCIDR = n.IPv4.CIDR
	}
	env = append(env, "EROUTER0_IPV4="+ipWithPrefix(wanIface.IPv4, wanCIDR))
	env = append(env, "EROUTER0_IPV6="+wanIface.IPv6)
	env = append(env, "EROUTER0_IPV4_GATEWAY="+wanIface.Gateway4)
	env = append(env, "EROUTER0_IPV6_GATEWAY="+wanIface.Gateway6)

	if cfg.Erouter.VLAN != 0 {
		env = append(env, fmt.Sprintf("EROUTER0_VLAN=%d", cfg.Erouter.VLAN))
	}

	// CM interface network vars (used by some xb10 services internally).
	cmIface := ifaceByRole[cmRole]
	env = append(env, "WAN0_IPV4="+cmIface.IPv4)
	env = append(env, "WAN0_IPV6="+cmIface.IPv6)

	composeYAML := renderXB10Compose(input.Service.Name, inst.Interfaces, input.Service.Volumes, input.Service.Ports)

	return render.Result{
		Renderer: "xb10-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.yaml", Content: composeYAML},
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		},
	}, nil
}

// renderXB10Compose generates a compose.yaml for the xb10 container wired to
// the exact interfaces from the resolved instance, pinning each network
// attachment by MAC address.
func renderXB10Compose(svcName string, ifaces []plan.Interface, extraVolumes []string, ports []string) string {
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

// ipWithPrefix returns ip with the prefix length from cidr appended.
// e.g. ip="10.1.2.3", cidr="10.1.2.0/24" → "10.1.2.3/24".
// Returns ip unchanged when cidr is empty or unparseable.
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

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
