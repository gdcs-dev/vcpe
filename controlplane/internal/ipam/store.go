// Package ipam is the sole authority for IP assignment. It validates and leases
// network CIDRs (rejecting overlaps within a request and against persisted
// leases) and allocates concrete host addresses from declared pools so that no
// other component invents IP addresses.
package ipam

import (
	"fmt"
	"net/netip"
	"sort"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// Lease is a persisted network reservation keyed by deployment and role. The
// persist layer stores the deployment name in its customer_id column.
type Lease struct {
	Deployment string `json:"deployment"`
	Role       string `json:"role"`
	CIDR       string `json:"cidr"`
}

type Store struct {
	persist *persist.Store
}

func NewStore(ps *persist.Store) *Store {
	return &Store{persist: ps}
}

// requestedCIDRs returns every family CIDR a network declares.
func networkCIDRs(n manifest.Network) []string {
	out := []string{}
	if n.IPv4 != nil && n.IPv4.CIDR != "" {
		out = append(out, n.IPv4.CIDR)
	}
	if n.IPv6 != nil && n.IPv6.CIDR != "" {
		out = append(out, n.IPv6.CIDR)
	}
	return out
}

// primaryCIDR returns the CIDR persisted for a role (IPv4 preferred).
func primaryCIDR(n manifest.Network) string {
	if n.IPv4 != nil && n.IPv4.CIDR != "" {
		return n.IPv4.CIDR
	}
	if n.IPv6 != nil && n.IPv6.CIDR != "" {
		return n.IPv6.CIDR
	}
	return ""
}

// CheckConflicts validates requested networks against each other and against
// persisted leases held by other deployments.
func (s *Store) CheckConflicts(deployment string, networks []manifest.Network) ([]string, error) {
	type req struct {
		role string
		cidr string
	}
	requested := []req{}
	for _, n := range networks {
		for _, cidr := range networkCIDRs(n) {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return nil, fmt.Errorf("invalid CIDR %q for role %q: %w", cidr, n.Role, err)
			}
			requested = append(requested, req{role: n.Role, cidr: cidr})
		}
	}

	for i := range requested {
		for j := i + 1; j < len(requested); j++ {
			if requested[i].role == requested[j].role {
				continue
			}
			if overlaps(requested[i].cidr, requested[j].cidr) {
				return nil, fmt.Errorf("requested CIDR %s overlaps %s", requested[i].cidr, requested[j].cidr)
			}
		}
	}

	existing, err := s.Load()
	if err != nil {
		return nil, err
	}
	for _, r := range requested {
		for _, ex := range existing {
			if ex.Deployment == deployment {
				continue
			}
			if overlaps(r.cidr, ex.CIDR) {
				return nil, fmt.Errorf("requested CIDR %s (%s/%s) overlaps existing lease %s (%s/%s)", r.cidr, deployment, r.role, ex.CIDR, ex.Deployment, ex.Role)
			}
		}
	}

	diagnostics := []string{
		fmt.Sprintf("checked %d existing lease(s)", len(existing)),
		fmt.Sprintf("validated %d requested CIDR(s)", len(requested)),
	}
	return diagnostics, nil
}

// Apply persists the network leases for a deployment, replacing prior leases.
func (s *Store) Apply(deployment string, networks []manifest.Network) error {
	leases := make([]persist.IPAMLease, 0, len(networks))
	for _, n := range networks {
		cidr := primaryCIDR(n)
		if cidr == "" {
			continue
		}
		leases = append(leases, persist.IPAMLease{CustomerID: deployment, Role: n.Role, CIDR: cidr})
	}
	return s.persist.ReplaceCustomerLeases(deployment, leases)
}

func (s *Store) Load() ([]Lease, error) {
	pLeases, err := s.persist.ListIPAMLeases()
	if err != nil {
		return nil, err
	}
	leases := make([]Lease, 0, len(pLeases))
	for _, l := range pLeases {
		leases = append(leases, Lease{Deployment: l.CustomerID, Role: l.Role, CIDR: l.CIDR})
	}
	return leases, nil
}

