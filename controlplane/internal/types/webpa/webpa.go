// Package webpa implements the WebPA service type. WebPA has no typed config;
// it generates a compose.yaml and compose.env from the resolved interfaces so
// it can connect to any network role, not just the hardcoded mgmt network.
package webpa

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for WebPA.
const TypeName = "webpa"

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

// Config is the optional configuration for a webpa service.
type Config struct {
	Env map[string]string `yaml:"env,omitempty"`
}

// ValidateConfig accepts an optional env map; all other config is rejected.
func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind == 0 {
		return nil
	}
	var cfg Config
	return typeregistry.StrictDecode(node, &cfg)
}

func (serviceType) Renderer() render.Renderer { return renderer{} }

func (serviceType) ExpectedRoles() []typeregistry.RoleRequirement {
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: false}}
}

func (serviceType) DefaultImagePolicy() string { return "build" }

func (serviceType) ValidateInterfaces(_ []manifest.Interface) error { return nil }

func (serviceType) Description() string {
	return "USP/WebPA device-management server"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/webpa" }

type renderer struct{}

func (renderer) Name() string { return "webpa-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("webpa %q has no instances", input.Service.Name)
	}
	inst := input.Service.Instances[0]
	env := render.IfaceEnv(input.Deployment, input.Service, inst)

	var cfg Config
	_ = typeregistry.StrictDecode(input.Service.Config, &cfg)
	if len(cfg.Env) > 0 {
		extra := make([]string, 0, len(cfg.Env))
		for k, v := range cfg.Env {
			extra = append(extra, k+"="+v)
		}
		sort.Strings(extra)
		env = append(env, extra...)
	}

	ipamNone := map[string]bool{}
	for _, n := range input.Deployment.Networks {
		if n.IPAMDriver == "none" {
			ipamNone[n.Role] = true
		}
	}
	composeYAML := renderWebPACompose(input.Service.Name, inst.Interfaces, ipamNone)

	return render.Result{
		Renderer: "webpa-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.yaml", Content: composeYAML},
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		},
	}, nil
}

// renderWebPACompose generates a compose.yaml that wires up every interface
// from the resolved instance, regardless of role name. This replaces the
// curated services/webpa/compose.yaml so WebPA can connect to any network.
func renderWebPACompose(svcName string, ifaces []plan.Interface, ipamNone map[string]bool) string {
	svcNets := map[string]any{}
	topNets := map[string]any{}
	for _, iface := range ifaces {
		key := strings.ToUpper(strings.ReplaceAll(iface.Role, "-", "_"))
		entry := map[string]any{
			"mac_address": "${IFACE_" + key + "_MAC}",
		}
		if !ipamNone[iface.Role] {
			entry["ipv4_address"] = "${IFACE_" + key + "_IPV4}"
		}
		svcNets[iface.Role] = entry
		topNets[iface.Role] = map[string]any{
			"external": true,
			"name":     "${IFACE_" + key + "_NETWORK}",
		}
	}
	doc := map[string]any{
		"services": map[string]any{
			svcName: map[string]any{
				"image":          "${IMAGE}",
				"container_name": "${DEPLOYMENT_NAME}-${SERVICE_NAME}",
				"hostname":       "${SERVICE_NAME}",
				"privileged":     true,
				"cap_add":        []string{"NET_ADMIN", "NET_RAW"},
				"env_file":       []string{"compose.env"},
				"networks":       svcNets,
			},
		},
		"networks": topNets,
	}
	out, _ := yaml.Marshal(doc)
	return string(out)
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
