package render_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
)

func TestSortedEnv(t *testing.T) {
	values := map[string]string{"ZED": "last", "ALPHA": "first", "EMPTY": ""}
	before := map[string]string{"ZED": "last", "ALPHA": "first", "EMPTY": ""}
	if got, want := render.SortedEnv(values), []string{"ALPHA=first", "EMPTY=", "ZED=last"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SortedEnv() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(values, before) {
		t.Errorf("SortedEnv mutated input: got %#v, want %#v", values, before)
	}
	if got := render.SortedEnv(nil); len(got) != 0 {
		t.Errorf("SortedEnv(nil) = %#v, want empty", got)
	}
}

func TestInstanceEnvArtifacts(t *testing.T) {
	input := render.Input{Service: plan.Service{Instances: []plan.Instance{{Index: 2}, {Index: 0}}}}
	var called []int
	artifacts := render.InstanceEnvArtifacts(input, func(instance plan.Instance) []string {
		called = append(called, instance.Index)
		return []string{"INSTANCE=" + string(rune('0'+instance.Index))}
	})
	want := []render.Artifact{
		{Key: "compose.env", Content: "INSTANCE=2\n"},
		{Key: "instances/3/compose.env", Content: "INSTANCE=2\n"},
		{Key: "instances/1/compose.env", Content: "INSTANCE=0\n"},
	}
	if !reflect.DeepEqual(artifacts, want) {
		t.Errorf("InstanceEnvArtifacts() = %#v, want %#v", artifacts, want)
	}
	if !reflect.DeepEqual(called, []int{2, 0}) {
		t.Errorf("callback order = %#v, want plan order", called)
	}
	if got := render.InstanceEnvArtifacts(render.Input{}, func(plan.Instance) []string { return nil }); len(got) != 0 {
		t.Errorf("zero-instance artifacts = %#v, want empty", got)
	}
}

func TestImageRefCurrentBehavior(t *testing.T) {
	tests := []struct {
		name  string
		image manifest.Image
		want  string
	}{
		{name: "empty repository", want: ""},
		{name: "omitted tag", image: manifest.Image{Repository: "example/workload"}, want: "example/workload:latest"},
		{name: "whitespace-only tag", image: manifest.Image{Repository: "example/workload", Tag: "  "}, want: "example/workload:latest"},
		{name: "explicit tag", image: manifest.Image{Repository: "example/workload", Tag: "v2"}, want: "example/workload:v2"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := render.ImageRef(testCase.image); got != testCase.want {
				t.Errorf("ImageRef(%+v) = %q, want %q", testCase.image, got, testCase.want)
			}
		})
	}
}

func TestIPWithPrefix(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		cidr string
		want string
	}{
		{name: "empty IP", cidr: "10.1.2.0/24", want: ""},
		{name: "empty CIDR", ip: "10.1.2.3", want: "10.1.2.3"},
		{name: "invalid CIDR", ip: "10.1.2.3", cidr: "not-a-cidr", want: "10.1.2.3"},
		{name: "IPv4", ip: "10.1.2.3", cidr: "10.1.2.0/24", want: "10.1.2.3/24"},
		{name: "IPv6", ip: "2001:db8::3", cidr: "2001:db8::/64", want: "2001:db8::3/64"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := render.IPWithPrefix(testCase.ip, testCase.cidr); got != testCase.want {
				t.Errorf("IPWithPrefix(%q, %q) = %q, want %q", testCase.ip, testCase.cidr, got, testCase.want)
			}
		})
	}
}

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