func overlaps(a, b string) bool {
	pa, errA := netip.ParsePrefix(a)
	pb, errB := netip.ParsePrefix(b)
	if errA != nil || errB != nil {
		return false
	}
	return pa.Overlaps(pb)
}

// AllocateInterfaces fills empty interface addresses from their network's pool.
// It is deterministic: addresses are handed out in service/instance/interface
// order from the pool start, skipping the gateway and any explicitly assigned
// addresses. It mutates the plan in place and is the only path that assigns
// dynamic host addresses.
func AllocateInterfaces(dep *plan.Deployment) error {
	netByRole := map[string]plan.Network{}
	for _, n := range dep.Networks {
		netByRole[n.Role] = n
	}

	// Seed used-address sets per role/family with gateways and explicit values.
	used4 := map[string]map[netip.Addr]struct{}{}
	used6 := map[string]map[netip.Addr]struct{}{}
	mark := func(m map[string]map[netip.Addr]struct{}, role, addr string) {
		if addr == "" {
			return
		}
		a, err := netip.ParseAddr(addr)
		if err != nil {
			return
		}
		if m[role] == nil {
			m[role] = map[netip.Addr]struct{}{}
		}
		m[role][a] = struct{}{}
	}
	for _, n := range dep.Networks {
		if n.IPv4 != nil {
			mark(used4, n.Role, n.IPv4.Gateway)
		}
		if n.IPv6 != nil {
			mark(used6, n.Role, n.IPv6.Gateway)
		}
	}
	for si := range dep.Services {
		for ii := range dep.Services[si].Instances {
			for _, iface := range dep.Services[si].Instances[ii].Interfaces {
				mark(used4, iface.Role, iface.IPv4)
				mark(used6, iface.Role, iface.IPv6)
			}
		}
	}

	cursor4 := map[string]netip.Addr{}
	cursor6 := map[string]netip.Addr{}

	for si := range dep.Services {
		for ii := range dep.Services[si].Instances {
			ifaces := dep.Services[si].Instances[ii].Interfaces
			for k := range ifaces {
				iface := &ifaces[k]
				net, ok := netByRole[iface.Role]
				if !ok {
					continue
				}
				if iface.IPv4 == "" && net.IPv4 != nil && net.IPv4.Pool != nil {
					addr, err := allocate(net.IPv4.Pool, cursor4, used4, iface.Role)
					if err != nil {
						return fmt.Errorf("allocate ipv4 for role %q: %w", iface.Role, err)
					}
					iface.IPv4 = addr
				}
				if iface.IPv6 == "" && net.IPv6 != nil && net.IPv6.Pool != nil {
					addr, err := allocate(net.IPv6.Pool, cursor6, used6, iface.Role)
					if err != nil {
						return fmt.Errorf("allocate ipv6 for role %q: %w", iface.Role, err)
					}
					iface.IPv6 = addr
				}
			}
		}
	}
	return nil
}

func allocate(pool *plan.Pool, cursors map[string]netip.Addr, used map[string]map[netip.Addr]struct{}, role string) (string, error) {
	start, err := netip.ParseAddr(pool.Start)
	if err != nil {
		return "", fmt.Errorf("invalid pool start %q: %w", pool.Start, err)
	}
	end, err := netip.ParseAddr(pool.End)
	if err != nil {
		return "", fmt.Errorf("invalid pool end %q: %w", pool.End, err)
	}
	cur, ok := cursors[role]
	if !ok {
		cur = start
	}
	if used[role] == nil {
		used[role] = map[netip.Addr]struct{}{}
	}
	for cur.Compare(end) <= 0 {
		if _, taken := used[role][cur]; !taken {
			used[role][cur] = struct{}{}
			cursors[role] = cur.Next()
			return cur.String(), nil
		}
		cur = cur.Next()
	}
	return "", fmt.Errorf("pool %s-%s exhausted", pool.Start, pool.End)
}

// SortLeases returns leases in deterministic order (for diagnostics/tests).
func SortLeases(leases []Lease) {
	sort.Slice(leases, func(i, j int) bool {
		if leases[i].Deployment == leases[j].Deployment {
			return leases[i].Role < leases[j].Role
		}
		return leases[i].Deployment < leases[j].Deployment
	})
}
