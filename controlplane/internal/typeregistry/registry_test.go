package typeregistry_test

import (
	"reflect"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

func TestBaseServiceTypeDefaults(t *testing.T) {
	var base typeregistry.BaseServiceType
	if got, want := base.Health(), (typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878}); got != want {
		t.Errorf("Health() = %+v, want %+v", got, want)
	}
	if got := base.DefaultImagePolicy(); got != "build" {
		t.Errorf("DefaultImagePolicy() = %q, want build", got)
	}
	if err := base.ValidateInterfaces([]manifest.Interface{{Role: "mgmt"}}); err != nil {
		t.Errorf("ValidateInterfaces() error = %v, want nil", err)
	}
}

func TestBuiltInMetadata(t *testing.T) {
	types.Register()
	tests := []struct {
		name         string
		health       typeregistry.HealthBehavior
		imagePolicy  string
		description  string
		defaultImage string
		roles        []typeregistry.RoleRequirement
		renderer     string
	}{
		{
			name:         "bng",
			health:       typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878},
			imagePolicy:  "build",
			description:  "Broadband Network Gateway — DHCP4/RADVD/DNS on WAN and CM segments",
			defaultImage: "ghcr.io/gdcs-dev/bng",
			roles:        []typeregistry.RoleRequirement{{Role: "wan"}, {Role: "cm"}, {Role: "mgmt"}},
			renderer:     "bng-renderer",
		},
		{
			name:         "event-sink",
			health:       typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878},
			imagePolicy:  "build",
			description:  "Generic XMiDT webhook consumer and event logger",
			defaultImage: "ghcr.io/gdcs-dev/event-sink",
			roles:        []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}},
			renderer:     "event-sink-renderer",
		},
		{
			name:         "gateway",
			health:       typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878},
			imagePolicy:  "build",
			description:  "Cable-modem / CPE simulator with LAN bridging",
			defaultImage: "ghcr.io/gdcs-dev/gateway",
			roles:        []typeregistry.RoleRequirement{{Role: "wan"}, {Role: "cm"}, {Role: "lan-p1"}},
			renderer:     "gateway-renderer",
		},
		{
			name:        "generic-container",
			health:      typeregistry.HealthBehavior{Mode: typeregistry.HealthModeOptional, ContainerPort: 9878},
			imagePolicy: "build",
			description: "Catch-all generic container workload",
			renderer:    "generic-container-renderer",
		},
		{
			name:        "oktopus",
			health:      typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878},
			imagePolicy: "build",
			description: "Oktopus USP controller — cloud-native device management platform",
			roles:       []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}},
			renderer:    "oktopus-renderer",
		},
		{
			name:         "webpa",
			health:       typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878},
			imagePolicy:  "build",
			description:  "USP/WebPA device-management server",
			defaultImage: "ghcr.io/gdcs-dev/webpa",
			roles:        []typeregistry.RoleRequirement{{Role: "mgmt"}},
			renderer:     "webpa-renderer",
		},
		{
			name:         "xb10",
			health:       typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878},
			imagePolicy:  "build",
			description:  "XB10 CPE gateway simulator",
			defaultImage: "ghcr.io/gdcs-dev/xb10",
			roles:        []typeregistry.RoleRequirement{{Role: "wan"}, {Role: "cm"}},
			renderer:     "xb10-renderer",
		},
	}
	if registered := typeregistry.Registered(); len(registered) != len(tests) {
		t.Fatalf("registered type count = %d, want %d: %v", len(registered), len(tests), registered)
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			registered, ok := typeregistry.Lookup(testCase.name)
			if !ok {
				t.Fatalf("%s is not registered", testCase.name)
			}
			if got := registered.Health(); got != testCase.health {
				t.Errorf("Health() = %+v, want %+v", got, testCase.health)
			}
			if got := registered.DefaultImagePolicy(); got != testCase.imagePolicy {
				t.Errorf("DefaultImagePolicy() = %q, want %q", got, testCase.imagePolicy)
			}
			if got := registered.Description(); got != testCase.description {
				t.Errorf("Description() = %q, want %q", got, testCase.description)
			}
			if got := registered.DefaultImage(); got != testCase.defaultImage {
				t.Errorf("DefaultImage() = %q, want %q", got, testCase.defaultImage)
			}
			if got := registered.ExpectedRoles(); !reflect.DeepEqual(got, testCase.roles) {
				t.Errorf("ExpectedRoles() = %#v, want %#v", got, testCase.roles)
			}
			if got := registered.Renderer().Name(); got != testCase.renderer {
				t.Errorf("Renderer().Name() = %q, want %q", got, testCase.renderer)
			}
			if err := registered.ValidateInterfaces([]manifest.Interface{{Role: "mgmt"}}); err != nil {
				t.Errorf("ValidateInterfaces() error = %v, want nil", err)
			}
		})
	}
}

// TestRegistryCompleteness asserts that every registered service type supplies
// the full behavior contract: a validator, a renderer, and an expected-roles
// declaration. This guards the registry invariant that "supported" means a type
// can be validated and rendered.
func TestRegistryCompleteness(t *testing.T) {
	types.Register()

	registered := typeregistry.Registered()
	if len(registered) == 0 {
		t.Fatal("expected at least one registered service type")
	}

	// The v1 type set is locked in decisions.md.
	want := map[string]bool{"bng": false, "gateway": false, "webpa": false, "generic-container": false}
	for _, name := range registered {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected v1 type %q to be registered", name)
		}
	}

	for _, name := range registered {
		st, ok := typeregistry.Lookup(name)
		if !ok {
			t.Fatalf("Registered() returned %q but Lookup failed", name)
		}
		if st.Type() != name {
			t.Errorf("type %q reports Type()=%q", name, st.Type())
		}
		if st.Renderer() == nil {
			t.Errorf("type %q has nil renderer", name)
		}
		if st.Renderer() != nil && st.Renderer().Name() == "" {
			t.Errorf("type %q renderer has empty Name()", name)
		}
		if !st.Health().Valid() {
			t.Errorf("type %q has invalid health behavior: %+v", name, st.Health())
		}
		if policy := st.DefaultImagePolicy(); policy == "" {
			t.Errorf("type %q has empty DefaultImagePolicy", name)
		}
		if desc := st.Description(); desc == "" {
			t.Errorf("type %q has empty Description", name)
		}
		// DefaultImage may be empty for types with no canonical default image.
		_ = st.DefaultImage()
		// ValidateInterfaces with nil slice must not panic and must return nil
		// for types that impose no per-interface constraints.
		// (Gateway will return an error for missing device, tested separately.)
		_ = st.ValidateInterfaces(nil)
		// ExpectedRoles may be empty (e.g. generic-container) but must not panic
		// and each declared role must be non-empty.
		for _, req := range st.ExpectedRoles() {
			if req.Role == "" {
				t.Errorf("type %q declares an expected role with empty name", name)
			}
		}
	}
}

// TestUnregisteredLookupFails confirms the registry reports unknown types as
// unsupported rather than returning a zero value silently.
func TestUnregisteredLookupFails(t *testing.T) {
	types.Register()
	if _, ok := typeregistry.Lookup("does-not-exist"); ok {
		t.Fatal("expected lookup of unknown type to fail")
	}
}
