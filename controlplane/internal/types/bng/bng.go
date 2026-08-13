// Package bng implements the BNG (broadband network gateway) service type. It
// owns the typed DHCP/RADVD configuration and renders dhcpd.conf, dhcpd6.conf,
// and radvd.conf alongside the interface environment consumed by the curated
// compose file at services/bng/compose.yaml.
package bng

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for BNG.
const TypeName = "bng"

// Config is the typed configuration for a BNG service.
type Config struct {
	Access []AccessSegment   `yaml:"access"`
	Env    map[string]string `yaml:"env,omitempty"`
}

// AccessSegment configures DHCP/RADVD service on one access role. The role must
// correspond to an interface the BNG service attaches to.
type AccessSegment struct {
	Role  string `yaml:"role"`
	DHCP4 *DHCP4 `yaml:"dhcp4,omitempty"`
	DHCP6 *DHCP6 `yaml:"dhcp6,omitempty"`
	RADVD *RADVD `yaml:"radvd,omitempty"`
}

type DHCP4 struct {
	Subnet       string            `yaml:"subnet"`
	Ranges       []Range           `yaml:"ranges"`
	Options      map[string]string `yaml:"options,omitempty"`
	LeaseSeconds int               `yaml:"leaseSeconds,omitempty"`
}

type DHCP6 struct {
	Subnet       string            `yaml:"subnet"`
	Ranges       []Range           `yaml:"ranges"`
	Options      map[string]string `yaml:"options,omitempty"`
	LeaseSeconds int               `yaml:"leaseSeconds,omitempty"`
}

type Range struct {
	Start string `yaml:"start"`
	End   string `yaml:"end"`
}

type RADVD struct {
	Prefix         string   `yaml:"prefix"`
	AdvManagedFlag bool     `yaml:"advManagedFlag,omitempty"`
	AdvOtherConfig bool     `yaml:"advOtherConfig,omitempty"`
	RDNSS          []string `yaml:"rdnss,omitempty"`
}

type serviceType struct{ typeregistry.BaseServiceType }

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	var cfg Config
	if err := typeregistry.StrictDecode(node, &cfg); err != nil {
		return err
	}
	for _, seg := range cfg.Access {
		if seg.Role == "" {
			return fmt.Errorf("bng access segment is missing role")
		}
	}
	return nil
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "bng-renderer",
		Mode:           servicetemplate.PerInstance,
		DecodeConfig:   decodeConfig,
		RenderInstance: renderBNGInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{
		{Role: "wan", Required: false},
		{Role: "cm", Required: false},
		{Role: "mgmt", Required: false},
	}
}

