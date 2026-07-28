// Package eventsink implements the event-sink service type. Like webpa, it has
// no typed config and renders only the interface environment consumed by the
// curated compose file at services/event-sink/compose.yaml.
package eventsink

import (
	"context"
	"fmt"
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

// ValidateConfig rejects any config: event-sink takes none; all configuration
// is supplied as environment variables in the curated compose.yaml.
func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind != 0 {
		return fmt.Errorf("event-sink does not accept config")
	}
	return nil
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
