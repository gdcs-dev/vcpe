// Package genericcontainer implements the catch-all service type. It renders a
// generated compose file plus an environment file from a small typed config and
// the resolved interface identities. It absorbs the former bespoke "client"
// service, which is now just a generic container.
package genericcontainer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for generic containers.
const TypeName = "generic-container"

// Config is the typed configuration for a generic container.
type Config struct {
	Command []string          `yaml:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Ports   []string          `yaml:"ports,omitempty"`
	Volumes []string          `yaml:"volumes,omitempty"`
}

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

func (serviceType) ValidateConfig(node yaml.Node) error {
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer { return renderer{} }

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement { return nil }

func (serviceType) DefaultImagePolicy() string { return "build" }

type renderer struct{}

func (renderer) Name() string { return "generic-container-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	var cfg Config
	if err := typeregistry.StrictDecode(input.Service.Config, &cfg); err != nil {
		return render.Result{}, fmt.Errorf("generic-container %q: %w", input.Service.Name, err)
	}
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("generic-container %q has no instances", input.Service.Name)
	}

	// Determine which DNS servers to inject for LAN-connected services.
	// Podman's aardvark-dns always writes the network bridge IP (.254) into
	// the container's /etc/resolv.conf. For LAN networks, the gateway's
	// brlan0 dnsmasq (at .1) is the correct resolver — it forwards to BNG
	// which knows about all container hostnames. We collect unique PodmanDNS
	// values from every network role this service connects to.
	roleDNS := map[string]string{}
	for _, n := range input.Deployment.Networks {
		if n.PodmanDNS != "" {
			roleDNS[n.Role] = n.PodmanDNS
		}
	}
	dnsSet := map[string]struct{}{}
	var lanDNS []string
	for _, iface := range input.Service.Instances[0].Interfaces {
		if ip, ok := roleDNS[iface.Role]; ok {
			if _, seen := dnsSet[ip]; !seen {
				dnsSet[ip] = struct{}{}
				lanDNS = append(lanDNS, ip)
			}
		}
	}

	env := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])
	extra := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		extra = append(extra, k+"="+v)
	}
	sort.Strings(extra)
	env = append(env, extra...)

	composeYAML, err := generateCompose(input, cfg, lanDNS)
	if err != nil {
		return render.Result{}, err
	}

	artifacts := []render.Artifact{
		{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		{Key: "compose.yaml", Content: composeYAML},
	}

	// When any interface connects to a gateway LAN network, emit a static
	// resolv.conf and mount it over /etc/resolv.conf. This overrides
	// Podman's aardvark-dns injection (which uses the LAN bridge .254) with
	// the gateway brlan0 IP (.1). udhcpc's mv-based resolv.conf update will
	// fail silently (bind-mount is read-only) leaving our nameserver intact.
	if len(lanDNS) > 0 {
		var rb strings.Builder
		rb.WriteString("search dns.podman\n")
		for _, ns := range lanDNS {
			fmt.Fprintf(&rb, "nameserver %s\n", ns)
		}
		artifacts = append(artifacts, render.Artifact{Key: "resolv.conf", Content: rb.String()})
	}

	return render.Result{
		Renderer:  "generic-container-renderer",
		Artifacts: artifacts,
	}, nil
}

func generateCompose(input render.Input, cfg Config, lanDNS []string) (string, error) {
	inst := input.Service.Instances[0]
	replicas := input.Service.Replicas

	// Build the top-level external network declarations from the first
	// instance's interface list (network names are the same across replicas).
	topNetworks := map[string]any{}
	for _, iface := range inst.Interfaces {
		key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
		topNetworks[iface.Role] = map[string]any{
			"external": true,
			"name":     "${IFACE_" + key + "_NETWORK}",
		}
	}

	// buildSvcEntry constructs a single compose service map.
	// pinMAC controls whether mac_address is included in the network
	// attachment; single-replica services pin the IPAM MAC, multi-replica
	// services let Podman assign a unique random MAC to each container.
	buildSvcEntry := func(pinMAC bool) map[string]any {
		svcNetworks := map[string]any{}
		for _, iface := range inst.Interfaces {
			key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
			netEntry := map[string]any{}
			if pinMAC {
				netEntry["mac_address"] = "${IFACE_" + key + "_MAC}"
			}
			svcNetworks[iface.Role] = netEntry
		}
		svc := map[string]any{
			"image":    render.ImageRef(input.Service.Image),
			"env_file": []string{"compose.env"},
			"restart":  "unless-stopped",
			"cap_add":  []string{"NET_ADMIN", "NET_RAW"},
		}
		if len(svcNetworks) > 0 {
			svc["networks"] = svcNetworks
		}
		// For services connected to gateway LAN networks, mount a static
		// resolv.conf that points to the gateway's brlan0 dnsmasq (.1).
		// Podman's aardvark-dns always injects .254 as the nameserver;
		// the explicit volume mount overrides that, and udhcpc's mv-based
		// update will fail silently against the read-only bind mount —
		// leaving our nameserver in place.
		volumes := append([]string(nil), cfg.Volumes...)
		if len(lanDNS) > 0 {
			volumes = append(volumes, "./resolv.conf:/etc/resolv.conf:ro")
		}
		if len(volumes) > 0 {
			svc["volumes"] = volumes
		}
		if len(cfg.Command) > 0 {
			svc["command"] = cfg.Command
		}
		if len(cfg.Ports) > 0 {
			svc["ports"] = cfg.Ports
		}
		if len(cfg.Env) > 0 {
			envMap := map[string]string{}
			for k, v := range cfg.Env {
				envMap[k] = v
			}
			svc["environment"] = envMap
		}
		return svc
	}

	services := map[string]any{}
	if replicas <= 1 {
		services[input.Service.Name] = buildSvcEntry(true)
	} else {
		// One named service entry per replica so podman-compose starts each
		// as an independent container with a Podman-assigned unique MAC.
		for i := 0; i < replicas; i++ {
			services[fmt.Sprintf("%s-%d", input.Service.Name, i+1)] = buildSvcEntry(false)
		}
	}

	doc := map[string]any{"services": services}
	if len(topNetworks) > 0 {
		doc["networks"] = topNetworks
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal generated compose: %w", err)
	}
	return string(out), nil
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
