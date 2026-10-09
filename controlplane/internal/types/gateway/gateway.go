// Package gateway implements the gateway service type. It renders the interface
// environment plus a small set of gateway-specific variables derived from its typed
// config, consumed by the curated compose file at services/gateway/compose.yaml.
package gateway

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
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

type serviceType struct{ typeregistry.BaseServiceType }

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
	var cfg Config
	if err := typeregistry.StrictDecode(node, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ValidateMeshLAN(service manifest.Service) error {
	for _, radio := range service.Radios {
		if radio.Mode == manifest.RadioModeMesh && radio.Mesh != nil {
			cfg, err := decodeConfig(service.Config)
			if err != nil {
				return err
			}
			return validateMeshLANConfig(service.Bridges, radio.Mesh.Bridge, cfg)
		}
	}
	return nil
}

func validateMeshLANConfig(bridges []manifest.BridgeSpec, bridgeName string, cfg Config) error {
	for _, bridge := range bridges {
		if bridge.Name != bridgeName {
			continue
		}
		for _, field := range []struct{ name, legacy, declared string }{
			{"bridge", cfg.LAN.Bridge, bridge.Name},
			{"ipv4", cfg.LAN.IPv4, bridge.IPv4},
			{"ipv6", cfg.LAN.IPv6, bridge.IPv6},
			{"dhcpStart", cfg.LAN.DHCPStart, bridge.DHCPStart},
			{"dhcpEnd", cfg.LAN.DHCPEnd, bridge.DHCPEnd},
		} {
			if field.legacy != "" && field.legacy != field.declared {
				return fmt.Errorf("mesh bridge %q: config.lan.%s conflicts with bridges[]", bridgeName, field.name)
			}
		}
		return nil
	}
	return fmt.Errorf("mesh bridge %q is not declared", bridgeName)
}

func renderGatewayInstance(_ context.Context, input render.Input, cfg Config) (render.Result, error) {

	env := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])
	env = append(env, "VCPE_STARTUP_CONTRACT=/etc/vcpe/startup-contract.json")
	meshBridge := ""
	for _, radio := range input.Service.Instances[0].Radios {
		if radio.Mode == manifest.RadioModeMesh {
			meshBridge = radio.Mesh.Bridge
		}
	}
	if meshBridge != "" {
		if err := validateMeshLANConfig(input.Service.Bridges, meshBridge, cfg); err != nil {
			return render.Result{}, err
		}
	}
	if meshBridge == "" && cfg.LAN.IPv4 != "" {
		env = append(env, "LAN_IPV4="+cfg.LAN.IPv4)
	}
	if meshBridge == "" && cfg.LAN.IPv6 != "" {
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
	if meshBridge != "" {
		lanBridge = meshBridge
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
	for _, bridge := range input.Service.Bridges {
		key := strings.ToUpper(strings.ReplaceAll(bridge.Name, "-", "_"))
		env = append(env, "BRIDGE_"+key+"_MAC="+plan.CanonicalMAC(input.Deployment.Name, input.Service.Name, "bridge/"+bridge.Name, input.Service.Instances[0].Index))
	}

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
	if meshBridge == "" {
		lanBridgeIPv4 := cfg.LAN.IPv4
		if len(input.Service.Bridges) > 0 && input.Service.Bridges[0].IPv4 != "" {
			lanBridgeIPv4 = input.Service.Bridges[0].IPv4
		}
		env = append(env, "BRLAN0_IPV4="+lanBridgeIPv4)
		env = append(env, "BRLAN0_DHCP_START="+cfg.LAN.DHCPStart)
		env = append(env, "BRLAN0_DHCP_END="+cfg.LAN.DHCPEnd)
	}

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

	hostapdArtifacts, err := renderHostapdArtifacts(input.Deployment, inst)
	if err != nil {
		return render.Result{}, err
	}
	meshArtifacts := []render.Artifact{}
	for _, radio := range inst.Radios {
		if radio.Mode != manifest.RadioModeMesh || radio.Mesh == nil {
			continue
		}
		medium := input.Deployment.WirelessMedium(radio.Medium)
		if medium == nil {
			return render.Result{}, fmt.Errorf("mesh radio %q references unknown medium %q", radio.Name, radio.Medium)
		}
		frequency := 0
		switch medium.Band {
		case "2.4ghz":
			frequency = 2407 + 5*medium.Channel
			if medium.Channel == 14 {
				frequency = 2484
			}
		case "5ghz":
			frequency = 5000 + 5*medium.Channel
		case "6ghz":
			frequency = 5950 + 5*medium.Channel
		default:
			return render.Result{}, fmt.Errorf("mesh radio %q has unsupported band %q", radio.Name, medium.Band)
		}
		policy := fmt.Sprintf("device=%s\nid=%s\nbridge=%s\nfrequency=%d\ncredential=%s\n", radio.Device, radio.Mesh.ID,
			radio.Mesh.Bridge, frequency, secrets.MeshCredentialContainerPath(radio.Mesh.ID))
		meshArtifacts = append(meshArtifacts, render.Artifact{Key: "mesh/" + radio.Name + ".conf", Content: policy})
	}
	composeYAML := renderGatewayCompose(input, inst, len(hostapdArtifacts) > 0, len(meshArtifacts) > 0)

	artifacts := []render.Artifact{
		{Key: "compose.yaml", Content: composeYAML},
		{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
	}
	artifacts = append(artifacts, hostapdArtifacts...)
	artifacts = append(artifacts, meshArtifacts...)
	return render.Result{
		Renderer:  "gateway-renderer",
		Artifacts: artifacts,
	}, nil
}

func renderHostapdArtifacts(deployment plan.Deployment, instance plan.Instance) ([]render.Artifact, error) {
	radios := append([]plan.Radio(nil), instance.Radios...)
	sort.Slice(radios, func(i, j int) bool { return radios[i].Name < radios[j].Name })
	artifacts := make([]render.Artifact, 0, len(radios))
	for _, radio := range radios {
		if radio.Mode != "ap" {
			continue
		}
		if radio.Medium != "" {
			content, err := renderMediaHostapdPolicy(deployment, radio)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, render.Artifact{Key: "hostapd/" + radio.Name + ".conf", Content: content})
			continue
		}
		var medium *plan.WirelessNetwork
		for index := range deployment.WirelessNetworks {
			if deployment.WirelessNetworks[index].Name == radio.Network {
				medium = &deployment.WirelessNetworks[index]
				break
			}
		}
		if medium == nil {
			return nil, fmt.Errorf("gateway AP radio %q references unknown wireless network %q", radio.Name, radio.Network)
		}
		if strings.ContainsAny(medium.SSID, "\r\n") {
			return nil, fmt.Errorf("gateway AP radio %q SSID contains a line break", radio.Name)
		}
		content := fmt.Sprintf("interface=%s\ndriver=nl80211\nssid=%s\nhw_mode=g\nchannel=%d\nbridge=%s\nauth_algs=1\n",
			radio.Device, medium.SSID, medium.Channel, radio.Bridge)
		switch medium.Security {
		case manifest.WirelessOpen:
			content += "wpa=0\n"
		case manifest.WirelessWPA2Personal, manifest.WirelessWPA3Personal:
			content += fmt.Sprintf("# vcpe-security=%s\n# vcpe-passphrase-file=%s\n",
				medium.Security, secrets.WirelessCredentialContainerPath(medium.Name))
		default:
			return nil, fmt.Errorf("gateway AP radio %q requires unsupported security %q", radio.Name, medium.Security)
		}
		artifacts = append(artifacts, render.Artifact{Key: "hostapd/" + radio.Name + ".conf", Content: content})
	}
	return artifacts, nil
}

func renderMediaHostapdPolicy(deployment plan.Deployment, radio plan.Radio) (string, error) {
	medium := deployment.WirelessMedium(radio.Medium)
	if medium == nil {
		return "", fmt.Errorf("gateway AP radio %q references unknown medium %q", radio.Name, radio.Medium)
	}
	mode := "a"
	if medium.Band == "2.4ghz" {
		mode = "g"
	}
	var policy strings.Builder
	fmt.Fprintf(&policy, "interface=%s\ndriver=nl80211\nhw_mode=%s\nchannel=%d\n", radio.Device, mode, medium.Channel)
	if medium.Band == "6ghz" {
		policy.WriteString("country_code=US\ncountry3=0x49\nieee80211d=1\nop_class=131\nieee80211ax=1\nhe_oper_chwidth=0\nhe_6ghz_reg_pwr_type=0\n")
	}
	vaps := append([]plan.VAP(nil), radio.VAPs...)
	sort.Slice(vaps, func(left, right int) bool { return vaps[left].Slot < vaps[right].Slot })
	for _, vap := range vaps {
		profile := deployment.WirelessNetwork(vap.Network)
		if profile == nil {
			return "", fmt.Errorf("gateway AP radio %q slot %d references unknown profile %q", radio.Name, vap.Slot, vap.Network)
		}
		if strings.ContainsAny(profile.SSID, "\r\n") {
			return "", fmt.Errorf("gateway AP radio %q slot %d SSID contains a line break", radio.Name, vap.Slot)
		}
		if vap.Slot != 0 {
			fmt.Fprintf(&policy, "bss=%s\n", vap.Device)
		}
		fmt.Fprintf(&policy, "bssid=%s\nssid=%s\nbridge=%s\nauth_algs=1\n", vap.MAC, profile.SSID, vap.Bridge)
		switch profile.Security {
		case manifest.WirelessOpen:
			policy.WriteString("wpa=0\n")
		case manifest.WirelessWPA2Personal, manifest.WirelessWPA3Personal:
			fmt.Fprintf(&policy, "# vcpe-security=%s\n# vcpe-passphrase-file=%s\n",
				profile.Security, secrets.WirelessCredentialContainerPath(profile.Name))
			if medium.Band == "6ghz" {
				policy.WriteString("sae_pwe=1\n")
			}
		default:
			return "", fmt.Errorf("gateway AP radio %q slot %d requires unsupported security %q", radio.Name, vap.Slot, profile.Security)
		}
	}
	return policy.String(), nil
}

// renderGatewayCompose generates a compose.yaml for the gateway service wired
// to the exact interfaces from the resolved instance. This replaces the curated
// services/gateway/compose.yaml when the gateway connects to non-standard roles
// (e.g. lan-7-p1 instead of lan-p1).
func renderGatewayCompose(input render.Input, inst plan.Instance, hasHostapdConfig, hasMeshConfig bool) string {
	svc, topNets := servicetemplate.BuildComposeService(input, inst, servicetemplate.MACOnlyAttachment)
	svc["privileged"] = true
	svc["cap_add"] = []string{"NET_ADMIN", "NET_RAW"}
	volumes := append([]string(nil), input.Service.Volumes...)
	volumes = append(volumes, "../startup-contracts/"+input.Service.Name+".json:/etc/vcpe/startup-contract.json:ro")
	volumes = append(volumes, render.SecretFileMounts(input.WirelessCredentialFiles)...)
	if hasHostapdConfig {
		volumes = append(volumes, "./hostapd:/etc/vcpe/hostapd:ro")
	}
	if hasMeshConfig {
		volumes = append(volumes, "./mesh:/etc/vcpe/mesh:ro")
	}
	if len(volumes) > 0 {
		svc["volumes"] = volumes
	}
	ports := append([]string(nil), input.Service.Ports...)
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	instanceName := inst.ComposeServiceName(input.Service.Name)
	services := map[string]any{instanceName: svc}
	svcNets, _ := svc["networks"].(map[string]any)
	servicetemplate.AttachHealthPublication(input, inst, input.HealthPorts[inst.Index], 9878, topNets, svcNets, svc)
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
