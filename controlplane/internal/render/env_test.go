package render_test

import (
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
)

func TestIfaceEnvDefaultRoute(t *testing.T) {
	dep := plan.Deployment{Name: "test"}
	svc := plan.Service{Name: "client"}
	inst := plan.Instance{
		Interfaces: []plan.Interface{
			{Role: "lan-p1", Device: "eth0", DefaultRoute: true},
			{Role: "mgmt", Device: "mgmt0"},
		},
	}
	env := strings.Join(render.IfaceEnv(dep, svc, inst), "\n")
	if !strings.Contains(env, "IFACE_LAN_P1_DEFAULT_ROUTE=1") {
		t.Errorf("expected IFACE_LAN_P1_DEFAULT_ROUTE=1 in env:\n%s", env)
	}
	if strings.Contains(env, "IFACE_MGMT_DEFAULT_ROUTE") {
		t.Errorf("expected no IFACE_MGMT_DEFAULT_ROUTE in env:\n%s", env)
	}
}

func TestIfaceEnvAddressing(t *testing.T) {
	dep := plan.Deployment{Name: "test"}
	svc := plan.Service{Name: "client"}
	inst := plan.Instance{
		Interfaces: []plan.Interface{
			{Role: "wan", Device: "eth0", Addressing: "static"},
			{Role: "mgmt", Device: "eth1"},
		},
	}
	env := strings.Join(render.IfaceEnv(dep, svc, inst), "\n")
	if !strings.Contains(env, "IFACE_WAN_ADDRESSING=static") {
		t.Errorf("expected IFACE_WAN_ADDRESSING=static in env:\n%s", env)
	}
	if !strings.Contains(env, "IFACE_MGMT_ADDRESSING=dhcp") {
		t.Errorf("expected IFACE_MGMT_ADDRESSING=dhcp (default) in env:\n%s", env)
	}
}

func TestIfaceEnvNetworkManaged(t *testing.T) {
	dep := plan.Deployment{Name: "test"}
	svc := plan.Service{Name: "client"}
	inst := plan.Instance{
		Interfaces: []plan.Interface{
			{Role: "mgmt", Device: "eth0", ManagedNetwork: true},
			{Role: "wan", Device: "eth1"},
		},
	}
	env := strings.Join(render.IfaceEnv(dep, svc, inst), "\n")
	if !strings.Contains(env, "IFACE_MGMT_NETWORK_MANAGED=1") {
		t.Errorf("expected IFACE_MGMT_NETWORK_MANAGED=1 in env:\n%s", env)
	}
	if strings.Contains(env, "IFACE_WAN_NETWORK_MANAGED") {
		t.Errorf("expected no IFACE_WAN_NETWORK_MANAGED for an unmanaged network in env:\n%s", env)
	}
}
