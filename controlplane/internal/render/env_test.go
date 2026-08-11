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
