// Package eventsink implements the event-sink service type. Like webpa, it has
// no typed config and renders only the interface environment consumed by the
// curated compose file at services/event-sink/compose.yaml.
package eventsink

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// TypeName is the manifest discriminator for the event-sink service.
const TypeName = "event-sink"

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

// Config is the optional configuration for an event-sink service.
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
	return []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}
}

func (serviceType) DefaultImagePolicy() string { return "build" }

func (serviceType) ValidateInterfaces(_ []manifest.Interface) error { return nil }

func (serviceType) Description() string {
	return "Generic XMiDT webhook consumer and event logger"
}

func (serviceType) DefaultImage() string { return "ghcr.io/gdcs-dev/event-sink" }

type renderer struct{}

func (renderer) Name() string { return "event-sink-renderer" }

func (renderer) Render(_ context.Context, input render.Input) (render.Result, error) {
	if len(input.Service.Instances) == 0 {
		return render.Result{}, fmt.Errorf("event-sink %q has no instances", input.Service.Name)
	}
	env := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])

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
	return render.Result{
		Renderer: "event-sink-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		},
	}, nil
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
