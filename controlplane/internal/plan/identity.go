package plan

import (
	"crypto/sha1"
	"fmt"
)

// ifnameMax is the usable length of a Linux network interface name. The kernel
// IFNAMSIZ constant is 16 including the trailing NUL, leaving 15 usable bytes.
const ifnameMax = 15

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
