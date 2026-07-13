package manifest

import (
	"fmt"
	"net/netip"
)

// Validate performs schema-structural validation that does not require the
// service-type registry. Type-aware validation (per-type Config decoding and
// expected-role satisfaction) and IP allocation are layered on top by the
// orchestrator and IPAM, which own the registry and lease state respectively.
func Validate(doc Document) error {
	if doc.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q: expected %q", doc.APIVersion, APIVersion)
	}
	if doc.Kind != Kind {
		return fmt.Errorf("unsupported kind %q: expected %q", doc.Kind, Kind)
	}
	if doc.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if len(doc.Spec.Services) == 0 {
		return fmt.Errorf("spec.services must include at least one service")
	}

	networks, err := validateNetworks(doc)
	if err != nil {
		return err
	}
	if err := validateServices(doc, networks); err != nil {
		return err
	}
	if err := validateSecrets(doc); err != nil {
		return err
	}

	return nil
}

// validateNetworks validates network declarations and returns a map of declared
// role -> parsed prefixes (one per address family) for downstream checks.
func validateNetworks(doc Document) (map[string][]netip.Prefix, error) {
	roles := map[string][]netip.Prefix{}
	cidrs := map[string]struct{}{}
	for _, n := range doc.Spec.Networks {
		if n.Role == "" {
			return nil, fmt.Errorf("spec.networks[].role is required")
		}
		if _, dup := roles[n.Role]; dup {
			return nil, fmt.Errorf("duplicate network role %q", n.Role)
		}
		// Driver-specific validation.
		if n.Driver != "" && n.Driver != "bridge" {
			if n.NAT {
				return nil, fmt.Errorf("network role %q: nat is not supported for driver %q", n.Role, n.Driver)
			}
			if n.Firewall {
				return nil, fmt.Errorf("network role %q: firewall is not supported for driver %q", n.Role, n.Driver)
			}
		}
		if n.Driver == "macvlan" || n.Driver == "ipvlan" {
			if n.DriverOptions["parent"] == "" {
				return nil, fmt.Errorf("network role %q: driver %q requires driverOptions.parent", n.Role, n.Driver)
			}
		}
		prefixes := []netip.Prefix{}
		for family, fam := range map[string]*AddressFamily{"ipv4": n.IPv4, "ipv6": n.IPv6} {
			if fam == nil {
				continue
			}
			prefix, err := netip.ParsePrefix(fam.CIDR)
			if err != nil {
				return nil, fmt.Errorf("invalid %s CIDR %q for role %q: %w", family, fam.CIDR, n.Role, err)
			}
			if _, dup := cidrs[fam.CIDR]; dup {
				return nil, fmt.Errorf("duplicate network CIDR %q", fam.CIDR)
			}
			cidrs[fam.CIDR] = struct{}{}
			if fam.Gateway != "" {
				gw, err := netip.ParseAddr(fam.Gateway)
				if err != nil {
					return nil, fmt.Errorf("invalid %s gateway %q for role %q: %w", family, fam.Gateway, n.Role, err)
				}
				if !prefix.Contains(gw) {
					return nil, fmt.Errorf("%s gateway %q is outside CIDR %q for role %q", family, fam.Gateway, fam.CIDR, n.Role)
				}
			}
			if fam.Pool != nil {
				if err := validatePool(family, n.Role, prefix, fam.Pool); err != nil {
					return nil, err
				}
			}
			prefixes = append(prefixes, prefix)
		}
		roles[n.Role] = prefixes
	}
	return roles, nil
}

func validatePool(family, role string, prefix netip.Prefix, pool *Pool) error {
	start, err := netip.ParseAddr(pool.Start)
	if err != nil {
		return fmt.Errorf("invalid %s pool start %q for role %q: %w", family, pool.Start, role, err)
	}
	end, err := netip.ParseAddr(pool.End)
	if err != nil {
		return fmt.Errorf("invalid %s pool end %q for role %q: %w", family, pool.End, role, err)
	}
	if !prefix.Contains(start) || !prefix.Contains(end) {
		return fmt.Errorf("%s pool %s-%s is outside CIDR %q for role %q", family, pool.Start, pool.End, prefix.String(), role)
	}
	if start.Compare(end) > 0 {
		return fmt.Errorf("%s pool start %q is greater than end %q for role %q", family, pool.Start, pool.End, role)
	}
	return nil
}

