// Package webpa implements the WebPA service type. WebPA has no typed config;
// it renders only the interface environment consumed by the curated compose
// file shipped at services/webpa/compose.yaml.
package webpa

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

// TypeName is the manifest discriminator for WebPA.
const TypeName = "webpa"

type serviceType struct{}

func (serviceType) Type() string { return TypeName }

// ValidateConfig rejects any config: WebPA takes none.
func (serviceType) ValidateConfig(node yaml.Node) error {
	if node.Kind != 0 {
		return fmt.Errorf("webpa does not accept config")
	}
	return nil
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
	env := render.IfaceEnv(input.Deployment, input.Service, input.Service.Instances[0])
	return render.Result{
		Renderer: "webpa-renderer",
		Artifacts: []render.Artifact{
			{Key: "compose.env", Content: strings.Join(env, "\n") + "\n"},
		},
	}, nil
}

// Register wires this service type into the global registry. It is idempotent.
func Register() { once.Do(func() { typeregistry.Register(serviceType{}) }) }

var once sync.Once
