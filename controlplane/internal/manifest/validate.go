package manifest

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"
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
	secrets, err := validateSecrets(doc)
	if err != nil {
		return err
	}
	wirelessNetworks, err := validateWirelessNetworks(doc, secrets)
	if err != nil {
		return err
	}
	if err := validateWirelessMedia(doc.Spec.WirelessMedia); err != nil {
		return err
	}
	if err := validateServices(doc, networks, wirelessNetworks, secrets); err != nil {
		return err
	}
	if err := validateWirelessScenarios(doc); err != nil {
		return err
	}

	return nil
}

type scenarioAPKey struct {
	service string
	replica int
	radio   string
	slot    int
}

func validateWirelessScenarios(doc Document) error {
	services := make(map[string]Service, len(doc.Spec.Services))
	for _, service := range doc.Spec.Services {
		services[service.Name] = service
	}
	names := map[string]bool{}
	for _, scenario := range doc.Spec.WirelessScenarios {
		if scenario.Name == "" {
			return fmt.Errorf("wireless scenario name is required")
		}
		if names[scenario.Name] {
			return fmt.Errorf("duplicate wireless scenario %q", scenario.Name)
		}
		names[scenario.Name] = true
		stationService, exists := services[scenario.Station.Service]
		if !exists || scenario.Station.Replica < 1 || scenario.Station.Replica > stationService.Replicas {
			return fmt.Errorf("wireless scenario %q station service %q replica %d is not declared", scenario.Name, scenario.Station.Service, scenario.Station.Replica)
		}
		var station *Radio
		for index := range stationService.Radios {
			if stationService.Radios[index].Name == scenario.Station.Radio {
				station = &stationService.Radios[index]
				break
			}
		}
		if station == nil || station.Mode != RadioModeStation || station.Roaming == nil {
			return fmt.Errorf("wireless scenario %q station %q must be a declared roaming station", scenario.Name, scenario.Station.Radio)
		}
		if len(scenario.APs) < 2 {
			return fmt.Errorf("wireless scenario %q requires at least two AP references", scenario.Name)
		}
		aps := map[scenarioAPKey]bool{}
		for _, ap := range scenario.APs {
			key, err := resolveScenarioAP(ap, *station, services)
			if err != nil {
				return fmt.Errorf("wireless scenario %q AP: %w", scenario.Name, err)
			}
			if aps[key] {
				return fmt.Errorf("wireless scenario %q has duplicate AP %q slot %d", scenario.Name, ap.Radio, key.slot)
			}
			aps[key] = true
		}
		initial, err := resolveScenarioAP(scenario.Assertions.InitialAP, *station, services)
		if err != nil || !aps[initial] {
			return fmt.Errorf("wireless scenario %q initialAP is not an eligible declared AP", scenario.Name)
		}
		final, err := resolveScenarioAP(scenario.Assertions.FinalAP, *station, services)
		if err != nil || !aps[final] {
			return fmt.Errorf("wireless scenario %q finalAP is not an eligible declared AP", scenario.Name)
		}
		if initial == final {
			return fmt.Errorf("wireless scenario %q initialAP and finalAP must be different", scenario.Name)
		}
		if !scenario.Assertions.SameIPv4 {
			return fmt.Errorf("wireless scenario %q assertions.sameIPv4 must be true", scenario.Name)
		}
		if scenario.Assertions.MaxRoamMs < 1 || scenario.Assertions.MaxRoamMs > 120000 {
			return fmt.Errorf("wireless scenario %q assertions.maxRoamMs must be 1 through 120000", scenario.Name)
		}
		if scenario.Assertions.MaxGapMs < 1 || scenario.Assertions.MaxGapMs > 120000 {
			return fmt.Errorf("wireless scenario %q assertions.maxGapMs must be 1 through 120000", scenario.Name)
		}
		if len(scenario.Steps) < 2 {
			return fmt.Errorf("wireless scenario %q requires at least two SNR steps", scenario.Name)
		}
		seenUpdates := map[struct {
			ap   scenarioAPKey
			atMs int
		}]bool{}
		previous := -1
		for _, step := range scenario.Steps {
			if step.AtMs == nil || *step.AtMs < 0 || *step.AtMs > 120000 {
				return fmt.Errorf("wireless scenario %q step.atMs must be 0 through 120000", scenario.Name)
			}
			if *step.AtMs < previous {
				return fmt.Errorf("wireless scenario %q steps are out of order", scenario.Name)
			}
			previous = *step.AtMs
			if step.SNRDb == nil || *step.SNRDb < -100 || *step.SNRDb > 100 {
				return fmt.Errorf("wireless scenario %q step.snrDb must be -100 through 100", scenario.Name)
			}
			key, err := resolveScenarioAP(step.AP, *station, services)
			if err != nil || !aps[key] {
				return fmt.Errorf("wireless scenario %q step AP is not an eligible declared AP", scenario.Name)
			}
			update := struct {
				ap   scenarioAPKey
				atMs int
			}{key, *step.AtMs}
			if seenUpdates[update] {
				return fmt.Errorf("wireless scenario %q has duplicate link update at %d ms", scenario.Name, *step.AtMs)
			}
			seenUpdates[update] = true
		}
	}
	return nil
}

