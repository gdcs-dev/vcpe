// Package xb10 implements the xb10 service type for the RDK-B gateway
// simulator. It mirrors the gateway renderer pattern: it emits a compose.yaml
// that wires the container interfaces by MAC address, and a compose.env that
// maps the IFACE_* contract variables to the legacy names expected by the xb10
// entrypoint script (LAN1_MAC … LAN4_MAC, WAN0_MAC, EROUTER0_MAC, etc.).
package xb10

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

// TypeName is the manifest discriminator for xb10.
const TypeName = "xb10"

// Config is the typed configuration for an xb10 service.
type Config struct {
	Erouter ErouterConfig     `yaml:"erouter,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
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

type serviceType struct {
	typeregistry.BaseServiceType
}

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil // config is optional; defaults cover the common topology
	}
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "xb10-renderer",
		Mode:           servicetemplate.PerInstance,
		DecodeConfig:   decodeConfig,
		RenderInstance: renderXB10Instance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{
		{Role: "wan", Required: false},
		{Role: "cm", Required: false},
	}
}

func (serviceType) Description() string {
	return "XB10 CPE gateway simulator"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/xb10" }

func decodeConfig(node yaml.Node) (Config, error) {
	var config Config
	if node.Kind != 0 {
		if err := typeregistry.StrictDecode(node, &config); err != nil {
			return Config{}, err
		}
	}
	return config, nil
}

func renderXB10Instance(_ context.Context, input render.Input, cfg Config) (render.Result, error) {
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
	env := render.IfaceEnv(input.Deployment, input.Service, inst)
	ifaceByRole := make(map[string]plan.Interface, len(inst.Interfaces))
	for _, iface := range inst.Interfaces {
		ifaceByRole[iface.Role] = iface
	}
	wanIface := ifaceByRole[wanRole]
	wanCIDR := ""
	if network := input.Deployment.Network(wanRole); network != nil && network.IPv4 != nil {
		wanCIDR = network.IPv4.CIDR
	}
	env = append(env, "EROUTER0_IPV4="+render.IPWithPrefix(wanIface.IPv4, wanCIDR), "EROUTER0_IPV6="+wanIface.IPv6, "EROUTER0_IPV4_GATEWAY="+wanIface.Gateway4, "EROUTER0_IPV6_GATEWAY="+wanIface.Gateway6)
	if cfg.Erouter.VLAN != 0 {
		env = append(env, fmt.Sprintf("EROUTER0_VLAN=%d", cfg.Erouter.VLAN))
	}
	env = append(env, render.SortedEnv(cfg.Env)...)
	cmIface := ifaceByRole[cmRole]
	env = append(env, "WAN0_IPV4="+cmIface.IPv4, "WAN0_IPV6="+cmIface.IPv6)

	return render.Result{
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
			{Key: "compose.yaml", Content: renderXB10Compose(input)},
		},
	}, nil
}

// renderXB10Compose generates a compose.yaml for the xb10 container wired to
// the exact interfaces from the resolved instance, pinning each network
// attachment by MAC address.
func renderXB10Compose(input render.Input) string {
	topNets := map[string]any{}
	services := map[string]any{}
	for _, inst := range input.Service.Instances {
		svcNets := map[string]any{}
		for _, iface := range inst.Interfaces {
			svcNets[iface.Role] = map[string]any{"mac_address": iface.MAC}
			topNets[iface.Role] = map[string]any{"external": true, "name": iface.Network}
		}
		instanceName := fmt.Sprintf("%s-%d", input.Service.Name, inst.Index+1)
		svc := map[string]any{"image": render.ImageRef(input.Service.Image), "container_name": input.Deployment.Name + "-" + instanceName, "hostname": instanceName, "privileged": true, "cap_add": []string{"NET_ADMIN", "NET_RAW"}, "env_file": []string{fmt.Sprintf("instances/%d/compose.env", inst.Index+1)}, "networks": svcNets}
		if len(input.Service.Volumes) > 0 {
			svc["volumes"] = input.Service.Volumes
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
	doc := map[string]any{
		"services": services,
		"networks": topNets,
	}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
