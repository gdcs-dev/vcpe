package app

import (
	"fmt"
	"sort"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"gopkg.in/yaml.v3"
)

// classifyDisruptive compares a desired v1 manifest against persisted state and
// reports whether applying it would be disruptive, with human-readable reasons.
// Disruptive changes are: a network CIDR change for an existing role, a
// deployment-identity reset (prior snapshot under the same name declared a
// different metadata.name), and scaling a previously-running service to zero.
func classifyDisruptive(ps *persist.Store, doc manifest.Document) (bool, []string, error) {
	name := doc.Metadata.Name
	var reasons []string

	// CIDR changes vs. persisted leases for this deployment.
	leases, err := ps.ListIPAMLeases()
	if err != nil {
		return false, nil, err
	}
	existingCIDR := map[string]string{}
	for _, l := range leases {
		if l.CustomerID == name {
			existingCIDR[l.Role] = l.CIDR
		}
	}
	for _, n := range doc.Spec.Networks {
		prior, ok := existingCIDR[n.Role]
		if !ok {
			continue
		}
		desired := primaryCIDR(n)
		if desired != "" && prior != "" && desired != prior {
			reasons = append(reasons, fmt.Sprintf("network role %q CIDR changes from %s to %s", n.Role, prior, desired))
		}
	}

	// Identity reset and scale-to-zero vs. the prior desired snapshot.
	if snap, ok, err := ps.LatestDesiredSnapshot(name); err != nil {
		return false, nil, err
	} else if ok {
		var prev manifest.Document
		if yaml.Unmarshal(snap, &prev) == nil {
			if prev.Metadata.Name != "" && prev.Metadata.Name != name {
				reasons = append(reasons, fmt.Sprintf("deployment identity reset: %s -> %s", prev.Metadata.Name, name))
			}
			prevReplicas := map[string]int{}
			for _, svc := range prev.Spec.Services {
				prevReplicas[svc.Name] = svc.Replicas
			}
			for _, svc := range doc.Spec.Services {
				if was, ok := prevReplicas[svc.Name]; ok && was > 0 && svc.Replicas == 0 {
					reasons = append(reasons, fmt.Sprintf("service %q scale-to-zero (was %d replicas)", svc.Name, was))
				}
			}
		}
	}

	sort.Strings(reasons)
	return len(reasons) > 0, reasons, nil
}

// primaryCIDR returns the IPv4 CIDR for a network, falling back to IPv6. It
// mirrors the lease key the IPAM store persists.
func primaryCIDR(n manifest.Network) string {
	if n.IPv4 != nil && n.IPv4.CIDR != "" {
		return n.IPv4.CIDR
	}
	if n.IPv6 != nil && n.IPv6.CIDR != "" {
		return n.IPv6.CIDR
	}
	return ""
}
