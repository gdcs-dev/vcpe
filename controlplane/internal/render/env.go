package render

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/imageref"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
)

// ImageRef returns the fully qualified image reference, defaulting the tag to
// "latest" when unset.
func ImageRef(img manifest.Image) string {
	return imageref.Format(img)
}

// IPWithPrefix appends the prefix length from cidr to ip. It returns ip
// unchanged when either value is empty or cidr is invalid.
func IPWithPrefix(ip, cidr string) string {
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

// SortedEnv returns environment entries in lexicographic key order.
func SortedEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, key+"="+values[key])
	}
	return entries
}

// InstanceEnvArtifacts returns the conventional root and per-instance
// compose.env artifacts for a service render input.
func InstanceEnvArtifacts(input Input, entries func(plan.Instance) []string) []Artifact {
	artifacts := make([]Artifact, 0, len(input.Service.Instances)+1)
	for position, instance := range input.Service.Instances {
		content := strings.Join(entries(instance), "\n") + "\n"
		if position == 0 {
			artifacts = append(artifacts, Artifact{Key: "compose.env", Content: content})
		}
		artifacts = append(artifacts, Artifact{Key: fmt.Sprintf("instances/%d/compose.env", instance.Index+1), Content: content})
	}
	return artifacts
}

// envKey normalizes a network role into an environment-variable-safe token:
// upper-cased with hyphens converted to underscores.
func envKey(role string) string {
	return strings.ToUpper(strings.ReplaceAll(role, "-", "_"))
}

// IfaceEnv produces the deterministic, ordered environment lines describing a
// service instance's network attachments. For every interface it emits the
// IFACE_<ROLE>_{NETWORK,DEVICE,MAC,IPV4,IPV6,GATEWAY4,GATEWAY6} family, plus the
// deployment-level DEPLOYMENT_NAME, SERVICE_NAME, and IMAGE keys. Roles that
// repeat within an instance are disambiguated with a numeric suffix.
func IfaceEnv(dep plan.Deployment, svc plan.Service, inst plan.Instance) []string {
	lines := []string{
		"DEPLOYMENT_NAME=" + dep.Name,
		"SERVICE_NAME=" + svc.Name,
		"IMAGE=" + ImageRef(svc.Image),
	}

	roleCount := map[string]int{}
	for _, iface := range inst.Interfaces {
		key := envKey(iface.Role)
		if n := roleCount[iface.Role]; n > 0 {
			key = fmt.Sprintf("%s_%d", key, n)
		}
		roleCount[iface.Role]++

		prefix := "IFACE_" + key + "_"
		addressing := iface.Addressing
		if addressing == "" {
			addressing = manifest.AddressingDHCP
		}
		lines = append(lines,
			prefix+"NETWORK="+iface.Network,
			prefix+"DEVICE="+iface.Device,
			prefix+"BRIDGE="+iface.Bridge,
			prefix+"MAC="+iface.MAC,
			prefix+"IPV4="+iface.IPv4,
			prefix+"IPV6="+iface.IPv6,
			prefix+"GATEWAY4="+iface.Gateway4,
			prefix+"GATEWAY6="+iface.Gateway6,
			prefix+"ADDRESSING="+addressing,
		)
		if iface.DefaultRoute {
			lines = append(lines, prefix+"DEFAULT_ROUTE=1")
		}
		if iface.ManagedNetwork {
			lines = append(lines, prefix+"NETWORK_MANAGED=1")
		}
	}
	lines = append(lines, RadioEnv(dep, inst)...)

	head := lines[:3]
	tail := lines[3:]
	sort.Strings(tail)
	return append(head, tail...)
}

