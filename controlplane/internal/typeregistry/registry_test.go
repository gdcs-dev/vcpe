package typeregistry_test

import (
	"reflect"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

func TestBaseServiceTypeDefaults(t *testing.T) {
	base := typeregistry.BaseServiceType{}
	wantHealth := typeregistry.HealthBehavior{
		Mode:          typeregistry.HealthModeCurated,
		ContainerPort: 9878,
	}
	if got := base.Health(); got != wantHealth {
		t.Errorf("Health() = %#v, want %#v", got, wantHealth)
	}
	if got := base.DefaultImagePolicy(); got != "build" {
		t.Errorf("DefaultImagePolicy() = %q, want build", got)
	}
	if err := base.ValidateInterfaces(nil); err != nil {
		t.Errorf("ValidateInterfaces(nil) = %v, want nil", err)
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

func TestBuiltInMetadata(t *testing.T) {
	types.Register()
	curated := typeregistry.HealthBehavior{Mode: typeregistry.HealthModeCurated, ContainerPort: 9878}
	optional := typeregistry.HealthBehavior{Mode: typeregistry.HealthModeOptional, ContainerPort: 9878}
	tests := []struct {
		name         string
		health       typeregistry.HealthBehavior
		description  string
		defaultImage string
		roles        []typeregistry.RoleRequirement
		renderer     string
	}{
		{name: "bng", health: curated, description: "Broadband Network Gateway \u2014 DHCP4/RADVD/DNS on WAN and CM segments", defaultImage: "ghcr.io/gdcs-dev/bng", roles: []typeregistry.RoleRequirement{{Role: "wan"}, {Role: "cm"}, {Role: "mgmt"}}, renderer: "bng-renderer"},
		{name: "event-sink", health: curated, description: "Generic XMiDT webhook consumer and event logger", defaultImage: "ghcr.io/gdcs-dev/event-sink", roles: []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}, renderer: "event-sink-renderer"},
		{name: "gateway", health: curated, description: "Cable-modem / CPE simulator with LAN bridging", defaultImage: "ghcr.io/gdcs-dev/gateway", roles: []typeregistry.RoleRequirement{{Role: "wan"}, {Role: "cm"}, {Role: "lan-p1"}}, renderer: "gateway-renderer"},
		{name: "generic-container", health: optional, description: "Catch-all generic container workload", renderer: "generic-container-renderer"},
		{name: "oktopus", health: curated, description: "Oktopus USP controller \u2014 cloud-native device management platform", roles: []typeregistry.RoleRequirement{{Role: "mgmt", Required: true}}, renderer: "oktopus-renderer"},
		{name: "webpa", health: curated, description: "USP/WebPA device-management server", defaultImage: "ghcr.io/gdcs-dev/webpa", roles: []typeregistry.RoleRequirement{{Role: "mgmt"}}, renderer: "webpa-renderer"},
		{name: "xb10", health: curated, description: "XB10 CPE gateway simulator", defaultImage: "ghcr.io/gdcs-dev/xb10", roles: []typeregistry.RoleRequirement{{Role: "wan"}, {Role: "cm"}}, renderer: "xb10-renderer"},
	}

	wantNames := make([]string, 0, len(tests))
	for _, test := range tests {
		wantNames = append(wantNames, test.name)
		t.Run(test.name, func(t *testing.T) {
			serviceType, ok := typeregistry.Lookup(test.name)
			if !ok {
				t.Fatalf("type is not registered")
			}
			if got := serviceType.Health(); got != test.health {
				t.Errorf("Health() = %#v, want %#v", got, test.health)
			}
			if got := serviceType.DefaultImagePolicy(); got != "build" {
				t.Errorf("DefaultImagePolicy() = %q, want build", got)
			}
			if err := serviceType.ValidateInterfaces(nil); err != nil {
				t.Errorf("ValidateInterfaces(nil) = %v, want nil", err)
			}
			if got := serviceType.Description(); got != test.description {
				t.Errorf("Description() = %q, want %q", got, test.description)
			}
			if got := serviceType.DefaultImage(); got != test.defaultImage {
				t.Errorf("DefaultImage() = %q, want %q", got, test.defaultImage)
			}
			if got := serviceType.ExpectedRoles(); !reflect.DeepEqual(got, test.roles) {
				t.Errorf("ExpectedRoles() = %#v, want %#v", got, test.roles)
			}
			if got := serviceType.Renderer().Name(); got != test.renderer {
				t.Errorf("Renderer().Name() = %q, want %q", got, test.renderer)
			}
		})
	}
	if got := typeregistry.Registered(); !reflect.DeepEqual(got, wantNames) {
		t.Errorf("Registered() = %q, want exactly %q", got, wantNames)
	}
}
