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
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// Build resolves the manifest into a deployment plan. Services in the result
// are ordered for startup (dependencies first); reverse the slice for teardown.
// previousReplicas maps service name to the replica count from the last
// successful apply, enabling delta computation. Pass nil for a fresh deploy.
func Build(doc manifest.Document, previousReplicas map[string]int) (plan.Deployment, error) {
	return buildResolved(doc, previousReplicas)
}

func buildResolved(doc manifest.Document, previousReplicas map[string]int) (plan.Deployment, error) {
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
		prev := 0
		if previousReplicas != nil {
			prev = previousReplicas[svc.Name]
		}
		services = append(services, resolveService(doc.Metadata.Name, svc, prev, netByRole))
	}
	if len(doc.Spec.WirelessMedia) != 0 {
		type apTarget struct {
			candidate plan.RoamingCandidate
			owner     string
		}
		apByPair := map[string][]apTarget{}
		roamingPairs := map[string]bool{}
		fixedPairs := map[string]bool{}
		media := map[string]manifest.WirelessMedium{}
		for _, medium := range doc.Spec.WirelessMedia {
			media[medium.Name] = medium
		}
		for _, service := range doc.Spec.Services {
			for _, radio := range service.Radios {
				if radio.Mode != manifest.RadioModeStation {
					continue
				}
				if radio.Roaming == nil {
					fixedPairs[radio.Medium+"\x00"+radio.Network] = true
				} else {
					for _, medium := range radio.Roaming.Media {
						roamingPairs[medium+"\x00"+radio.Network] = true
					}
				}
			}
		}
		for _, service := range services {
			for _, instance := range service.Instances {
				for _, radio := range instance.Radios {
					if radio.Mode == manifest.RadioModeAP {
						for _, vap := range radio.VAPs {
							key := radio.Medium + "\x00" + vap.Network
							owner := service.Name + "\x00" + radio.Name
							medium := media[radio.Medium]
							apByPair[key] = append(apByPair[key], apTarget{owner: owner, candidate: plan.RoamingCandidate{
								Service: service.Name, Replica: instance.Index, Radio: radio.Name, Slot: vap.Slot,
								Medium: radio.Medium, Band: medium.Band, Channel: medium.Channel, BSSID: vap.MAC,
							}})
						}
					}
				}
			}
		}
		for key, targets := range apByPair {
			for _, target := range targets[1:] {
				if target.owner != targets[0].owner && (!roamingPairs[key] || fixedPairs[key]) {
					medium, network, _ := strings.Cut(key, "\x00")
					return plan.Deployment{}, fmt.Errorf("multiple AP radios for medium %q profile %q", medium, network)
				}
			}
		}
		for serviceIndex := range services {
			for instanceIndex := range services[serviceIndex].Instances {
				for radioIndex := range services[serviceIndex].Instances[instanceIndex].Radios {
					radio := &services[serviceIndex].Instances[instanceIndex].Radios[radioIndex]
					if radio.Mode != manifest.RadioModeStation {
						continue
					}
					if len(radio.RoamingMedia) > 0 {
						for _, medium := range radio.RoamingMedia {
							for _, target := range apByPair[medium+"\x00"+radio.Network] {
								radio.Candidates = append(radio.Candidates, target.candidate)
							}
						}
						if len(radio.Candidates) < 2 {
							return plan.Deployment{}, fmt.Errorf("station radio %q requires at least two roaming AP candidates", radio.Name)
						}
						sort.Slice(radio.Candidates, func(left, right int) bool {
							first, second := radio.Candidates[left], radio.Candidates[right]
							if first.Service != second.Service {
								return first.Service < second.Service
							}
							if first.Replica != second.Replica {
								return first.Replica < second.Replica
							}
							if first.Radio != second.Radio {
								return first.Radio < second.Radio
							}
							return first.Slot < second.Slot
						})
					} else if radio.Medium != "" {
						targets := apByPair[radio.Medium+"\x00"+radio.Network]
						if len(targets) == 0 {
							return plan.Deployment{}, fmt.Errorf("station radio %q has no AP for medium %q profile %q", radio.Name, radio.Medium, radio.Network)
						}
						radio.APBSSID = targets[0].candidate.BSSID
					}
				}
			}
		}
	}

	resolved := plan.Deployment{
		Name:             doc.Metadata.Name,
		Labels:           doc.Metadata.Labels,
		Networks:         networks,
		WirelessMedia:    resolveWirelessMedia(doc.Spec.WirelessMedia),
		WirelessNetworks: resolveWirelessNetworks(doc.Spec.WirelessNetworks),
		Services:         services,
	}
	resolved.WirelessScenarios, err = resolveWirelessScenarios(doc.Spec.WirelessScenarios, services)
	if err != nil {
		return plan.Deployment{}, err
	}
	if len(resolved.WirelessMedia) != 0 {
		if err := checkWirelessIdentityCollisions(resolved); err != nil {
			return plan.Deployment{}, err
		}
	}
	return resolved, nil
}

