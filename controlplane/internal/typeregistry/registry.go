// Package typeregistry is the extension point for service types. Each service
// type registers an implementation that knows how to validate its typed config,
// render its artifacts, and declare the network roles it expects. New workloads
// are added by registering here rather than by editing the planner or renderer.
package typeregistry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"gopkg.in/yaml.v3"
)

// RoleRequirement declares a network role a service type expects to be attached
// to. Required roles must be present on the service; optional ones may be.
type RoleRequirement struct {
	Role     string
	Required bool
}

// ServiceType is implemented by every supported workload type.
type ServiceType interface {
	// Type is the discriminator matched against a manifest service's type field.
	Type() string
	// ValidateConfig strictly decodes and validates the opaque config subtree.
	ValidateConfig(node yaml.Node) error
	// Renderer returns the renderer that produces this type's artifacts.
	Renderer() render.Renderer
	// ExpectedRoles declares the network roles this type expects.
	ExpectedRoles() []RoleRequirement
	// DefaultImagePolicy returns the default pull policy ("build" or "pull").
	DefaultImagePolicy() string
	// Description returns a human-readable one-line description of the service
	// type, used by tooling such as the visual manifest editor palette.
	Description() string
	// DefaultImage returns the default OCI image repository for this type
	// (e.g. "ghcr.io/gdcs-dev/bng"). Returns an empty string when no canonical
	// default exists and the user must supply the image.
	DefaultImage() string
	// ValidateInterfaces validates the per-interface fields declared by a service
	// of this type. It is called at preflight for every service. Types with no
	// per-interface constraints must return nil.
	ValidateInterfaces(interfaces []manifest.Interface) error
}

var (
	mu       sync.RWMutex
	registry = map[string]ServiceType{}
)

// Register adds a service type. It panics on duplicate registration because
// that indicates a programming error in package init wiring.
func Register(t ServiceType) {
	mu.Lock()
	defer mu.Unlock()
	name := t.Type()
	if name == "" {
		panic("typeregistry: service type with empty Type()")
	}
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("typeregistry: duplicate registration for type %q", name))
	}
	registry[name] = t
}

// Lookup returns the registered service type and whether it was found.
func Lookup(name string) (ServiceType, bool) {
	mu.RLock()
	defer mu.RUnlock()
	t, ok := registry[name]
	return t, ok
}

// Registered returns the sorted list of registered type names.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