func resolveScenarioAP(ref VAPReference, station Radio, services map[string]Service) (scenarioAPKey, error) {
	if ref.Slot == nil || *ref.Slot < 0 || *ref.Slot > 7 {
		return scenarioAPKey{}, fmt.Errorf("AP %q requires a valid VAP slot", ref.Radio)
	}
	service, exists := services[ref.Service]
	if !exists || ref.Replica < 1 || ref.Replica > service.Replicas {
		return scenarioAPKey{}, fmt.Errorf("AP service %q replica %d is not declared", ref.Service, ref.Replica)
	}
	for _, radio := range service.Radios {
		if radio.Name != ref.Radio || radio.Mode != RadioModeAP {
			continue
		}
		for _, medium := range station.Roaming.Media {
			if medium != radio.Medium {
				continue
			}
			for _, vap := range radio.VAPs {
				if vap.Slot == *ref.Slot && vap.Network == station.Network {
					return scenarioAPKey{ref.Service, ref.Replica, ref.Radio, *ref.Slot}, nil
				}
			}
		}
	}
	return scenarioAPKey{}, fmt.Errorf("AP %q slot %d is not eligible for roaming station %q", ref.Radio, *ref.Slot, station.Name)
}

func validateWirelessMedia(media []WirelessMedium) error {
	names := make(map[string]struct{}, len(media))
	for _, medium := range media {
		if medium.Name == "" {
			return fmt.Errorf("spec.wirelessMedia[].name is required")
		}
		if _, exists := names[medium.Name]; exists {
			return fmt.Errorf("duplicate wireless medium name %q", medium.Name)
		}
		names[medium.Name] = struct{}{}
		if medium.WidthMHz != 20 {
			return fmt.Errorf("wireless medium %q has unsupported widthMHz %d: expected 20", medium.Name, medium.WidthMHz)
		}
		validChannel := false
		switch medium.Band {
		case "2.4ghz":
			validChannel = medium.Channel >= 1 && medium.Channel <= 14
		case "5ghz":
			for _, channel := range []int{36, 40, 44, 48, 149, 153, 157, 161, 165} {
				if medium.Channel == channel {
					validChannel = true
					break
				}
			}
		case "6ghz":
			validChannel = medium.Channel >= 5 && medium.Channel <= 229 && (medium.Channel-5)%16 == 0
		default:
			return fmt.Errorf("wireless medium %q has unsupported band %q", medium.Name, medium.Band)
		}
		if !validChannel {
			return fmt.Errorf("wireless medium %q has unsupported channel %d for band %q", medium.Name, medium.Channel, medium.Band)
		}
	}
	return nil
}