func resolveWirelessScenarios(scenarios []manifest.WirelessScenario, services []plan.Service) ([]plan.WirelessScenario, error) {
	resolved := make([]plan.WirelessScenario, 0, len(scenarios))
	for _, scenario := range scenarios {
		var station plan.ScenarioStation
		var candidates []plan.RoamingCandidate
		for _, service := range services {
			if service.Name != scenario.Station.Service || scenario.Station.Replica < 1 || scenario.Station.Replica > len(service.Instances) {
				continue
			}
			instance := service.Instances[scenario.Station.Replica-1]
			for _, radio := range instance.Radios {
				if radio.Name == scenario.Station.Radio && len(radio.RoamingMedia) > 0 {
					station = plan.ScenarioStation{Service: service.Name, Replica: instance.Index, Radio: radio.Name, Device: radio.Device, MAC: radio.MAC, ManagerName: radio.ManagerName}
					candidates = radio.Candidates
				}
			}
		}
		if candidates == nil {
			return nil, fmt.Errorf("scenario %q station is not a planned roaming station", scenario.Name)
		}
		candidateFor := func(ref manifest.VAPReference) (plan.RoamingCandidate, error) {
			if ref.Slot != nil {
				for _, candidate := range candidates {
					if candidate.Service == ref.Service && candidate.Replica == ref.Replica-1 && candidate.Radio == ref.Radio && candidate.Slot == *ref.Slot {
						return candidate, nil
					}
				}
			}
			return plan.RoamingCandidate{}, fmt.Errorf("scenario %q AP %q is not a planned roaming candidate", scenario.Name, ref.Radio)
		}
		planned := plan.WirelessScenario{Name: scenario.Name, Station: station}
		for _, ref := range scenario.APs {
			candidate, err := candidateFor(ref)
			if err != nil {
				return nil, err
			}
			planned.APs = append(planned.APs, candidate)
		}
		initial, err := candidateFor(scenario.Assertions.InitialAP)
		if err != nil {
			return nil, err
		}
		final, err := candidateFor(scenario.Assertions.FinalAP)
		if err != nil {
			return nil, err
		}
		planned.Assertions = plan.ScenarioAssertions{InitialAP: initial, FinalAP: final, SameIPv4: scenario.Assertions.SameIPv4, MaxRoamMs: scenario.Assertions.MaxRoamMs, MaxGapMs: scenario.Assertions.MaxGapMs}
		for _, step := range scenario.Steps {
			if step.AtMs == nil || step.SNRDb == nil {
				return nil, fmt.Errorf("scenario %q step requires atMs and snrDb", scenario.Name)
			}
			candidate, err := candidateFor(step.AP)
			if err != nil {
				return nil, err
			}
			planned.Steps = append(planned.Steps, plan.ScenarioStep{AtMs: *step.AtMs, AP: candidate, SNRDb: *step.SNRDb})
		}
		resolved = append(resolved, planned)
	}
	return resolved, nil
}

