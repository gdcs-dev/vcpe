// Package plan holds the resolved, concrete deployment model produced by the
// planner from a desired-state manifest. Renderers, the runtime-init contract
// builder, the host-network adapter, and the compose adapter all consume this
// model so that interface identities (device, MAC, addresses, gateways) are
// computed exactly once and stay consistent across components.
package plan

import (
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"gopkg.in/yaml.v3"
)

// Deployment is the fully resolved plan for a manifest.
type Deployment struct {
	Name              string
	Labels            map[string]string
	Networks          []Network
	WirelessMedia     []WirelessMedium
	WirelessNetworks  []WirelessNetwork
	WirelessScenarios []WirelessScenario
	Services          []Service
}

type ScenarioStation struct {
	Service     string
	Replica     int
	Radio       string
	Device      string
	MAC         string
	ManagerName string
}

type ScenarioStep struct {
	AtMs  int
	AP    RoamingCandidate
	SNRDb int
}

type ScenarioAssertions struct {
	InitialAP RoamingCandidate
	FinalAP   RoamingCandidate
	SameIPv4  bool
	MaxRoamMs int
	MaxGapMs  int
}

type WirelessScenario struct {
	Name       string
	Station    ScenarioStation
	APs        []RoamingCandidate
	Steps      []ScenarioStep
	Assertions ScenarioAssertions
}

// WirelessMedium is the resolved RF policy shared by radios on one medium.
type WirelessMedium struct {
	Name     string
	Band     string
	Channel  int
	WidthMHz int
}

// WirelessNetwork is resolved wireless policy and never becomes a Podman
// network or an IPAM allocation.
type WirelessNetwork struct {
	Name                string
	SSID                string
	Channel             int
	Security            string
	PassphraseSecretRef string
}

// Network is a resolved host-attached segment.
type Network struct {
	Role     string
	Bridge   string
	NAT      bool
	Firewall bool
	IPv4     *Family
	IPv6     *Family
	// Driver is the Podman network driver. Empty means bridge (Podman default).
	Driver string
	// DriverOptions are driver-specific options (e.g., parent for macvlan).
	DriverOptions map[string]string
	// IPAMDriver is an optional custom IPAM driver (passthrough).
	IPAMDriver string
	// HostBridgeGateway, when non-empty, is passed as --gateway to
	// podman network create so the Podman host bridge uses this IP
	// instead of the default (first usable IP). Set by the planner for
	// gateway LAN networks where the container's brlan0 will claim the
	// first IP; using the last usable IP avoids the ARP conflict.
	HostBridgeGateway string
	// PodmanDNS, when non-empty, is passed as --dns to podman network
	// create so the DNS server written into container resolv.conf is
	// this IP rather than the Podman bridge gateway. Set to the LAN
	// network's container-side gateway (.1) for gateway LAN networks so
	// dnsmasq on brlan0 handles DNS queries from LAN clients.
	PodmanDNS string
}

// Family is a resolved address family for a network.
type Family struct {
	CIDR    string
	Gateway string
	Pool    *Pool
}

// Pool is a resolved dynamic-allocation range.
type Pool struct {
	Start string
	End   string
}

// Service is a resolved workload with one entry in Instances per replica.
type Service struct {
	Name string
	Type string
	// Replicas is the desired replica count from the manifest.
	Replicas  int
	Image     manifest.Image
	DependsOn []string
	Bridges   []manifest.BridgeSpec
	Ports     []string
	Volumes   []string
	Config    yaml.Node
	Instances []Instance
	// PreviousReplicaCount is the replica count from the last successful apply,
	// read from persisted deployment state. Zero means no prior apply.
	PreviousReplicaCount int
	// Delta is the set of 0-based replica indices to add and remove during a
	// delta apply. It is computed from PreviousReplicaCount vs Replicas.
	Delta ReplicaDelta
}

// ReplicaDelta contains the 0-based replica indices to add and remove during
// a delta apply operation.
type ReplicaDelta struct {
	// ToAdd holds 0-based indices of replicas to create, in ascending order.
	ToAdd []int
	// ToRemove holds 0-based indices of replicas to remove, in descending
	// order (highest index first) to minimise disruption to lower-index replicas.
	ToRemove []int
}

// Instance is a single replica with its concrete interface identities.
// Index is 0-based internally; the external compose service name uses Index+1.
type Instance struct {
	Index         int
	InstanceName  string
	ContainerName string
	Interfaces    []Interface
	Radios        []Radio
}

// ComposeServiceName returns the planner-owned external replica name, with a
// deterministic fallback for tests or callers that construct plans directly.
func (i Instance) ComposeServiceName(service string) string {
	if i.InstanceName != "" {
		return i.InstanceName
	}
	return InstanceName(service, i.Index)
}

// PodmanContainerName returns the planner-owned container name, with a
// deterministic fallback for plans constructed outside the planner.
func (i Instance) PodmanContainerName(deployment, service string) string {
	if i.ContainerName != "" {
		return i.ContainerName
	}
	return ContainerName(deployment, service, i.Index)
}

// Radio is a resolved late-attached wireless device. GroupMask is assigned by
// persisted deployment allocation before runtime reconciliation.
type Radio struct {
	Name          string
	Medium        string
	Network       string
	APBSSID       string
	RoamingMedia  []string
	Candidates    []RoamingCandidate
	VAPs          []VAP
	Device        string
	Mode          string
	Mesh          *manifest.Mesh
	Bridge        string
	Addressing    string
	DefaultRoute  bool
	ManagerName   string
	MAC           string
	GroupMask     uint64
	ContainerName string
}

type RoamingCandidate struct {
	Service string
	Replica int
	Radio   string
	Slot    int
	Medium  string
	Band    string
	Channel int
	BSSID   string
}

// VAP is a resolved AP BSS bound to one radio slot and bridge.
type VAP struct {
	Slot    int
	Network string
	Bridge  string
	Device  string
	MAC     string
}

// Interface is a resolved attachment to a network role.
type Interface struct {
	Role         string
	Network      string
	Device       string
	Bridge       string
	MAC          string
	IPv4         string
	IPv6         string
	Gateway4     string
	Gateway6     string
	DefaultRoute bool
	// Addressing is the resolved "dhcp" or "static" mode for this interface;
	// see manifest.Interface.Addressing. Always normalized to a concrete value
	// (never empty) by the planner.
	Addressing string
	// ManagedNetwork is true when this interface's network has its address
	// assigned by Podman itself (any IPAMDriver other than "none"), as
	// opposed to a container-managed (ipamDriver: none) segment. Addressing:
	// dhcp has no runtime effect (no DHCP client is invoked) on a managed
	// network, since Podman has already assigned the address before the
	// container starts.
	ManagedNetwork bool
}

// Network returns the resolved network for a role, or nil when absent.
func (d Deployment) Network(role string) *Network {
	for i := range d.Networks {
		if d.Networks[i].Role == role {
			return &d.Networks[i]
		}
	}
	return nil
}

// WirelessNetwork returns the resolved wireless medium by name, or nil when
// absent.
func (d Deployment) WirelessNetwork(name string) *WirelessNetwork {
	for i := range d.WirelessNetworks {
		if d.WirelessNetworks[i].Name == name {
			return &d.WirelessNetworks[i]
		}
	}
	return nil
}

// WirelessMedium returns the resolved RF policy by name, or nil when absent.
func (d Deployment) WirelessMedium(name string) *WirelessMedium {
	for i := range d.WirelessMedia {
		if d.WirelessMedia[i].Name == name {
			return &d.WirelessMedia[i]
		}
	}
	return nil
}
