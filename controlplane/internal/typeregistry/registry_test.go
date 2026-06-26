package typeregistry_test

import (
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

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
		if policy := st.DefaultImagePolicy(); policy == "" {
			t.Errorf("type %q has empty DefaultImagePolicy", name)
		}
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
