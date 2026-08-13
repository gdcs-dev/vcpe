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
	return svc, externalNetworks
}

// AttachProxySidecar adds a health-transport proxy sidecar reachable by
// service name over the shared external health network, and wires the
// workload's own svc/svcNets to depend on and reach it. It is a no-op when
// healthPort is 0.
func AttachProxySidecar(input render.Input, instanceName string, healthPort int, services, topNets, svcNets, svc map[string]any) {
	if healthPort == 0 {
		return
	}
	healthNetworkName := input.Deployment.Name + "-00-health"
	topNets["aa-health"] = map[string]any{"external": true, "name": healthNetworkName}
	svcNets["aa-health"] = map[string]any{}
	sidecarName := instanceName + "-health"
	svc["depends_on"] = []string{sidecarName}
	services[sidecarName] = map[string]any{
		"image":          render.ImageRef(input.Service.Image),
		"container_name": input.Deployment.Name + "-" + instanceName + "-health",
		"entrypoint":     []string{"/usr/local/bin/vcpe-healthd"},
		// The upstream's own checks run sequentially and can each take up to
		// its own probe timeout, so the proxy waits longer than any single
		// check to avoid timing out on a slow-but-valid response.
		"command":  []string{"--proxy-url", "http://" + instanceName + ":9878/health", "--timeout", "10s"},
		"networks": map[string]any{"aa-health": map[string]any{}},
		"ports":    []string{fmt.Sprintf("127.0.0.1:%d:9878", healthPort)},
		"restart":  "unless-stopped",
	}
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
