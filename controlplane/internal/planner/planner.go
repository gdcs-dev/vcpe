// Package planner resolves a validated manifest into a concrete plan.Deployment.
// It derives network bridges, orders services by their dependsOn graph, and
// computes per-replica interface identities (device, MAC, gateways) using the
// shared determinism helpers so the runtime-init contract agrees byte-for-byte.
// IPAM remains the sole authority for dynamic IP assignment; the planner only
// carries explicit addresses and leaves dynamic ones empty for IPAM to fill.
package planner

import (
	"fmt"
	"net"
	"sort"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// Build resolves the manifest into a deployment plan. Services in the result
// are ordered for startup (dependencies first); reverse the slice for teardown.
func Build(doc manifest.Document) (plan.Deployment, error) {
	networks := resolveNetworks(doc)
	netByRole := map[string]plan.Network{}
	for _, n := range networks {
		netByRole[n.Role] = n
	}

	ordered, err := orderServices(doc.Spec.Services)
	if err != nil {
		return plan.Deployment{}, err
	}

	services := make([]plan.Service, 0, len(ordered))
	for _, svc := range ordered {
		services = append(services, resolveService(doc.Metadata.Name, svc, netByRole))
	}

	return plan.Deployment{
		Name:     doc.Metadata.Name,
		Labels:   doc.Metadata.Labels,
		Networks: networks,
		Services: services,
	}, nil
}

func resolveNetworks(doc manifest.Document) []plan.Network {
	out := make([]plan.Network, 0, len(doc.Spec.Networks))
	for _, n := range doc.Spec.Networks {
		bridge := n.Bridge
		if bridge == "" {
			bridge, _ = plan.DeriveBridgeName(doc.Metadata.Name, n.Role)
		}
		out = append(out, plan.Network{
			Role:     n.Role,
			Bridge:   bridge,
			NAT:      n.NAT,
			Firewall: n.Firewall,
			IPv4:     resolveFamily(n.IPv4),
			IPv6:     resolveFamily(n.IPv6),
		})
	}
	// For networks that a gateway service uses as LAN ports the container's
	// brlan0 will claim the network gateway IP (.1). The Podman host bridge
	// must not also claim it or ARP conflicts arise. Use the last usable IP
	// (.254 for /24) for the Podman bridge on those networks instead.
	lanRoles := gatewayLANRoles(doc)
	for i, n := range out {
		if lanRoles[n.Role] && n.IPv4 != nil {
			if gw, err := lastUsableIP(n.IPv4.CIDR); err == nil {
				out[i].HostBridgeGateway = gw
			}
			// Tell Podman to write the container-side gateway (.1) as
			// DNS in container resolv.conf. The gateway's brlan0 dnsmasq
			// listens there and forwards queries upstream to BNG.
			out[i].PodmanDNS = n.IPv4.Gateway
		}
	}
	return out
}

// gatewayLANRoles returns the set of network roles used as LAN ports by any
// gateway-type service. WAN and CM roles (as declared in the gateway config)
// are excluded; every other interface role is considered a LAN port.
func gatewayLANRoles(doc manifest.Document) map[string]bool {
	roles := map[string]bool{}
	for _, svc := range doc.Spec.Services {
		if svc.Type != "gateway" {
			continue
		}
		var cfg struct {
			Erouter struct {
				WanRole string `yaml:"wanRole"`
				CMRole  string `yaml:"cmRole"`
			} `yaml:"erouter"`
		}
		_ = svc.Config.Decode(&cfg)
		wanRole := cfg.Erouter.WanRole
		if wanRole == "" {
			wanRole = "wan"
		}
		cmRole := cfg.Erouter.CMRole
		if cmRole == "" {
			cmRole = "cm"
		}
		for _, iface := range svc.Interfaces {
			if iface.Role != wanRole && iface.Role != cmRole {
				roles[iface.Role] = true
			}
		}
	}
	return roles
}

// lastUsableIP returns the last usable host address in a CIDR (broadcast - 1).
func lastUsableIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		ip = ipNet.IP.To16()
	}
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^ipNet.Mask[i]
	}
	last := make(net.IP, len(broadcast))
	copy(last, broadcast)
	for i := len(last) - 1; i >= 0; i-- {
		if last[i] > 0 {
			last[i]--
			break
		}
	}
	return last.String(), nil
}