func validateWirelessNetworks(doc Document, secrets map[string]struct{}) (map[string]WirelessNetwork, error) {
	networks := make(map[string]WirelessNetwork, len(doc.Spec.WirelessNetworks))
	for _, network := range doc.Spec.WirelessNetworks {
		if network.Name == "" {
			return nil, fmt.Errorf("spec.wirelessNetworks[].name is required")
		}
		if _, exists := networks[network.Name]; exists {
			return nil, fmt.Errorf("duplicate wireless network name %q", network.Name)
		}
		if size := len([]byte(network.SSID)); size < 1 || size > 32 {
			return nil, fmt.Errorf("wireless network %q SSID must be 1 through 32 bytes", network.Name)
		}
		switch network.Security {
		case WirelessOpen:
			if network.PassphraseSecretRef != "" {
				return nil, fmt.Errorf("wireless network %q with security %q cannot declare passphraseSecretRef", network.Name, network.Security)
			}
		case WirelessWPA2Personal, WirelessWPA3Personal:
			if network.PassphraseSecretRef == "" {
				return nil, fmt.Errorf("wireless network %q with security %q requires passphraseSecretRef", network.Name, network.Security)
			}
			if _, exists := secrets[network.PassphraseSecretRef]; !exists {
				return nil, fmt.Errorf("wireless network %q references unknown passphrase secret %q", network.Name, network.PassphraseSecretRef)
			}
		default:
			return nil, fmt.Errorf("wireless network %q has unsupported security %q: expected %q, %q, or %q", network.Name, network.Security, WirelessOpen, WirelessWPA2Personal, WirelessWPA3Personal)
		}
		networks[network.Name] = network
	}
	return networks, nil
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

func validateServices(doc Document, networks map[string][]netip.Prefix, wirelessNetworks map[string]WirelessNetwork, secrets map[string]struct{}) error {
	names := map[string]struct{}{}
	apCounts := map[string]int{}
	stationCounts := map[string]int{}
	var roamingStations []struct {
		service string
		radio   Radio
	}
	meshPeers := map[string][]string{}
	meshContracts := map[string]Radio{}
	wirelessMedia := make(map[string]WirelessMedium, len(doc.Spec.WirelessMedia))
	for _, medium := range doc.Spec.WirelessMedia {
		wirelessMedia[medium.Name] = medium
	}
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
		if err := validateServiceAttachments(svc, networks, wirelessMedia, wirelessNetworks, secrets, apCounts, stationCounts); err != nil {
			return err
		}
		for _, radio := range svc.Radios {
			if radio.Mode == RadioModeStation && radio.Roaming != nil {
				roamingStations = append(roamingStations, struct {
					service string
					radio   Radio
				}{svc.Name, radio})
			}
			if radio.Mode != RadioModeMesh {
				continue
			}
			if peer, exists := meshContracts[radio.Mesh.ID]; exists && (peer.Medium != radio.Medium || peer.Mesh.SAESecretRef != radio.Mesh.SAESecretRef) {
				return fmt.Errorf("mesh %q has inconsistent medium or SAE secret between %q and %q", radio.Mesh.ID, meshPeers[radio.Mesh.ID][0], svc.Name)
			}
			meshContracts[radio.Mesh.ID] = radio
			meshPeers[radio.Mesh.ID] = append(meshPeers[radio.Mesh.ID], svc.Name)
		}
	}
	roamingPairs := map[string]bool{}
	for _, station := range roamingStations {
		candidates := 0
		for _, medium := range station.radio.Roaming.Media {
			pair := medium + "\x00" + station.radio.Network
			roamingPairs[pair] = true
			candidates += apCounts[pair]
		}
		if candidates < 2 {
			return fmt.Errorf("service %q station radio %q requires at least two eligible AP VAPs for roaming network %q, got %d", station.service, station.radio.Name, station.radio.Network, candidates)
		}
	}
	for name, count := range apCounts {
		if count > 1 && (!roamingPairs[name] || stationCounts[name] != 0) {
			return fmt.Errorf("wireless network %q has %d AP radios: at most one is supported", name, count)
		}
	}
	for name, count := range stationCounts {
		if count > 0 && apCounts[name] != 1 {
			return fmt.Errorf("wireless network %q has %d station radios but requires exactly one AP", name, count)
		}
	}
	for id, peers := range meshPeers {
		if len(peers) < 2 {
			return fmt.Errorf("mesh %q on service %q has no matching peer", id, peers[0])
		}
	}
	if len(meshPeers) > 0 {
		if err := validateMeshTopology(doc); err != nil {
			return err
		}
	}
	return validateDependsOn(doc)
}