// RadioEnv emits the deterministic RADIO_<NAME> environment contract for all
// late-attached radios in an instance. Logical names are normalized using the
// same environment-key rules as wired interface roles.
func RadioEnv(dep plan.Deployment, inst plan.Instance) []string {
	lines := make([]string, 0, len(inst.Radios)*9)
	for _, radio := range inst.Radios {
		if radio.Medium != "" {
			medium := dep.WirelessMedium(radio.Medium)
			if medium == nil {
				continue
			}
			prefix := "RADIO_" + envKey(radio.Name) + "_"
			defaultRoute := "0"
			if radio.DefaultRoute {
				defaultRoute = "1"
			}
			lines = append(lines,
				prefix+"DEVICE="+radio.Device,
				prefix+"MAC="+radio.MAC,
				prefix+"MODE="+radio.Mode,
				prefix+"MEDIUM="+medium.Name,
				prefix+"BAND="+medium.Band,
				prefix+"CHANNEL="+strconv.Itoa(medium.Channel),
				prefix+"WIDTH_MHZ="+strconv.Itoa(medium.WidthMHz),
				prefix+"ADDRESSING="+radio.Addressing,
				prefix+"DEFAULT_ROUTE="+defaultRoute,
			)
			if radio.Mode == manifest.RadioModeMesh && radio.Mesh != nil {
				lines = append(lines, prefix+"MESH_ID="+radio.Mesh.ID, prefix+"BRIDGE="+radio.Mesh.Bridge,
					prefix+"SAE_FILE="+secrets.MeshCredentialContainerPath(radio.Mesh.ID))
			}
			if radio.Mode == manifest.RadioModeStation {
				if len(radio.RoamingMedia) == 0 {
					lines = append(lines, prefix+"AP_BSSID="+radio.APBSSID)
				} else {
					lines = append(lines, prefix+"CANDIDATE_COUNT="+strconv.Itoa(len(radio.Candidates)))
					for index, candidate := range radio.Candidates {
						candidatePrefix := prefix + "CANDIDATE_" + strconv.Itoa(index) + "_"
						lines = append(lines,
							candidatePrefix+"SERVICE="+candidate.Service,
							candidatePrefix+"REPLICA="+strconv.Itoa(candidate.Replica+1),
							candidatePrefix+"RADIO="+candidate.Radio,
							candidatePrefix+"SLOT="+strconv.Itoa(candidate.Slot),
							candidatePrefix+"MEDIUM="+candidate.Medium,
							candidatePrefix+"BAND="+candidate.Band,
							candidatePrefix+"CHANNEL="+strconv.Itoa(candidate.Channel),
							candidatePrefix+"BSSID="+candidate.BSSID,
						)
					}
				}
				profile := dep.WirelessNetwork(radio.Network)
				if profile != nil {
					lines = append(lines, prefix+"NETWORK="+profile.Name, prefix+"SSID="+profile.SSID, prefix+"SECURITY="+profile.Security)
					if profile.Security != manifest.WirelessOpen {
						lines = append(lines, prefix+"PASSPHRASE_FILE="+secrets.WirelessCredentialContainerPath(profile.Name))
					}
				}
			}
			for _, vap := range radio.VAPs {
				profile := dep.WirelessNetwork(vap.Network)
				if profile == nil {
					continue
				}
				vapPrefix := prefix + "VAP_" + strconv.Itoa(vap.Slot) + "_"
				lines = append(lines,
					vapPrefix+"DEVICE="+vap.Device,
					vapPrefix+"BSSID="+vap.MAC,
					vapPrefix+"NETWORK="+profile.Name,
					vapPrefix+"SSID="+profile.SSID,
					vapPrefix+"SECURITY="+profile.Security,
					vapPrefix+"BRIDGE="+vap.Bridge,
				)
				if profile.Security != manifest.WirelessOpen {
					lines = append(lines, vapPrefix+"PASSPHRASE_FILE="+secrets.WirelessCredentialContainerPath(profile.Name))
				}
			}
			continue
		}
		medium := dep.WirelessNetwork(radio.Network)
		ssid := ""
		channel := 0
		if medium != nil {
			ssid = medium.SSID
			channel = medium.Channel
		}
		defaultRoute := "0"
		if radio.DefaultRoute {
			defaultRoute = "1"
		}
		prefix := "RADIO_" + envKey(radio.Name) + "_"
		lines = append(lines,
			prefix+"DEVICE="+radio.Device,
			prefix+"MAC="+radio.MAC,
			prefix+"MODE="+radio.Mode,
			prefix+"NETWORK="+radio.Network,
			prefix+"SSID="+ssid,
			prefix+"CHANNEL="+strconv.Itoa(channel),
			prefix+"BRIDGE="+radio.Bridge,
			prefix+"ADDRESSING="+radio.Addressing,
			prefix+"DEFAULT_ROUTE="+defaultRoute,
		)
		if medium != nil && (medium.Security == manifest.WirelessWPA2Personal || medium.Security == manifest.WirelessWPA3Personal) {
			lines = append(lines,
				prefix+"SECURITY="+medium.Security,
				prefix+"PASSPHRASE_FILE="+secrets.WirelessCredentialContainerPath(medium.Name),
			)
		}
	}
	sort.Strings(lines)
	return lines
}

// BridgeEnv emits BRIDGE_<KEY>_{NAME,IPV4,IPV6} env vars for each BridgeSpec
// declared in a service's `bridges` list. The container entrypoint uses these
// to create and configure the bridges after the interface rename step.
// KEY is the bridge name uppercased with hyphens converted to underscores.
func BridgeEnv(bridges []manifest.BridgeSpec) []string {
	if len(bridges) == 0 {
		return nil
	}
	var lines []string
	for _, b := range bridges {
		key := strings.ToUpper(strings.ReplaceAll(b.Name, "-", "_"))
		prefix := "BRIDGE_" + key + "_"
		lines = append(lines,
			// NAME carries the actual bridge name so entrypoints can round-trip
			// from the KEY back to the original name (avoids hyphen/underscore ambiguity).
			prefix+"NAME="+b.Name,
			prefix+"IPV4="+b.IPv4,
			prefix+"IPV6="+b.IPv6,
			prefix+"DHCP_START="+b.DHCPStart,
			prefix+"DHCP_END="+b.DHCPEnd,
		)
	}
	sort.Strings(lines)
	return lines
}