func resolveFamily(fam *manifest.AddressFamily) *plan.Family {
	if fam == nil {
		return nil
	}
	f := &plan.Family{CIDR: fam.CIDR, Gateway: fam.Gateway}
	if fam.Pool != nil {
		f.Pool = &plan.Pool{Start: fam.Pool.Start, End: fam.Pool.End}
	}
	return f
}

func resolveService(deployment string, svc manifest.Service, netByRole map[string]plan.Network) plan.Service {
	replicas := svc.Replicas
	out := plan.Service{
		Name:      svc.Name,
		Type:      svc.Type,
		Replicas:  replicas,
		Image:     svc.Image,
		DependsOn: append([]string(nil), svc.DependsOn...),
		Ports:     append([]string(nil), svc.Ports...),
		Volumes:   append([]string(nil), svc.Volumes...),
		Config:    svc.Config,
	}
	if replicas <= 0 {
		return out
	}
	for i := 0; i < replicas; i++ {
		out.Instances = append(out.Instances, resolveInstance(deployment, svc, i, replicas, netByRole))
	}
	return out
}

func resolveInstance(deployment string, svc manifest.Service, index, replicas int, netByRole map[string]plan.Network) plan.Instance {
	inst := plan.Instance{Index: index}
	for pos, iface := range svc.Interfaces {
		net := netByRole[iface.Role]

		device := iface.Device
		if device == "" {
			device = fmt.Sprintf("eth%d", pos)
		}

		mac := iface.MAC
		macIndex := 0
		if replicas > 1 {
			// Explicit MAC/address are ambiguous across replicas; derive per index.
			mac = ""
			macIndex = index
		}
		if mac == "" {
			mac = plan.CanonicalMAC(deployment, svc.Name, iface.Role, macIndex)
		}

		ipv4, ipv6 := "", ""
		if replicas == 1 {
			ipv4 = iface.IPv4
			ipv6 = iface.IPv6
		}

		resolved := plan.Interface{
			Role:         iface.Role,
			Network:      net.Bridge,
			Device:       device,
			MAC:          mac,
			IPv4:         ipv4,
			IPv6:         ipv6,
			DefaultRoute: iface.DefaultRoute,
		}
		if net.IPv4 != nil {
			resolved.Gateway4 = net.IPv4.Gateway
		}
		if net.IPv6 != nil {
			resolved.Gateway6 = net.IPv6.Gateway
		}
		inst.Interfaces = append(inst.Interfaces, resolved)
	}
	return inst
}

// orderServices returns services in deterministic startup order: a stable
// topological sort of the dependsOn graph (dependencies first). The manifest is
// assumed already validated for unknown deps and cycles.
func orderServices(services []manifest.Service) ([]manifest.Service, error) {
	byName := map[string]manifest.Service{}
	indegree := map[string]int{}
	dependents := map[string][]string{}
	names := make([]string, 0, len(services))
	for _, svc := range services {
		byName[svc.Name] = svc
		indegree[svc.Name] = 0
		names = append(names, svc.Name)
	}
	sort.Strings(names)
	for _, svc := range services {
		for _, dep := range svc.DependsOn {
			dependents[dep] = append(dependents[dep], svc.Name)
			indegree[svc.Name]++
		}
	}

	ready := []string{}
	for _, name := range names {
		if indegree[name] == 0 {
			ready = append(ready, name)
		}
	}

	ordered := make([]manifest.Service, 0, len(services))
	for len(ready) > 0 {
		sort.Strings(ready)
		name := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byName[name])
		deps := append([]string(nil), dependents[name]...)
		sort.Strings(deps)
		for _, d := range deps {
			indegree[d]--
			if indegree[d] == 0 {
				ready = append(ready, d)
			}
		}
	}

	if len(ordered) != len(services) {
		return nil, fmt.Errorf("dependsOn cycle detected while ordering services")
	}
	return ordered, nil
}
