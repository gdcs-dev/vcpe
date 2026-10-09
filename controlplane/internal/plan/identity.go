package plan

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
)

// ifnameMax is the usable length of a Linux network interface name. The kernel
// IFNAMSIZ constant is 16 including the trailing NUL, leaving 15 usable bytes.
const ifnameMax = 15

const managerNameMax = 31

// InstanceName is the stable 1-based external name for a service replica.
func InstanceName(service string, index int) string {
	return fmt.Sprintf("%s-%d", service, index+1)
}

// ContainerName is the stable Podman container name for a service replica.
func ContainerName(deployment, service string, index int) string {
	return deployment + "-" + InstanceName(service, index)
}

// RadioManagerName derives a stable manager identity within the manager's
// 31-character name limit.
func RadioManagerName(deployment, service string, index int, logicalName string) string {
	sum := radioIdentityHash("manager", deployment, service, index, logicalName)
	return "vcpe-" + fmt.Sprintf("%x", sum[:13])
}

// CanonicalRadioMAC derives a domain-separated locally administered unicast
// MAC for a radio identity.
func CanonicalRadioMAC(deployment, service string, index int, logicalName string) string {
	sum := radioIdentityHash("mac", deployment, service, index, logicalName)
	first := (sum[0] | 0x02) & 0xfe
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", first, sum[1], sum[2], sum[3], sum[4], sum[5])
}

// VAPDeviceName derives a Linux interface name for a secondary BSS slot.
// Slot 0 uses the manager-created primary radio device instead.
func VAPDeviceName(deployment, service string, index int, radioName string, slot int) string {
	sum := radioIdentityHash(fmt.Sprintf("vap-device/%d", slot), deployment, service, index, radioName)
	return fmt.Sprintf("vap%x", sum[:6])
}

// CanonicalVAPBSSID derives a slot-stable locally administered unicast BSSID.
func CanonicalVAPBSSID(deployment, service string, index int, radioName string, slot int) string {
	if slot == 0 {
		return CanonicalRadioMAC(deployment, service, index, radioName)
	}
	sum := radioIdentityHash(fmt.Sprintf("vap-bssid/%d", slot), deployment, service, index, radioName)
	first := (sum[0] | 0x02) & 0xfe
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", first, sum[1], sum[2], sum[3], sum[4], sum[5])
}

func radioIdentityHash(domain, deployment, service string, index int, logicalName string) [sha256.Size]byte {
	key := fmt.Sprintf("vcpe-radio/%s\x00%s\x00%s\x00%d\x00%s", domain, deployment, service, index, logicalName)
	return sha256.Sum256([]byte(key))
}

// CanonicalMAC derives a stable, locally-administered unicast MAC address from
// the deployment-scoped identity tuple. The key is always
// metadata.name/service/role/index (0-based index is always included so that
// MAC derivation is stable when replica count changes). The same helper is used
// by the planner and the runtime-init contract builder so both agree
// byte-for-byte.
func CanonicalMAC(deployment, service, role string, index int) string {
	key := fmt.Sprintf("%s/%s/%s/%d", deployment, service, role, index)
	sum := sha1.Sum([]byte(key))
	// 0x02 sets the locally-administered bit and clears the multicast bit.
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3], sum[4])
}

// DeriveBridgeName returns the default bridge name for a network role,
// "<deployment>-<role>", truncated to fit IFNAMSIZ. When truncation is needed a
// short hash suffix preserves uniqueness. The boolean reports whether the
// untruncated name exceeded the limit so callers can warn.
func DeriveBridgeName(deployment, role string) (string, bool) {
	full := deployment + "-" + role
	if len(full) <= ifnameMax {
		return full, false
	}
	sum := sha1.Sum([]byte(full))
	suffix := fmt.Sprintf("%x", sum[:2]) // 4 hex chars
	keep := ifnameMax - 1 - len(suffix)  // room for '-' + suffix
	if keep < 1 {
		keep = 1
	}
	return full[:keep] + "-" + suffix, true
}