func validateMeshTopology(doc Document) error {
	gateways, owners := 0, 0
	for _, svc := range doc.Spec.Services {
		if svc.Type != "gateway" {
			continue
		}
		gateways++
		var meshRadio *Radio
		for index := range svc.Radios {
			if svc.Radios[index].Mode == RadioModeMesh {
				if meshRadio != nil {
					return fmt.Errorf("mesh Gateway %q must declare exactly one mesh radio", svc.Name)
				}
				meshRadio = &svc.Radios[index]
			}
		}
		if meshRadio == nil {
			return fmt.Errorf("mesh topology requires exactly two Gateway nodes with mesh radios; %q has none", svc.Name)
		}
		if svc.Replicas != 1 {
			return fmt.Errorf("mesh Gateway %q must be single-replica", svc.Name)
		}
		accessAP := false
		for _, radio := range svc.Radios {
			if radio.Mode == RadioModeAP {
				for _, vap := range radio.VAPs {
					if vap.Bridge == meshRadio.Mesh.Bridge {
						accessAP = true
					}
				}
			}
		}
		if !accessAP {
			return fmt.Errorf("mesh Gateway %q requires a dedicated access AP on bridge %q", svc.Name, meshRadio.Mesh.Bridge)
		}
		for _, iface := range svc.Interfaces {
			if iface.Bridge == meshRadio.Mesh.Bridge {
				return fmt.Errorf("mesh Gateway %q cannot attach Podman interface role %q to client LAN bridge %q", svc.Name, iface.Role, iface.Bridge)
			}
		}
		for _, bridge := range svc.Bridges {
			if bridge.Name != meshRadio.Mesh.Bridge {
				continue
			}
			if bridge.IPv4 != "" || bridge.IPv6 != "" || bridge.DHCPStart != "" || bridge.DHCPEnd != "" {
				owners++
				if bridge.IPv4 == "" || bridge.DHCPStart == "" || bridge.DHCPEnd == "" {
					return fmt.Errorf("mesh Gateway %q root bridge %q requires IPv4 and both DHCP bounds", svc.Name, bridge.Name)
				}
			}
		}
	}
	if gateways != 2 {
		return fmt.Errorf("mesh topology requires exactly two Gateway nodes, got %d", gateways)
	}
	if owners != 1 {
		return fmt.Errorf("mesh topology requires exactly one LAN IP and DHCP owner, got %d", owners)
	}
	return nil
}

func validateServiceAttachments(svc Service, networks map[string][]netip.Prefix, wirelessMedia map[string]WirelessMedium, wirelessNetworks map[string]WirelessNetwork, secrets map[string]struct{}, apCounts, stationCounts map[string]int) error {
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
		if err := validateAddressing(svc.Name, iface); err != nil {
			return err
		}
		if iface.DefaultRoute {
			defaultRoutes++
		}
	}

	radioNames := map[string]struct{}{}
	deviceNames := map[string]struct{}{}
	bridgeNames := make(map[string]struct{}, len(svc.Bridges))
	for _, bridge := range svc.Bridges {
		bridgeNames[bridge.Name] = struct{}{}
	}
	for _, radio := range svc.Radios {
		if radio.Name == "" {
			return fmt.Errorf("service %q has a radio with no name", svc.Name)
		}
		if _, exists := radioNames[radio.Name]; exists {
			return fmt.Errorf("service %q has duplicate radio name %q", svc.Name, radio.Name)
		}
		radioNames[radio.Name] = struct{}{}
		if !validLinuxInterfaceName(radio.Device) {
			return fmt.Errorf("service %q radio %q has invalid Linux device name %q", svc.Name, radio.Name, radio.Device)
		}
		if _, exists := deviceNames[radio.Device]; exists {
			return fmt.Errorf("service %q has duplicate radio device %q", svc.Name, radio.Device)
		}
		deviceNames[radio.Device] = struct{}{}
		if err := validateMediaRadio(svc, radio, bridgeNames, wirelessMedia, wirelessNetworks, secrets, apCounts, stationCounts); err != nil {
			return err
		}
		if radio.DefaultRoute {
			defaultRoutes++
		}
	}
	if defaultRoutes > 1 {
		return fmt.Errorf("service %q declares %d default routes: at most one interface or station radio may set defaultRoute", svc.Name, defaultRoutes)
	}
	return nil
}

