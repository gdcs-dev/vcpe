package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// ImageRef returns the fully qualified image reference, defaulting the tag to
// "latest" when unset.
func ImageRef(img manifest.Image) string {
	tag := img.Tag
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	return img.Repository + ":" + tag
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
		lines = append(lines,
			prefix+"NETWORK="+iface.Network,
			prefix+"DEVICE="+iface.Device,
			prefix+"BRIDGE="+iface.Bridge,
			prefix+"MAC="+iface.MAC,
			prefix+"IPV4="+iface.IPv4,
			prefix+"IPV6="+iface.IPv6,
			prefix+"GATEWAY4="+iface.Gateway4,
			prefix+"GATEWAY6="+iface.Gateway6,
		)
	}

	head := lines[:3]
	tail := lines[3:]
	sort.Strings(tail)
	return append(head, tail...)
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
