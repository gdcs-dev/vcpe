package app

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/gdcs-dev/vcpe/controlplane/internal/ipam"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
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
		desired := ipam.PrimaryCIDR(n)
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
			reasons = append(reasons, disruptiveRadioChanges(prev, doc)...)
		}
	}
	persistedRadioReasons, err := disruptivePersistedRadioChanges(ps, doc)
	if err != nil {
		return false, nil, err
	}
	reasons = append(reasons, persistedRadioReasons...)

	sort.Strings(reasons)
	return len(reasons) > 0, reasons, nil
}

func disruptivePersistedRadioChanges(ps *persist.Store, desired manifest.Document) ([]string, error) {
	owned, err := ps.ListWirelessRadios(desired.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if len(owned) == 0 {
		return nil, nil
	}
	group, hasGroup, err := ps.WirelessGroup(desired.Metadata.Name)
	if err != nil {
		return nil, err
	}
	type desiredRadio struct {
		radio    manifest.Radio
		replicas int
	}
	desiredByKey := map[string]desiredRadio{}
	for _, service := range desired.Spec.Services {
		for _, radio := range service.Radios {
			desiredByKey[service.Name+"\x00"+radio.Name] = desiredRadio{radio: radio, replicas: service.Replicas}
		}
	}

	var reasons []string
	for _, radio := range owned {
		current, exists := desiredByKey[radio.Service+"\x00"+radio.LogicalName]
		if !exists || radio.Replica >= current.replicas {
			continue
		}
		identity := fmt.Sprintf("service %q radio %q replica %d", radio.Service, radio.LogicalName, radio.Replica)
		if radio.ManagerName != plan.RadioManagerName(desired.Metadata.Name, radio.Service, radio.Replica, radio.LogicalName) || radio.MAC != plan.CanonicalRadioMAC(desired.Metadata.Name, radio.Service, radio.Replica, radio.LogicalName) || radio.ContainerName != plan.ContainerName(desired.Metadata.Name, radio.Service, radio.Replica) {
			reasons = append(reasons, identity+" derived identity changes")
		}
		if !hasGroup || radio.GroupBit != group.Bit {
			reasons = append(reasons, identity+" hwsim group changes")
		}
		if (current.radio.Medium == "" && radio.Network != current.radio.Network) || radio.Device != current.radio.Device || radio.Mode != current.radio.Mode {
			reasons = append(reasons, identity+" immutable contract changes")
		}
	}
	return reasons, nil
}

func disruptiveRadioChanges(previous, desired manifest.Document) []string {
	type radioIdentity struct {
		service string
		radio   manifest.Radio
	}
	desiredRadios := map[string]radioIdentity{}
	for _, service := range desired.Spec.Services {
		for _, radio := range service.Radios {
			desiredRadios[service.Name+"\x00"+radio.Name] = radioIdentity{service: service.Name, radio: radio}
		}
	}

	var reasons []string
	mediaByName := map[string]manifest.WirelessMedium{}
	for _, medium := range desired.Spec.WirelessMedia {
		mediaByName[medium.Name] = medium
	}
	for _, old := range previous.Spec.WirelessMedia {
		if current, exists := mediaByName[old.Name]; !exists || current.Band != old.Band || current.Channel != old.Channel || current.WidthMHz != old.WidthMHz {
			reasons = append(reasons, fmt.Sprintf("wireless medium %q RF policy changes", old.Name))
		}
	}
	for _, service := range previous.Spec.Services {
		for _, previousRadio := range service.Radios {
			key := service.Name + "\x00" + previousRadio.Name
			current, exists := desiredRadios[key]
			if !exists {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q identity is removed", service.Name, previousRadio.Name))
				continue
			}
			if current.radio.Medium != previousRadio.Medium {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q medium changes from %q to %q", service.Name, previousRadio.Name, previousRadio.Medium, current.radio.Medium))
			}
			if previousRadio.Medium == "" && current.radio.Network != previousRadio.Network {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q wireless network changes from %q to %q", service.Name, previousRadio.Name, previousRadio.Network, current.radio.Network))
			}
			if current.radio.Device != previousRadio.Device {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q device changes from %q to %q", service.Name, previousRadio.Name, previousRadio.Device, current.radio.Device))
			}
			if current.radio.Mode != previousRadio.Mode {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q mode changes from %q to %q", service.Name, previousRadio.Name, previousRadio.Mode, current.radio.Mode))
			}
			if !reflect.DeepEqual(current.radio.Mesh, previousRadio.Mesh) {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q mesh contract changes", service.Name, previousRadio.Name))
			}
			if !reflect.DeepEqual(current.radio.Roaming, previousRadio.Roaming) {
				reasons = append(reasons, fmt.Sprintf("service %q radio %q roaming media changes", service.Name, previousRadio.Name))
			}
		}
	}
	return reasons
}
