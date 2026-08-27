// Package routerd implements the routerd service type: manifest-driven LAN
// bridge topology (via the standard interfaces[].bridge field, the same
// mechanism gateway/BNG use) plus per-interface addressing, compiled into
// routerd's own resource-kind ConfigDocument (Interface/Bridge/WanPolicy
// resources) and applied via `routerctl apply` at container start —
// replacing the legacy GATEWAY-derived render-config.sh/entrypoint.sh pipeline.
package routerd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render/servicetemplate"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for routerd.
const TypeName = "routerd"

// apiVersion is the routerd resource-kind API version namespace (matches the
// vendored routerd `ApiVersion::default()`).
const apiVersion = "net.routerd/v1"

// Config is the typed configuration for a routerd service. LAN bridge
// topology is not routerd-specific config: it reuses the manifest's standard
// services[].bridges[] + interfaces[].bridge fields (the same mechanism
// gateway/BNG use), so routerd currently declares no type-specific config.
type Config struct{}

type serviceType struct{ typeregistry.BaseServiceType }

var _ typeregistry.ServiceType = serviceType{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer {
	return servicetemplate.New(servicetemplate.Hooks[Config]{
		Name:           "routerd-renderer",
		Mode:           servicetemplate.PerInstance,
		DecodeConfig:   decodeConfig,
		RenderInstance: renderRouterdInstance,
	})
}

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "wan", Required: true}}
}