func validateServices(doc Document, networks map[string][]netip.Prefix) error {
	names := map[string]struct{}{}
	for _, svc := range doc.Spec.Services {
		if svc.Name == "" {
			return fmt.Errorf("spec.services[].name is required")
		}
		if _, dup := names[svc.Name]; dup {
			return fmt.Errorf("duplicate service name %q", svc.Name)
		}
		names[svc.Name] = struct{}{}
		if svc.Type == "" {
			return fmt.Errorf("service %q is missing type", svc.Name)
		}
		if svc.Replicas < 0 {
			return fmt.Errorf("service %q has invalid replicas %d: must be >= 0", svc.Name, svc.Replicas)
		}
		if doc.Spec.MaxReplicasPerService > 0 && svc.Replicas > doc.Spec.MaxReplicasPerService {
			return fmt.Errorf("service %q replicas %d exceed maxReplicasPerService %d", svc.Name, svc.Replicas, doc.Spec.MaxReplicasPerService)
		}
		if err := validateServiceInterfaces(svc, networks); err != nil {
			return err
		}
	}
	return validateDependsOn(doc)
}

func validateServiceInterfaces(svc Service, networks map[string][]netip.Prefix) error {
	defaultRoutes := 0
	for _, iface := range svc.Interfaces {
		if iface.Role == "" {
			return fmt.Errorf("service %q has an interface with no role", svc.Name)
		}
		prefixes, ok := networks[iface.Role]
		if !ok {
			return fmt.Errorf("service %q interface references unknown network role %q", svc.Name, iface.Role)
		}
		// Explicit MAC/addresses are unambiguous only for a single replica; with
		// replicas > 1 IPAM allocates per replica and MACs are indexed.
		if svc.Replicas > 1 && (iface.MAC != "" || iface.IPv4 != "" || iface.IPv6 != "") {
			return fmt.Errorf("service %q interface role %q sets an explicit mac/address but replicas is %d: explicit identities require replicas: 1", svc.Name, iface.Role, svc.Replicas)
		}
		if iface.IPv4 != "" {
			if err := assertWithin("ipv4", svc.Name, iface.IPv4, prefixes); err != nil {
				return err
			}
		}
		if iface.IPv6 != "" {
			if err := assertWithin("ipv6", svc.Name, iface.IPv6, prefixes); err != nil {
				return err
			}
		}
		if iface.DefaultRoute {
			defaultRoutes++
		}
	}
	if defaultRoutes > 1 {
		return fmt.Errorf("service %q declares %d default routes: at most one interface may set defaultRoute", svc.Name, defaultRoutes)
	}
	return nil
}

func assertWithin(family, service, addr string, prefixes []netip.Prefix) error {
	parsed, err := netip.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("service %q has invalid %s address %q: %w", service, family, addr, err)
	}
	for _, p := range prefixes {
		if p.Contains(parsed) {
			return nil
		}
	}
	return fmt.Errorf("service %q %s address %q is outside the CIDR(s) of its network role", service, family, addr)
}

func validateDependsOn(doc Document) error {
	graph := map[string][]string{}
	known := map[string]struct{}{}
	for _, svc := range doc.Spec.Services {
		known[svc.Name] = struct{}{}
	}
	for _, svc := range doc.Spec.Services {
		for _, dep := range svc.DependsOn {
			if _, ok := known[dep]; !ok {
				return fmt.Errorf("service %q depends on unknown service %q", svc.Name, dep)
			}
			if dep == svc.Name {
				return fmt.Errorf("service %q cannot depend on itself", svc.Name)
			}
			graph[svc.Name] = append(graph[svc.Name], dep)
		}
	}
	return detectCycle(graph)
}

func detectCycle(graph map[string][]string) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(node string) error
	visit = func(node string) error {
		color[node] = gray
		for _, next := range graph[node] {
			switch color[next] {
			case gray:
				return fmt.Errorf("dependsOn cycle detected involving service %q", next)
			case white:
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}
	for node := range graph {
		if color[node] == white {
			if err := visit(node); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSecrets(doc Document) error {
	for _, s := range doc.Spec.Secrets {
		if s.Name == "" || s.Provider == "" || s.Key == "" {
			return fmt.Errorf("spec.secrets[] entries require name, provider, and key")
		}
		switch s.Provider {
		case "env", "file":
		default:
			return fmt.Errorf("unsupported secret provider %q: expected env or file", s.Provider)
		}
	}
	return nil
}