func validateMediaRadio(svc Service, radio Radio, bridges map[string]struct{}, media map[string]WirelessMedium, profiles map[string]WirelessNetwork, secrets map[string]struct{}, apCounts, stationCounts map[string]int) error {
	service := svc.Name
	medium, exists := media[radio.Medium]
	if !exists && radio.Roaming == nil {
		return fmt.Errorf("service %q radio %q references unknown wireless medium %q", service, radio.Name, radio.Medium)
	}
	switch radio.Mode {
	case RadioModeAP:
		if radio.Network != "" || radio.Addressing != "" || radio.DefaultRoute || radio.Mesh != nil || radio.Roaming != nil {
			return fmt.Errorf("service %q AP radio %q must declare profiles and bridges only in vaps", service, radio.Name)
		}
		if len(radio.VAPs) == 0 || len(radio.VAPs) > 8 {
			return fmt.Errorf("service %q AP radio %q must declare 1 through 8 vaps", service, radio.Name)
		}
		slots := map[int]struct{}{}
		attachedProfiles := map[string]struct{}{}
		for _, vap := range radio.VAPs {
			if vap.Slot < 0 || vap.Slot > 7 {
				return fmt.Errorf("service %q AP radio %q has invalid VAP slot %d", service, radio.Name, vap.Slot)
			}
			if _, exists := slots[vap.Slot]; exists {
				return fmt.Errorf("service %q AP radio %q has duplicate VAP slot %d", service, radio.Name, vap.Slot)
			}
			slots[vap.Slot] = struct{}{}
			profile, exists := profiles[vap.Network]
			if !exists {
				return fmt.Errorf("service %q AP radio %q VAP slot %d references unknown wireless network %q", service, radio.Name, vap.Slot, vap.Network)
			}
			if _, exists := attachedProfiles[vap.Network]; exists {
				return fmt.Errorf("service %q AP radio %q repeats wireless network %q", service, radio.Name, vap.Network)
			}
			attachedProfiles[vap.Network] = struct{}{}
			if _, exists := bridges[vap.Bridge]; !exists {
				return fmt.Errorf("service %q AP radio %q VAP slot %d references undeclared bridge %q", service, radio.Name, vap.Slot, vap.Bridge)
			}
			if medium.Band == "6ghz" && profile.Security != WirelessWPA3Personal {
				return fmt.Errorf("service %q AP radio %q VAP slot %d requires wpa3-personal on 6ghz", service, radio.Name, vap.Slot)
			}
			apCounts[radio.Medium+"\x00"+vap.Network]++
		}
		if _, exists := slots[0]; !exists {
			return fmt.Errorf("service %q AP radio %q requires VAP slot 0", service, radio.Name)
		}
	case RadioModeStation:
		if len(radio.VAPs) != 0 || radio.Mesh != nil {
			return fmt.Errorf("service %q station radio %q cannot declare vaps", service, radio.Name)
		}
		profile, exists := profiles[radio.Network]
		if !exists {
			return fmt.Errorf("service %q station radio %q references unknown wireless network %q", service, radio.Name, radio.Network)
		}
		if radio.Addressing != "" && radio.Addressing != AddressingDHCP {
			return fmt.Errorf("service %q station radio %q must use dhcp addressing", service, radio.Name)
		}
		if radio.Roaming != nil {
			if radio.Medium != "" {
				return fmt.Errorf("service %q station radio %q cannot combine fixed medium with roaming.media", service, radio.Name)
			}
			if len(radio.Roaming.Media) == 0 {
				return fmt.Errorf("service %q station radio %q requires roaming.media", service, radio.Name)
			}
			selected := map[string]bool{}
			for _, name := range radio.Roaming.Media {
				if selected[name] {
					return fmt.Errorf("service %q station radio %q has duplicate roaming medium %q", service, radio.Name, name)
				}
				selected[name] = true
				if _, exists := media[name]; !exists {
					return fmt.Errorf("service %q station radio %q references unknown wireless medium %q", service, radio.Name, name)
				}
				if media[name].Band == "6ghz" && profile.Security != WirelessWPA3Personal {
					return fmt.Errorf("service %q station radio %q requires wpa3-personal on 6ghz", service, radio.Name)
				}
			}
			return nil
		}
		if medium.Band == "6ghz" && profile.Security != WirelessWPA3Personal {
			return fmt.Errorf("service %q station radio %q requires wpa3-personal on 6ghz", service, radio.Name)
		}
		stationCounts[radio.Medium+"\x00"+radio.Network]++
	case RadioModeMesh:
		if svc.Type != "gateway" {
			return fmt.Errorf("service %q mesh radio %q requires a gateway", service, radio.Name)
		}
		if radio.Mesh == nil {
			return fmt.Errorf("service %q mesh radio %q requires mesh id, bridge, and saeSecretRef", service, radio.Name)
		}
		if radio.Network != "" || len(radio.VAPs) != 0 || radio.Addressing != "" || radio.DefaultRoute || radio.Roaming != nil {
			return fmt.Errorf("service %q mesh radio %q cannot declare network, vaps, addressing, or defaultRoute", service, radio.Name)
		}
		if len(radio.Mesh.ID) < 1 || len(radio.Mesh.ID) > 32 {
			return fmt.Errorf("service %q mesh radio %q id must be 1 through 32 bytes", service, radio.Name)
		}
		for _, character := range []byte(radio.Mesh.ID) {
			if character < 0x20 || character == 0x7f {
				return fmt.Errorf("service %q mesh radio %q id must be printable", service, radio.Name)
			}
		}
		if _, exists := bridges[radio.Mesh.Bridge]; !exists {
			return fmt.Errorf("service %q mesh radio %q references undeclared bridge %q", service, radio.Name, radio.Mesh.Bridge)
		}
		if _, exists := secrets[radio.Mesh.SAESecretRef]; !exists || radio.Mesh.SAESecretRef == "" {
			return fmt.Errorf("service %q mesh radio %q references unknown SAE secret %q", service, radio.Name, radio.Mesh.SAESecretRef)
		}
	default:
		return fmt.Errorf("service %q radio %q has invalid mode %q", service, radio.Name, radio.Mode)
	}
	return nil
}

