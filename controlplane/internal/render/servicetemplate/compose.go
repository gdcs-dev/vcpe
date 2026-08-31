package servicetemplate

import (
	"fmt"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
)

// NetworkAttachment builds one interface's Compose network-attachment entry.
// managed reports whether the interface's network is Podman-managed
// (ipamDriver != "none").
type NetworkAttachment func(iface plan.Interface, managed bool) map[string]any

// DefaultAttachment always pins mac_address and pins ipv4_address iff the
// interface's network is Podman-managed.
func DefaultAttachment(iface plan.Interface, managed bool) map[string]any {
	entry := map[string]any{"mac_address": iface.MAC}
	if managed {
		entry["ipv4_address"] = iface.IPv4
	}
	return entry
}

// MACOnlyAttachment always pins mac_address and never pins ipv4_address. Used
// by types whose entrypoint self-configures addressing (gateway, xb10).
func MACOnlyAttachment(iface plan.Interface, _ bool) map[string]any {
	return map[string]any{"mac_address": iface.MAC}
}

// isManaged reports whether role's network is Podman-managed, i.e. any
// ipamDriver other than "none". A role with no declared network is treated as
// managed, matching every built-in type's pre-existing behavior.
func isManaged(dep plan.Deployment, role string) bool {
	n := dep.Network(role)
	return n == nil || n.IPAMDriver != "none"
}

// BuildComposeService returns the standard per-instance Compose service block
// (image, container_name, hostname, env_file, networks) and this instance's
// external network declarations. attach builds each interface's
// network-attachment entry; callers add any remaining fields (ports,
// privileged, cap_add, volumes, command, ...) directly on the returned map.
func BuildComposeService(input render.Input, instance plan.Instance, attach NetworkAttachment) (svc map[string]any, externalNetworks map[string]any) {
	svcNets := map[string]any{}
	externalNetworks = map[string]any{}
	for _, iface := range instance.Interfaces {
		svcNets[iface.Role] = attach(iface, isManaged(input.Deployment, iface.Role))
		externalNetworks[iface.Role] = map[string]any{"external": true, "name": iface.Network}
	}
	instanceName := fmt.Sprintf("%s-%d", input.Service.Name, instance.Index+1)
	svc = map[string]any{
		"image":          render.ImageRef(input.Service.Image),
		"container_name": input.Deployment.Name + "-" + instanceName,
		"hostname":       instanceName,
		"env_file":       []string{fmt.Sprintf("instances/%d/compose.env", instance.Index+1)},
		"networks":       svcNets,
	}
	if dns := managementDNS(input, instance); len(dns) > 0 {
		svc["dns"] = dns
	}
	return svc, externalNetworks
}

func managementDNS(input render.Input, instance plan.Instance) []string {
	if input.Service.Type == "bng" {
		return nil
	}

	managementNetwork := ""
	aardvarkAddress := ""
	for _, iface := range instance.Interfaces {
		if iface.Role != "mgmt" {
			continue
		}
		network := input.Deployment.Network(iface.Role)
		if network == nil || network.IPAMDriver == "none" || network.IPv4 == nil || network.IPv4.Gateway == "" {
			return nil
		}
		managementNetwork = iface.Network
		aardvarkAddress = network.IPv4.Gateway
		break
	}
	if managementNetwork == "" {
		return nil
	}

	for _, service := range input.Deployment.Services {
		if service.Type != "bng" {
			continue
		}
		for _, bngInstance := range service.Instances {
			for _, iface := range bngInstance.Interfaces {
				if iface.Role == "mgmt" && iface.Network == managementNetwork && iface.IPv4 != "" {
					return []string{iface.IPv4, aardvarkAddress}
				}
			}
		}
	}
	return nil
}

// AttachHealthPublication publishes an instance's own standard health
// endpoint directly: it adds the reserved loopback mapping to the workload's
// ports and, only when none of the instance's topology interfaces already has
// a Podman-managed network, attaches the workload to the deployment's shared
// private `aa-health` network so Podman can still forward the host port. It
// creates no separate transport proxy service. It is a no-op when healthPort
// is 0.
func AttachHealthPublication(input render.Input, instance plan.Instance, healthPort int, topNets, svcNets, svc map[string]any) {
	if healthPort == 0 {
		return
	}
	ports, _ := svc["ports"].([]string)
	svc["ports"] = append(ports, fmt.Sprintf("127.0.0.1:%d:9878", healthPort))
	for _, iface := range instance.Interfaces {
		if isManaged(input.Deployment, iface.Role) {
			return
		}
	}
	healthNetworkName := input.Deployment.Name + "-00-health"
	topNets["aa-health"] = map[string]any{"external": true, "name": healthNetworkName}
	svcNets["aa-health"] = map[string]any{}
}

// AttachProbeSidecar adds a probe-delegation health sidecar that shares the
// workload instance's network namespace and runs a caller-supplied
// HTTP/command probe via vcpe-healthd.
func AttachProbeSidecar(services map[string]any, serviceName string, index int, image string, command []string) {
	instanceKey := fmt.Sprintf("%s-%d", serviceName, index+1)
	services[fmt.Sprintf("%s-health-%d", serviceName, index+1)] = map[string]any{
		"image":        image,
		"network_mode": "service:" + instanceKey,
		"depends_on":   []string{instanceKey},
		"restart":      "unless-stopped",
		"volumes":      []string{"./vcpe-healthd:/run/vcpe/vcpe-healthd:ro"},
		"entrypoint":   []string{"/run/vcpe/vcpe-healthd"},
		"command":      command,
	}
}