func (serviceType) Description() string {
	return "Broadband Network Gateway — DHCP4/RADVD/DNS on WAN and CM segments"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/bng" }

func decodeConfig(node yaml.Node) (Config, error) {
	var cfg Config
	if err := typeregistry.StrictDecode(node, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func renderBNGInstance(_ context.Context, input render.Input, cfg Config) (render.Result, error) {
	inst := input.Service.Instances[0]
	devByRole := map[string]string{}
	ipByRole := map[string]string{}
	for _, iface := range inst.Interfaces {
		devByRole[iface.Role] = iface.Device
		ipByRole[iface.Role] = iface.IPv4
	}

	dhcpd, dhcpd6, radvd, err := renderConfigs(input.Service.Name, cfg, devByRole, ipByRole)
	if err != nil {
		return render.Result{}, err
	}

	env := render.IfaceEnv(input.Deployment, input.Service, inst)
	// IFACE_*_MAC / IFACE_*_DEVICE are the canonical env var contract.
	// Legacy role-specific aliases (MGMT_MAC=, WAN_MAC=, CM_MAC=) are no
	// longer emitted; the entrypoint uses IFACE_*_MAC + IFACE_*_DEVICE.
	env = append(env, render.BridgeEnv(input.Service.Bridges)...)

	// For container-managed (ipamDriver: none) networks, the BNG must assign
	// its own IPs. Add <ROLE>_IPV4_CIDR env vars so network-startup.sh can
	// run `ip addr add` on those interfaces.
	for _, iface := range inst.Interfaces {
		if iface.Role == "mgmt" || iface.IPv4 == "" {
			continue
		}
		n := input.Deployment.Network(iface.Role)
		if n == nil || n.IPv4 == nil || n.IPv4.CIDR == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(n.IPv4.CIDR)
		if err != nil {
			continue
		}
		ones, _ := ipNet.Mask.Size()
		roleKey := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
		env = append(env, fmt.Sprintf("%s_IPV4_CIDR=%s/%d", roleKey, iface.IPv4, ones))
	}

	composeYAML := renderBNGCompose(input, inst)

	env = append(env, render.SortedEnv(cfg.Env)...)

	return render.Result{
		Renderer: "bng-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.yaml", Content: composeYAML},
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
			{Key: "etc/dhcp/dhcpd.conf", Content: dhcpd},
			{Key: "etc/dhcp/dhcpd6.conf", Content: dhcpd6},
			{Key: "etc/radvd.conf", Content: radvd},
			{Key: "etc/service-interfaces.env", Content: renderServiceInterfaces(cfg, devByRole)},
			{Key: "etc/sysctl.conf", Content: renderSysctl(cfg, devByRole)},
			{Key: "etc/iptables.rules.v4", Content: renderIPTablesV4(input.Deployment, devByRole)},
			{Key: "etc/iptables.rules.v6", Content: ""},
			{Key: "etc/dnsmasq.conf", Content: renderDnsmasqConf(ipByRole, input.Deployment)},
			{Key: "etc/dnsmasq.hosts", Content: renderDnsmasqHosts(input)},
			{Key: "etc/dnsmasq.dynamic.hosts", Content: ""},
			{Key: "etc/dnsmasq.dhcp-hosts.map", Content: ""},
			{Key: "etc/dnsmasq.dhcp-subnets.map", Content: renderDnsmasqSubnetsMap(input.Deployment)},
			{Key: "etc/ntp.conf", Content: bngNTPConf},
			{Key: "etc/ports.conf", Content: renderPortsConf(ipByRole)},
			{Key: "etc/network-startup.sh", Content: bngNetworkStartup},
			{Key: "var/www/html/DCMresponse.txt", Content: bngDCMResponse},
		},
	}, nil
}

func renderConfigs(service string, cfg Config, devByRole map[string]string, ipByRole map[string]string) (string, string, string, error) {
	var v4, v6, ra strings.Builder
	v4.WriteString("# generated by vcpe bng-renderer\n")
	v6.WriteString("# generated by vcpe bng-renderer\n")
	ra.WriteString("# generated by vcpe bng-renderer\n")

	for _, seg := range cfg.Access {
		if seg.DHCP4 != nil {
			d4 := *seg.DHCP4
			// Auto-inject BNG's own IP as the DNS server so downstream
			// containers (GATEWAY, etc.) resolve hostnames via BNG dnsmasq.
			if bngIP := ipByRole[seg.Role]; bngIP != "" {
				if _, exists := d4.Options["domain-name-servers"]; !exists {
					if d4.Options == nil {
						d4.Options = map[string]string{}
					}
					d4.Options["domain-name-servers"] = bngIP
				}
			}
			block, err := renderDHCP4(&d4)
			if err != nil {
				return "", "", "", fmt.Errorf("bng %q role %q dhcp4: %w", service, seg.Role, err)
			}
			v4.WriteString(block)
		}
		if seg.DHCP6 != nil {
			block, err := renderDHCP6(seg.DHCP6)
			if err != nil {
				return "", "", "", fmt.Errorf("bng %q role %q dhcp6: %w", service, seg.Role, err)
			}
			v6.WriteString(block)
		}
		if seg.RADVD != nil {
			dev := devByRole[seg.Role]
			if dev == "" {
				return "", "", "", fmt.Errorf("bng %q role %q radvd: no interface device resolved for role", service, seg.Role)
			}
			ra.WriteString(renderRADVD(dev, seg.RADVD))
		}
	}
	return v4.String(), v6.String(), ra.String(), nil
}

func renderDHCP4(d *DHCP4) (string, error) {
	subnet, mask, err := subnetAndMask(d.Subnet)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "subnet %s netmask %s {\n", subnet, mask)
	for _, r := range d.Ranges {
		fmt.Fprintf(&b, "  range %s %s;\n", r.Start, r.End)
	}
	for _, k := range sortedKeys(d.Options) {
		fmt.Fprintf(&b, "  option %s %s;\n", k, d.Options[k])
	}
	if d.LeaseSeconds > 0 {
		fmt.Fprintf(&b, "  default-lease-time %d;\n", d.LeaseSeconds)
		fmt.Fprintf(&b, "  max-lease-time %d;\n", d.LeaseSeconds)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func renderDHCP6(d *DHCP6) (string, error) {
	if _, _, err := net.ParseCIDR(d.Subnet); err != nil {
		return "", fmt.Errorf("invalid subnet %q: %w", d.Subnet, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "subnet6 %s {\n", d.Subnet)
	for _, r := range d.Ranges {
		fmt.Fprintf(&b, "  range6 %s %s;\n", r.Start, r.End)
	}
	for _, k := range sortedKeys(d.Options) {
		fmt.Fprintf(&b, "  option %s %s;\n", k, d.Options[k])
	}
	if d.LeaseSeconds > 0 {
		fmt.Fprintf(&b, "  default-lease-time %d;\n", d.LeaseSeconds)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func renderRADVD(device string, r *RADVD) string {
	var b strings.Builder
	fmt.Fprintf(&b, "interface %s {\n", device)
	b.WriteString("  AdvSendAdvert on;\n")
	fmt.Fprintf(&b, "  AdvManagedFlag %s;\n", onOff(r.AdvManagedFlag))
	fmt.Fprintf(&b, "  AdvOtherConfigFlag %s;\n", onOff(r.AdvOtherConfig))
	if r.Prefix != "" {
		fmt.Fprintf(&b, "  prefix %s {\n", r.Prefix)
		b.WriteString("    AdvOnLink on;\n")
		b.WriteString("    AdvAutonomous on;\n")
		b.WriteString("  };\n")
	}
	if len(r.RDNSS) > 0 {
		fmt.Fprintf(&b, "  RDNSS %s {\n  };\n", strings.Join(r.RDNSS, " "))
	}
	b.WriteString("};\n")
	return b.String()
}

// renderServiceInterfaces emits the DHCP4_INTERFACES / DHCP6_INTERFACES env
// file that start-services.sh sources to know which devices to bind dhcpd on.
func renderServiceInterfaces(cfg Config, devByRole map[string]string) string {
	var devs []string
	for _, seg := range cfg.Access {
		if dev := devByRole[seg.Role]; dev != "" {
			devs = append(devs, dev)
		}
	}
	ifaces := strings.Join(devs, " ")
	return fmt.Sprintf("DHCP4_INTERFACES=%q\nDHCP6_INTERFACES=%q\n", ifaces, ifaces)
}

// renderSysctl produces the sysctl.conf that enables IP forwarding and RA
// acceptance on access interfaces.
func renderSysctl(cfg Config, devByRole map[string]string) string {
	lines := []string{
		"# generated by vcpe bng-renderer",
		"net.ipv4.ip_forward=1",
		"net.ipv6.conf.all.forwarding=1",
	}
	for _, seg := range cfg.Access {
		if dev := devByRole[seg.Role]; dev != "" {
			lines = append(lines, fmt.Sprintf("net.ipv6.conf.%s.accept_ra=2", dev))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// renderIPTablesV4 produces a minimal iptables ruleset. NAT networks get a
// MASQUERADE rule for traffic leaving via the mgmt interface.
func renderIPTablesV4(dep plan.Deployment, devByRole map[string]string) string {
	var b strings.Builder
	b.WriteString("*nat\n")
	b.WriteString(":PREROUTING ACCEPT [0:0]\n")
	b.WriteString(":INPUT ACCEPT [0:0]\n")
	b.WriteString(":OUTPUT ACCEPT [0:0]\n")
	b.WriteString(":POSTROUTING ACCEPT [0:0]\n")
	mgmtDev := devByRole["mgmt"]
	if mgmtDev == "" {
		mgmtDev = "eth0"
	}
	// Masquerade ALL traffic leaving via the mgmt interface. Using a single
	// broad rule (rather than per-subnet rules) ensures that traffic forwarded
	// from gateway LAN clients (192.168.10.x) is also masqueraded, so webpa
	// and other mgmt-side services see packets from the BNG's own mgmt IP and
	// can reply correctly without needing routes to the LAN subnets.
	fmt.Fprintf(&b, "-A POSTROUTING -o %s -j MASQUERADE\n", mgmtDev)
	b.WriteString("COMMIT\n")
	return b.String()
}

// webpaVirtualHosts lists the virtual service hostnames that all resolve to the
// WebPA server IP so CPE devices can reach each microservice by name.
var webpaVirtualHosts = []string{
	"webpa", "consul", "talaria", "scytale", "tr1d1um", "argus", "caduceus", "petasos", "themis",
}

// renderDnsmasqConf produces the main dnsmasq.conf. It binds dnsmasq to the
// loopback address plus all interface IPs that are currently resolved, and
// serves DHCP on the mgmt interface so containers receive IPs dynamically.
// dnsmasq integrates DHCP leases into DNS, making each container resolvable
// by hostname without a static hosts file.
func renderDnsmasqConf(ipByRole map[string]string, dep plan.Deployment) string {
	var b strings.Builder
	// Bind to loopback plus every interface IP BNG has, so dnsmasq answers
	// DNS/DHCP on all segments regardless of how roles are named in the manifest.
	listenIPs := []string{"127.0.0.1"}
	if ip := ipByRole["mgmt"]; ip != "" {
		listenIPs = append(listenIPs, ip) // mgmt always first
	}
	for _, role := range sortedKeys(ipByRole) {
		if role == "mgmt" {
			continue
		}
		if ip := ipByRole[role]; ip != "" {
			listenIPs = append(listenIPs, ip)
		}
	}
	fmt.Fprintf(&b, "port=53\nbind-dynamic\nlisten-address=%s\nno-hosts\n", strings.Join(listenIPs, ","))
	fmt.Fprintf(&b, "addn-hosts=/etc/dnsmasq.hosts\naddn-hosts=/etc/dnsmasq.dynamic.hosts\n")
	fmt.Fprintf(&b, "resolv-file=/etc/dnsmasq.upstream-resolv.conf\ncache-size=0\n")

	// Serve DHCP on the management segment. dnsmasq automatically maps DHCP
	// client hostnames to their assigned IPs, so no static hosts file is
	// needed for mgmt-side containers.
	if mgmt := dep.Network("mgmt"); mgmt != nil && mgmt.IPv4 != nil && mgmt.IPv4.Pool != nil {
		fmt.Fprintf(&b, "dhcp-range=%s,%s,12h\n", mgmt.IPv4.Pool.Start, mgmt.IPv4.Pool.End)
		fmt.Fprintf(&b, "dhcp-option=3,%s\n", mgmt.IPv4.Gateway)
		fmt.Fprintf(&b, "dhcp-authoritative\n")
	}

	// Emit CNAME aliases for WebPA virtual hostnames so they follow the
	// webpa container's live IP (resolved via aardvark-dns) without needing
	// a planned IP baked into the static hosts file. The target is the actual
	// compose/aardvark instance alias (e.g. "webpa-1"), not the bare manifest
	// service name, which curated renderers no longer register as a network
	// alias.
	for _, svc := range dep.Services {
		if svc.Type != "webpa" {
			continue
		}
		alias := firstInstanceAlias(svc)
		for _, hostname := range webpaVirtualHosts {
			if hostname == alias {
				continue // the instance alias itself resolves via aardvark
			}
			fmt.Fprintf(&b, "cname=%s,%s\n", hostname, alias)
		}
		// Clients on Podman-managed networks receive "search dns.podman" in
		// their resolv.conf, causing a bare hostname to resolve as
		// "<name>.dns.podman" first. Podman's aardvark-dns answers with the
		// IPAM address, but BNG DHCP replaces that address with a lease from
		// our pool, leaving the IPAM address unreachable. Override all
		// *.dns.podman names for every webpa virtual host so queries that
		// arrive here return the live DHCP-assigned IP instead.
		for _, hostname := range webpaVirtualHosts {
			podmanName := hostname + ".dns.podman"
			fmt.Fprintf(&b, "cname=%s,%s\n", podmanName, alias)
		}
		break
	}
	return b.String()
}

// firstInstanceAlias returns the compose service key / aardvark-dns network
// alias for a service's first instance, matching the "<name>-<index+1>"
// convention every curated and generic renderer uses.
func firstInstanceAlias(svc plan.Service) string {
	return fmt.Sprintf("%s-%d", svc.Name, svc.Instances[0].Index+1)
}

// renderDnsmasqHosts emits a hosts file entry for every deployment peer that
// has a mgmt-network interface, using the IPAM-assigned IP. The hostname is
// the peer's compose/aardvark instance alias (e.g. "webpa-1"): the entrypoint
// resolves each of these names via aardvark-dns at boot to seed the dynamic
// hosts file, and a bare service name is no longer a registered network
// alias, so using it here would leave the lookup empty. Virtual hostnames
// (consul, talaria, etc.) are handled as dnsmasq CNAMEs in dnsmasq.conf so
// they track the live IP without requiring a planned-IP bake-in.
func renderDnsmasqHosts(input render.Input) string {
	var b strings.Builder
	for _, svc := range input.Deployment.Services {
		if svc.Name == input.Service.Name {
			continue // BNG is the DNS server; skip self
		}
		if len(svc.Instances) == 0 {
			continue
		}
		mgmtIP := ""
		for _, iface := range svc.Instances[0].Interfaces {
			if iface.Role == "mgmt" {
				mgmtIP = iface.IPv4
				break
			}
		}
		if mgmtIP == "" {
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", mgmtIP, firstInstanceAlias(svc))
	}
	return b.String()
}

// renderDnsmasqSubnetsMap maps each access network CIDR to the expected CPE
// hostname for dnsmasq's subnet-host resolution feature.
func renderDnsmasqSubnetsMap(dep plan.Deployment) string {
	var b strings.Builder
	customer := dep.Labels["customer"]
	if customer == "" {
		customer = dep.Name
	}
	for _, n := range dep.Networks {
		if n.IPv4 == nil {
			continue
		}
		switch n.Role {
		case "wan":
			fmt.Fprintf(&b, "%s xb10-%s\n", n.IPv4.CIDR, customer)
		case "cm":
			fmt.Fprintf(&b, "%s xb10-cm-%s\n", n.IPv4.CIDR, customer)
		}
	}
	return b.String()
}

// renderBNGCompose generates a compose.yaml for the BNG service that wires up
// every interface from the resolved instance, regardless of role name. This
// replaces the curated services/bng/compose.yaml for deployments where the BNG
// connects to more than the standard mgmt/wan/cm trio.
func renderBNGCompose(input render.Input, inst plan.Instance) string {
	svc, topNets := servicetemplate.BuildComposeService(input, inst, servicetemplate.DefaultAttachment)
	svc["privileged"] = true
	svc["cap_add"] = []string{"NET_ADMIN", "NET_RAW"}
	svc["volumes"] = append([]string{fmt.Sprintf("./instances/%d:/runtime-config:ro", inst.Index+1)}, input.Service.Volumes...)
	ports := append([]string(nil), input.Service.Ports...)
	if healthPort := input.HealthPorts[inst.Index]; healthPort != 0 {
		ports = append(ports, fmt.Sprintf("127.0.0.1:%d:9878", healthPort))
	}
	if len(ports) > 0 {
		svc["ports"] = ports
	}
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, inst.Index+1)
	doc := map[string]any{
		"services": map[string]any{
			instanceName: svc,
		},
		"networks": topNets,
	}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// renderPortsConf produces an Apache ports.conf that listens on the WAN
// interface IP so CPE devices can reach the ACS/DCM HTTP server.
func renderPortsConf(ipByRole map[string]string) string {
	var b strings.Builder
	b.WriteString("# generated by vcpe bng-renderer\n")
	if ip := ipByRole["wan"]; ip != "" {
		fmt.Fprintf(&b, "Listen %s:80\n", ip)
	} else {
		b.WriteString("Listen 80\n")
	}
	b.WriteString("\n<IfModule ssl_module>\n\tListen 443\n</IfModule>\n")
	b.WriteString("<IfModule mod_gnutls.c>\n\tListen 443\n</IfModule>\n")
	return b.String()
}

// bngNTPConf is a static NTP client configuration using public pool servers.
const bngNTPConf = `# generated by vcpe bng-renderer
server 0.pool.ntp.org iburst
server 1.pool.ntp.org iburst
server 2.pool.ntp.org iburst
driftfile /var/lib/ntp/ntp.drift
`

// bngNetworkStartup brings up all interfaces after the entrypoint's rename
// pass, handles manifest-declared bridges, and assigns IPs on ipamDriver:none
// networks. All names are driven by IFACE_*_DEVICE env vars — no eth* globbing.
const bngNetworkStartup = `#!/bin/bash
set -euo pipefail
# ── 1. Create and IP-configure manifest-declared bridges ───────────────────
# BRIDGE_*_NAME = actual bridge name (avoids hyphen/underscore ambiguity)
# BRIDGE_*_IPV4 = CIDR to assign on the bridge
while IFS='=' read -r var bridge_name; do
    [[ "$var" == BRIDGE_*_NAME ]] || continue
    [[ -n "$bridge_name" ]] || continue
    ip link add "$bridge_name" type bridge 2>/dev/null || true
    ip link set "$bridge_name" up || true
done < <(env)
while IFS='=' read -r var cidr; do
    [[ "$var" == BRIDGE_*_IPV4 ]] || continue
    [[ -n "$cidr" ]] || continue
    key="${var%_IPV4}"
    name_var="${key}_NAME"
    bridge_name="${!name_var:-}"
    [[ -n "$bridge_name" ]] || continue
    ip addr show "$bridge_name" | grep -qF "${cidr%%/*}" || ip addr add "$cidr" dev "$bridge_name" || true
done < <(env)
# ── 2. Enslave interfaces to their declared bridge ─────────────────────────
while IFS='=' read -r var bridge_name; do
    [[ "$var" == IFACE_*_BRIDGE ]] || continue
    [[ -n "$bridge_name" ]] || continue
    role_key="${var%_BRIDGE}"; role_key="${role_key#IFACE_}"
    dev_var="IFACE_${role_key}_DEVICE"
    dev="${!dev_var:-}"
    [[ -n "$dev" ]] || continue
    ip link set "$dev" master "$bridge_name" || true
done < <(env)
# ── 3. Bring up all declared interfaces ────────────────────────────────────
while IFS='=' read -r var dev; do
    [[ "$var" == IFACE_*_DEVICE ]] || continue
    [[ -n "$dev" ]] || continue
    ip link set "$dev" up || true
done < <(env)
# ── 4. Assign IPs on container-managed (ipamDriver:none) networks ──────────
# <ROLE>_IPV4_CIDR is set by the bng-renderer for non-Podman-IPAM interfaces.
for var in $(compgen -v | grep '_IPV4_CIDR$' 2>/dev/null || env | grep -o '^[A-Z_]*_IPV4_CIDR' || true); do
    cidr="${!var:-}"
    [[ -n "$cidr" ]] || continue
    role="${var%_IPV4_CIDR}"
    dev_var="IFACE_${role}_DEVICE"
    dev="${!dev_var:-}"
    [[ -n "$dev" ]] || continue
    ip addr show "$dev" | grep -qF "${cidr%%/*}" || ip addr add "$cidr" dev "$dev" || true
done
# ── 5. Restore default route removed by the interface rename ───────────────
if ! ip route show default | grep -q .; then
    mgmt_gw="${IFACE_MGMT_GATEWAY4:-}"
    mgmt_dev="${IFACE_MGMT_DEVICE:-}"
    if [[ -n "$mgmt_gw" && -n "$mgmt_dev" ]]; then
        ip route add default via "$mgmt_gw" dev "$mgmt_dev" || true
    fi
fi
`

// bngDCMResponse is the static DCM response payload served by BNG over HTTP
// so CPE devices can retrieve their telemetry configuration.
const bngDCMResponse = `{
    "urn:settings:DCMSettings:DownloadConfig:StartTime": "01:00",
    "urn:settings:DCMSettings:DownloadConfig:MaxRandomDelay": "180",
    "urn:settings:TelemetryProfile": {
        "id": "d45419bb-6b21-4d54-a8a5-f29fda4e8229",
        "telemetryProfile": [{
                "header": "LoadAvg_split",
                "content": "LOAD_AVERAGE:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "MPSTAT_USR_split",
                "content": "MPSTAT_USR:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "MPSTAT_SYS_split",
                "content": "MPSTAT_SYS:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "MPSTAT_NICE_split",
                "content": "MPSTAT_NICE:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "MPSTAT_IRQ_split",
                "content": "MPSTAT_IRQ:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "MPSTAT_IDLE_split",
                "content": "MPSTAT_IDLE:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "TMPFS_USE_PERCENTAGE_split",
                "content": "TMPFS_USE_PERCENTAGE:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "RDKLOGS_USE_PERCENTAGE_split",
                "content": "RDKLOGS_USE_PERCENTAGE:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "NVRAM_USE_PERCENTAGE_split",
                "content": "NVRAM_USE_PERCENTAGE:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "SWAP_MEMORY_split",
                "content": "SWAP_MEMORY:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "CACHE_MEMORY_split",
                "content": "CACHE_MEMORY:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "BUFFER_MEMORY_split",
                "content": "BUFFER_MEMORY:",
                "type": "SelfHeal.txt.0",
                "pollingFrequency": "0"
            },
            {
                "header": "Total_Ethernet_Clients_split",
                "content": "ccsp-lm-lite",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "Total_online_clients_split",
                "content": "ccsp-lm-lite",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "Total_devices_connected_split",
                "content": "ccsp-lm-lite",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "SYS_SH_Zebra_restart",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "SYS_SH_Dibbler_restart",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "SYS_SH_PAM_Restart",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "SYS_INFO_NoIPv6_Address",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "RF_ERROR_IPV6PingFailed",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "RF_ERROR_IPV4PingFailed",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            },
            {
                "header": "SYS_ERROR_PSMCrash_reboot",
                "content": "telemetry_client",
                "type": "<event>",
                "pollingFrequency": "0"
            }
        ],
        "schedule": "*/15 * * * *",
        "telemetryProfile:name": "RDKB-MNG",
        "uploadRepository:URL": "http://192.168.2.120/",
        "uploadRepository:uploadProtocol": "HTTP"
    }
}
`

func subnetAndMask(cidr string) (string, string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("invalid subnet %q: %w", cidr, err)
	}
	m := ipnet.Mask
	if len(m) != net.IPv4len {
		return "", "", fmt.Errorf("subnet %q is not IPv4", cidr)
	}
	return ipnet.IP.String(), fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3]), nil
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Register wires this service type into the global registry. It is idempotent
// so tests and the aggregate registrar can call it freely.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