func validLinuxInterfaceName(name string) bool {
	return name != "" && name != "." && name != ".." && utf8.ValidString(name) && len([]byte(name)) <= 15 && !strings.ContainsAny(name, "/:\x00")
}

// validateAddressing enforces the addressing/address consistency rule: static
// requires an explicit ipv4/ipv6, dhcp (explicit or defaulted) forbids one.
// Bridge-enslaved interfaces are exempt since the bridge, not the member port,
// carries an address. There is deliberately no check against a service's own
// config (e.g. a DHCP-server role misconfigured as dhcp) — see design.md.
func validateAddressing(service string, iface Interface) error {
	if iface.Bridge != "" {
		return nil
	}
	addressing := iface.Addressing
	if addressing == "" {
		addressing = AddressingDHCP
	}
	switch addressing {
	case AddressingDHCP:
		if iface.IPv4 != "" || iface.IPv6 != "" {
			return fmt.Errorf("service %q interface role %q declares an ipv4/ipv6 address but addressing is %q: set addressing: %s or remove the address", service, iface.Role, AddressingDHCP, AddressingStatic)
		}
	case AddressingStatic:
		if iface.IPv4 == "" && iface.IPv6 == "" {
			return fmt.Errorf("service %q interface role %q sets addressing: %s but declares no ipv4/ipv6 address", service, iface.Role, AddressingStatic)
		}
	default:
		return fmt.Errorf("service %q interface role %q has invalid addressing %q: must be %q or %q", service, iface.Role, iface.Addressing, AddressingDHCP, AddressingStatic)
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

func validateSecrets(doc Document) (map[string]struct{}, error) {
	names := make(map[string]struct{}, len(doc.Spec.Secrets))
	for _, s := range doc.Spec.Secrets {
		if s.Name == "" || s.Provider == "" {
			return nil, fmt.Errorf("spec.secrets[] entries require name and provider")
		}
		if _, exists := names[s.Name]; exists {
			return nil, fmt.Errorf("duplicate secret name %q", s.Name)
		}
		names[s.Name] = struct{}{}
		switch s.Provider {
		case "env", "file":
			if s.Key == "" || s.Value != "" {
				return nil, fmt.Errorf("secret %q with provider %q requires key and forbids value", s.Name, s.Provider)
			}
		case "literal":
			if doc.Metadata.Labels["environment"] != "development" {
				return nil, fmt.Errorf("secret %q with provider %q requires metadata.labels.environment=development", s.Name, s.Provider)
			}
			if s.Value == "" || s.Key != "" {
				return nil, fmt.Errorf("secret %q with provider %q requires value and forbids key", s.Name, s.Provider)
			}
		default:
			return nil, fmt.Errorf("unsupported secret provider %q: expected env, file, or development-only literal", s.Provider)
		}
	}
	return names, nil
}