func (serviceType) Description() string {
	return "RDK-B-style router control plane — manifest-driven LAN bridge topology and WAN policy"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/routerd" }

func decodeConfig(node yaml.Node) (Config, error) {
	var cfg Config
	if err := typeregistry.StrictDecode(node, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// wireDocument mirrors the vendored routerd `resource_types::ConfigDocument`
// (`{"resources": [...]}`) accepted by `routerctl apply`.
type wireDocument struct {
	Resources []wireResource `json:"resources"`
}

// wireResource mirrors `resource_types::Resource<RawSpec>` — field names are
// the Rust struct's own (unrenamed) snake_case names.
type wireResource struct {
	APIVersion string   `json:"api_version"`
	Kind       string   `json:"kind"`
	Metadata   wireMeta `json:"metadata"`
	Spec       any      `json:"spec"`
}

// wireMeta mirrors `resource_types::ObjectMeta`.
type wireMeta struct {
	ID     string            `json:"id"`
	Labels map[string]string `json:"labels"`
}

// wireRef mirrors `resource_types::ResourceRef`.
type wireRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// wireInterfaceSpec mirrors `l2_types::InterfaceSpec`. admin_up/mtu are
// always present (as JSON null when unmanaged) since the Rust fields have no
// `#[serde(default)]`/skip_serializing_if annotation.
type wireInterfaceSpec struct {
	Name    string  `json:"name"`
	AdminUp *bool   `json:"admin_up"`
	MTU     *uint32 `json:"mtu"`
}

// wireBridgeSpec mirrors `l2_types::BridgeSpec`.
type wireBridgeSpec struct {
	Name    string    `json:"name"`
	Members []wireRef `json:"members"`
	AdminUp *bool     `json:"admin_up"`
	MTU     *uint32   `json:"mtu"`
}

// wireIpAddressSpec mirrors `l3_types::IpAddressSpec`. interface_ref may point
// at either an Interface or a Bridge resource — the real L3 controller
// resolves both kinds (see `l3-controller/src/resolve_ref.rs`).
type wireIpAddressSpec struct {
	InterfaceRef wireRef `json:"interface_ref"`
	Address      string  `json:"address"`
	PrefixLen    int     `json:"prefix_len"`
}

// wireWanPolicySpec mirrors `wan_types::WanPolicySpec`.
type wireWanPolicySpec struct {
	InterfaceRef wireRef       `json:"interface_ref"`
	Source       wireWanSource `json:"source"`
	Priority     uint32        `json:"priority"`
}

// wireWanSource mirrors `wan_types::WanSource`, an internally-tagged enum
// (`#[serde(tag = "kind", rename_all = "snake_case")]`): the Dhcp variant
// carries only the tag; the Static variant merges address/prefix_len/gateway
// into the same object.
type wireWanSource struct {
	Kind      string `json:"kind"`
	Address   string `json:"address,omitempty"`
	PrefixLen *int   `json:"prefix_len,omitempty"`
	Gateway   string `json:"gateway,omitempty"`
}

// boolPtr returns a pointer to b, for wire spec fields that must distinguish
// "explicitly set" from "unmanaged" (JSON null).
func boolPtr(b bool) *bool { return &b }

func renderRouterdInstance(_ context.Context, input render.Input, _ Config) (render.Result, error) {
	inst := input.Service.Instances[0]

	ifaceByRole := map[string]plan.Interface{}
	var roles []string
	for _, iface := range inst.Interfaces {
		ifaceByRole[iface.Role] = iface
		roles = append(roles, iface.Role)
	}
	sort.Strings(roles)

	// Group interfaces by their declared bridge — the same interfaces[].bridge
	// field gateway/BNG use — rather than a routerd-specific schema.
	bridgeMembers := map[string][]string{}
	for _, role := range roles {
		if b := ifaceByRole[role].Bridge; b != "" {
			bridgeMembers[b] = append(bridgeMembers[b], role)
		}
	}

	var resources []wireResource
	for _, role := range roles {
		iface := ifaceByRole[role]
		resources = append(resources, wireResource{
			APIVersion: apiVersion,
			Kind:       "Interface",
			Metadata:   wireMeta{ID: iface.Device, Labels: map[string]string{}},
			Spec:       wireInterfaceSpec{Name: iface.Device, AdminUp: boolPtr(true)},
		})
	}

	bridgeNames := make([]string, 0, len(bridgeMembers))
	for name := range bridgeMembers {
		bridgeNames = append(bridgeNames, name)
	}
	sort.Strings(bridgeNames)
	bridgeSpecByName := map[string]manifest.BridgeSpec{}
	for _, b := range input.Service.Bridges {
		bridgeSpecByName[b.Name] = b
	}
	for _, name := range bridgeNames {
		memberRoles := append([]string(nil), bridgeMembers[name]...)
		sort.Strings(memberRoles)
		members := make([]wireRef, 0, len(memberRoles))
		for _, role := range memberRoles {
			members = append(members, wireRef{Kind: "Interface", ID: ifaceByRole[role].Device})
		}
		resources = append(resources, wireResource{
			APIVersion: apiVersion,
			Kind:       "Bridge",
			Metadata:   wireMeta{ID: name, Labels: map[string]string{}},
			Spec:       wireBridgeSpec{Name: name, Members: members, AdminUp: boolPtr(true)},
		})

		if b, ok := bridgeSpecByName[name]; ok && b.IPv4 != "" {
			address, prefixLen, err := parseHostCIDR(b.IPv4)
			if err != nil {
				return render.Result{}, fmt.Errorf("routerd %q bridge %q ipv4: %w", input.Service.Name, name, err)
			}
			resources = append(resources, wireResource{
				APIVersion: apiVersion,
				Kind:       "IpAddress",
				Metadata:   wireMeta{ID: name + "-addr", Labels: map[string]string{}},
				Spec: wireIpAddressSpec{
					InterfaceRef: wireRef{Kind: "Bridge", ID: name},
					Address:      address,
					PrefixLen:    prefixLen,
				},
			})
		}
	}

	// Unbridged interfaces each get a WanPolicy; the one marked defaultRoute
	// (if any) is priority 1, the rest follow in role order — otherwise two
	// DHCP'd uplinks at the same priority produce a routerd route conflict
	// (equal-authority contenders excluded) instead of a deterministic winner.
	var unbridgedRoles []string
	for _, role := range roles {
		if ifaceByRole[role].Bridge == "" {
			unbridgedRoles = append(unbridgedRoles, role)
		}
	}
	sort.SliceStable(unbridgedRoles, func(i, j int) bool {
		di, dj := ifaceByRole[unbridgedRoles[i]].DefaultRoute, ifaceByRole[unbridgedRoles[j]].DefaultRoute
		if di != dj {
			return di
		}
		return unbridgedRoles[i] < unbridgedRoles[j]
	})

	for priority, role := range unbridgedRoles {
		iface := ifaceByRole[role]
		source, err := wanSourceFor(input.Deployment, iface)
		if err != nil {
			return render.Result{}, fmt.Errorf("routerd %q interface role %q: %w", input.Service.Name, role, err)
		}
		resources = append(resources, wireResource{
			APIVersion: apiVersion,
			Kind:       "WanPolicy",
			Metadata:   wireMeta{ID: iface.Device + "-wan-policy", Labels: map[string]string{}},
			Spec: wireWanPolicySpec{
				InterfaceRef: wireRef{Kind: "Interface", ID: iface.Device},
				Source:       source,
				Priority:     uint32(priority + 1),
			},
		})
	}

	configJSON, err := json.MarshalIndent(wireDocument{Resources: resources}, "", "  ")
	if err != nil {
		return render.Result{}, fmt.Errorf("routerd %q marshal config document: %w", input.Service.Name, err)
	}

	env := render.IfaceEnv(input.Deployment, input.Service, inst)
	composeYAML := renderRouterdCompose(input, inst)

	return render.Result{
		Renderer: "routerd-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.yaml", Content: composeYAML},
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
			{Key: "etc/routerd/config.json", Content: string(configJSON) + "\n"},
		},
	}, nil
}

// wanSourceFor derives an unbridged interface's WanPolicy source from its
// resolved addressing mode, per the interface-addressing-mode capability.
func wanSourceFor(dep plan.Deployment, iface plan.Interface) (wireWanSource, error) {
	addressing := iface.Addressing
	if addressing == "" {
		addressing = manifest.AddressingDHCP
	}
	if addressing != manifest.AddressingStatic {
		return wireWanSource{Kind: "dhcp"}, nil
	}

	if iface.IPv4 == "" {
		return wireWanSource{}, fmt.Errorf("addressing: static requires a resolved ipv4 address")
	}
	if iface.Gateway4 == "" {
		return wireWanSource{}, fmt.Errorf("addressing: static requires a resolved gateway (WanPolicy.source.Static.gateway is mandatory)")
	}
	prefixLen, err := resolvedPrefixLen(dep, iface.Role)
	if err != nil {
		return wireWanSource{}, err
	}
	return wireWanSource{
		Kind:      "static",
		Address:   iface.IPv4,
		PrefixLen: &prefixLen,
		Gateway:   iface.Gateway4,
	}, nil
}

func resolvedPrefixLen(dep plan.Deployment, role string) (int, error) {
	n := dep.Network(role)
	if n == nil || n.IPv4 == nil || n.IPv4.CIDR == "" {
		return 0, fmt.Errorf("no resolved ipv4 network CIDR for role %q", role)
	}
	_, ipNet, err := net.ParseCIDR(n.IPv4.CIDR)
	if err != nil {
		return 0, fmt.Errorf("parse network cidr for role %q: %w", role, err)
	}
	ones, _ := ipNet.Mask.Size()
	return ones, nil
}

// parseHostCIDR splits a "host-address/prefix" string (e.g. a
// services[].bridges[].ipv4 value) into its address and prefix length,
// keeping the host bits — unlike net.ParseCIDR's *net.IPNet return, which
// masks them off.
func parseHostCIDR(cidr string) (string, int, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid ipv4 %q: %w", cidr, err)
	}
	ones, _ := ipNet.Mask.Size()
	return ip.String(), ones, nil
}

// renderRouterdCompose generates the per-instance Compose fragment. routerd
// needs NET_ADMIN/NET_RAW for its netlink controllers and mounts the rendered
// instance directory read-only, mirroring bng's curated compose shape.
func renderRouterdCompose(input render.Input, inst plan.Instance) string {
	svc, topNets := servicetemplate.BuildComposeService(input, inst, servicetemplate.DefaultAttachment)
	svc["privileged"] = true
	svc["cap_add"] = []string{"NET_ADMIN", "NET_RAW"}
	svc["volumes"] = append([]string{fmt.Sprintf("./instances/%d:/runtime-config:ro", inst.Index+1)}, input.Service.Volumes...)
	if len(input.Service.Ports) > 0 {
		svc["ports"] = append([]string(nil), input.Service.Ports...)
	}
	svcNets, _ := svc["networks"].(map[string]any)
	servicetemplate.AttachHealthPublication(input, inst, input.HealthPorts[inst.Index], topNets, svcNets, svc)
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, inst.Index+1)
	doc := map[string]any{
		"services": map[string]any{instanceName: svc},
		"networks": topNets,
	}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// Register installs the routerd service type. It is idempotent so tests and
// the daemon can call it freely.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