func checkWirelessIdentityCollisions(deployment plan.Deployment) error {
	macOwners := map[string]string{}
	for _, service := range deployment.Services {
		for _, instance := range service.Instances {
			deviceOwners := map[string]string{}
			register := func(device, mac, owner string) error {
				if previous := deviceOwners[device]; previous != "" {
					return fmt.Errorf("interface name %q collision between %s and %s", device, previous, owner)
				}
				deviceOwners[device] = owner
				key := strings.ToLower(mac)
				if previous := macOwners[key]; previous != "" {
					return fmt.Errorf("MAC %q collision between %s and %s", mac, previous, owner)
				}
				macOwners[key] = owner
				return nil
			}
			for _, iface := range instance.Interfaces {
				if err := register(iface.Device, iface.MAC, service.Name+"/"+instance.InstanceName+"/interface "+iface.Role); err != nil {
					return err
				}
			}
			for _, radio := range instance.Radios {
				owner := service.Name + "/" + instance.InstanceName + "/radio " + radio.Name
				if err := register(radio.Device, radio.MAC, owner); err != nil {
					return err
				}
				for _, vap := range radio.VAPs {
					if vap.Slot == 0 {
						if vap.Device != radio.Device || !strings.EqualFold(vap.MAC, radio.MAC) {
							return fmt.Errorf("%s slot 0 identity differs from primary radio", owner)
						}
						continue
					}
					if err := register(vap.Device, vap.MAC, fmt.Sprintf("%s/slot %d", owner, vap.Slot)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func resolveWirelessNetworks(networks []manifest.WirelessNetwork) []plan.WirelessNetwork {
	resolved := make([]plan.WirelessNetwork, 0, len(networks))
	for _, network := range networks {
		resolved = append(resolved, plan.WirelessNetwork{
			Name: network.Name, SSID: network.SSID, Security: network.Security,
			PassphraseSecretRef: network.PassphraseSecretRef,
		})
	}
	return resolved
}

func resolveWirelessMedia(media []manifest.WirelessMedium) []plan.WirelessMedium {
	resolved := make([]plan.WirelessMedium, 0, len(media))
	for _, medium := range media {
		resolved = append(resolved, plan.WirelessMedium{
			Name: medium.Name, Band: medium.Band, Channel: medium.Channel, WidthMHz: medium.WidthMHz,
		})
	}
	return resolved
}

// computeReplicaDelta derives the set of 0-based replica indices to add and
// remove. ToAdd is ascending; ToRemove is descending (highest index first).
func computeReplicaDelta(previous, desired int) plan.ReplicaDelta {
	delta := plan.ReplicaDelta{}
	// Indices in [previous, desired) are new.
	for i := previous; i < desired; i++ {
		delta.ToAdd = append(delta.ToAdd, i)
	}
	// Indices in [desired, previous) are excess; remove highest first.
	for i := previous - 1; i >= desired; i-- {
		delta.ToRemove = append(delta.ToRemove, i)
	}
	return delta
}

func resolveNetworks(doc manifest.Document) []plan.Network {
	out := make([]plan.Network, 0, len(doc.Spec.Networks))
	for _, n := range doc.Spec.Networks {
		bridge := n.Bridge
		if bridge == "" {
			bridge, _ = plan.DeriveBridgeName(doc.Metadata.Name, n.Role)
		}
		out = append(out, plan.Network{
			Role:          n.Role,
			Bridge:        bridge,
			NAT:           n.NAT,
			Firewall:      n.Firewall,
			IPv4:          resolveFamily(n.IPv4),
			IPv6:          resolveFamily(n.IPv6),
			Driver:        n.Driver,
			DriverOptions: n.DriverOptions,
			IPAMDriver:    n.IPAMDriver,
		})
	}
	// HostBridgeGateway/PodmanDNS are bridge-specific. Skip derivation for
	// non-bridge drivers (macvlan/ipvlan have no host bridge to configure).
	lanRoles := gatewayLANRoles(doc)
	lanBridgeDNS := gatewayLANBridgeDNS(doc)
	for i, n := range out {
		if n.Driver != "" && n.Driver != "bridge" {
			continue // non-bridge driver — no host bridge to configure
		}
		if lanRoles[n.Role] && n.IPv4 != nil {
			if gw, err := lastUsableIP(n.IPv4.CIDR); err == nil {
				out[i].HostBridgeGateway = gw
			}
			// Tell Podman to write the container-side gateway (.1) as
			// DNS in container resolv.conf. The gateway's brlan0 dnsmasq
			// listens there and forwards queries upstream to BNG.
			out[i].PodmanDNS = n.IPv4.Gateway
		} else if lanRoles[n.Role] && n.IPv4 == nil {
			// ipamDriver:none LAN network — no network-level IPv4, but the
			// gateway may have a container-internal bridge for this role.
			// Use that bridge's IP as PodmanDNS so clients get a static
			// resolv.conf pointing to the gateway dnsmasq.
			if dns, ok := lanBridgeDNS[n.Role]; ok {
				out[i].PodmanDNS = dns
			}
		} else if n.IPv4 != nil && n.IPAMDriver != "none" && n.IPv4.Gateway != "" {
			// Non-LAN plain bridge: pass the manifest gateway to podman network
			// create so the actual network gateway matches EROUTER0_IPV4_GATEWAY
			// and the entrypoint's default-route setup is correct.
			out[i].HostBridgeGateway = n.IPv4.Gateway
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

// gatewayLANBridgeDNS returns a map of LAN network role → gateway bridge IP
// for roles whose interfaces are enslaved to a container-internal bridge on a
// gateway service. Used to set PodmanDNS for ipamDriver:none LAN networks so
// clients receive a static resolv.conf pointing to the gateway dnsmasq.
func gatewayLANBridgeDNS(doc manifest.Document) map[string]string {
	result := map[string]string{}
	for _, svc := range doc.Spec.Services {
		if svc.Type != "gateway" {
			continue
		}
		// bridge name → gateway IP (host part of the CIDR, e.g. "10.0.10.1")
		bridgeGW := map[string]string{}
		for _, b := range svc.Bridges {
			if b.IPv4 == "" {
				continue
			}
			host, _, err := net.ParseCIDR(b.IPv4)
			if err != nil {
				host = net.ParseIP(b.IPv4)
			}
			if host != nil {
				bridgeGW[b.Name] = host.String()
			}
		}
		for _, iface := range svc.Interfaces {
			if iface.Bridge == "" {
				continue
			}
			if gw, ok := bridgeGW[iface.Bridge]; ok {
				result[iface.Role] = gw
			}
		}
	}
	return result
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

func resolveService(deployment string, svc manifest.Service, prev int, netByRole map[string]plan.Network) plan.Service {
	replicas := svc.Replicas
	out := plan.Service{
		Name:                 svc.Name,
		Type:                 svc.Type,
		Replicas:             replicas,
		Image:                svc.Image,
		DependsOn:            append([]string(nil), svc.DependsOn...),
		Bridges:              append([]manifest.BridgeSpec(nil), svc.Bridges...),
		Ports:                append([]string(nil), svc.Ports...),
		Volumes:              append([]string(nil), svc.Volumes...),
		Config:               svc.Config,
		PreviousReplicaCount: prev,
		Delta:                computeReplicaDelta(prev, replicas),
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
	containerName := plan.ContainerName(deployment, svc.Name, index)
	inst := plan.Instance{
		Index:         index,
		InstanceName:  plan.InstanceName(svc.Name, index),
		ContainerName: containerName,
	}
	for pos, iface := range svc.Interfaces {
		net := netByRole[iface.Role]

		device := iface.Device
		if device == "" {
			device = fmt.Sprintf("eth%d", pos)
		}

		// Always derive the MAC from the 0-based index so the key is stable
		// regardless of replica count. Explicit MACs are only honoured for
		// single-replica services; multi-replica services always derive.
		mac := iface.MAC
		if replicas > 1 {
			// Explicit MAC/address are ambiguous across replicas; derive per index.
			mac = ""
		}
		if mac == "" {
			mac = plan.CanonicalMAC(deployment, svc.Name, iface.Role, index)
		}

		ipv4, ipv6 := "", ""
		if replicas == 1 {
			ipv4 = iface.IPv4
			ipv6 = iface.IPv6
		}

		addressing := iface.Addressing
		if addressing == "" {
			addressing = manifest.AddressingDHCP
		}

		resolved := plan.Interface{
			Role:           iface.Role,
			Network:        net.Bridge,
			Device:         device,
			Bridge:         iface.Bridge,
			MAC:            mac,
			IPv4:           ipv4,
			IPv6:           ipv6,
			DefaultRoute:   iface.DefaultRoute,
			Addressing:     addressing,
			ManagedNetwork: net.IPAMDriver != "none",
		}
		if net.IPv4 != nil {
			resolved.Gateway4 = net.IPv4.Gateway
		}
		if net.IPv6 != nil {
			resolved.Gateway6 = net.IPv6.Gateway
		}
		inst.Interfaces = append(inst.Interfaces, resolved)
	}
	for _, radio := range svc.Radios {
		addressing := radio.Addressing
		if radio.Mode == manifest.RadioModeStation && addressing == "" {
			addressing = manifest.AddressingDHCP
		}
		resolvedRadio := plan.Radio{
			Name:          radio.Name,
			Medium:        radio.Medium,
			Network:       radio.Network,
			Device:        radio.Device,
			Mode:          radio.Mode,
			Mesh:          radio.Mesh,
			Addressing:    addressing,
			DefaultRoute:  radio.DefaultRoute,
			ManagerName:   plan.RadioManagerName(deployment, svc.Name, index, radio.Name),
			MAC:           plan.CanonicalRadioMAC(deployment, svc.Name, index, radio.Name),
			ContainerName: containerName,
		}
		if radio.Mesh != nil {
			resolvedRadio.Bridge = radio.Mesh.Bridge
		}
		if radio.Roaming != nil {
			resolvedRadio.RoamingMedia = append([]string(nil), radio.Roaming.Media...)
		}
		for _, vap := range radio.VAPs {
			device := plan.VAPDeviceName(deployment, svc.Name, index, radio.Name, vap.Slot)
			if vap.Slot == 0 {
				device = radio.Device
			}
			resolvedRadio.VAPs = append(resolvedRadio.VAPs, plan.VAP{
				Slot: vap.Slot, Network: vap.Network, Bridge: vap.Bridge,
				Device: device, MAC: plan.CanonicalVAPBSSID(deployment, svc.Name, index, radio.Name, vap.Slot),
			})
		}
		sort.Slice(resolvedRadio.VAPs, func(left, right int) bool {
			return resolvedRadio.VAPs[left].Slot < resolvedRadio.VAPs[right].Slot
		})
		inst.Radios = append(inst.Radios, resolvedRadio)
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
