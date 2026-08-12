package render_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
)

func TestSortedEnv(t *testing.T) {
	env := map[string]string{
		"Z_LAST":  "value with spaces",
		"A_FIRST": "left=right",
		"MIDDLE":  "",
	}
	original := map[string]string{
		"Z_LAST":  "value with spaces",
		"A_FIRST": "left=right",
		"MIDDLE":  "",
	}

	got := render.SortedEnv(env)
	want := []string{"A_FIRST=left=right", "MIDDLE=", "Z_LAST=value with spaces"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedEnv() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(env, original) {
		t.Fatalf("SortedEnv() mutated input: got %#v, want %#v", env, original)
	}
}

func TestSortedEnvEmpty(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			if got := render.SortedEnv(env); len(got) != 0 {
				t.Fatalf("SortedEnv() = %#v, want empty", got)
			}
		})
	}
}

func TestInstanceEnvArtifacts(t *testing.T) {
	input := render.Input{Service: plan.Service{Instances: []plan.Instance{
		{Index: 2},
		{Index: 0},
		{Index: 1},
	}}}

	got := render.InstanceEnvArtifacts(input, func(instance plan.Instance) []string {
		return []string{
			"INDEX=" + string(rune('0'+instance.Index)),
			"VALUE=preserved\n",
		}
	})
	want := []render.Artifact{
		{Key: "instances/3/compose.env", Content: "INDEX=2\nVALUE=preserved\n"},
		{Key: "compose.env", Content: "INDEX=0\nVALUE=preserved\n"},
		{Key: "instances/1/compose.env", Content: "INDEX=0\nVALUE=preserved\n"},
		{Key: "instances/2/compose.env", Content: "INDEX=1\nVALUE=preserved\n"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("InstanceEnvArtifacts() = %#v, want %#v", got, want)
	}
}

func TestInstanceEnvArtifactsNoInstances(t *testing.T) {
	called := false
	got := render.InstanceEnvArtifacts(render.Input{}, func(plan.Instance) []string {
		called = true
		return nil
	})
	if len(got) != 0 {
		t.Fatalf("InstanceEnvArtifacts() = %#v, want empty", got)
	}
	if called {
		t.Fatal("InstanceEnvArtifacts() called callback with no instances")
	}
}

func TestImageRef(t *testing.T) {
	tests := []struct {
		name  string
		image manifest.Image
		want  string
	}{
		{name: "empty repository", image: manifest.Image{}, want: ""},
		{name: "omitted tag", image: manifest.Image{Repository: "example.test/workload"}, want: "example.test/workload:latest"},
		{name: "whitespace-only tag", image: manifest.Image{Repository: "example.test/workload", Tag: " \t "}, want: "example.test/workload:latest"},
		{name: "explicit tag", image: manifest.Image{Repository: "example.test/workload", Tag: "v2"}, want: "example.test/workload:v2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := render.ImageRef(test.image); got != test.want {
				t.Errorf("ImageRef(%#v) = %q, want %q", test.image, got, test.want)
			}
		})
	}
}

func TestImageReferenceSurfacesAgree(t *testing.T) {
	tests := map[string]manifest.Image{
		"empty repository":    {},
		"omitted tag":         {Repository: "example.test/workload"},
		"whitespace-only tag": {Repository: "example.test/workload", Tag: " \t "},
		"explicit tag":        {Repository: "example.test/workload", Tag: "v2"},
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			if got, want := render.ImageRef(spec), image.ImageReference(spec); got != want {
				t.Errorf("render reference = %q, image reference = %q", got, want)
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
		{name: "invalid CIDR", ip: "10.1.2.3", cidr: "invalid", want: "10.1.2.3"},
		{name: "IPv4", ip: "10.1.2.3", cidr: "10.1.2.0/24", want: "10.1.2.3/24"},
		{name: "IPv6", ip: "2001:db8::2", cidr: "2001:db8::/64", want: "2001:db8::2/64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := render.IPWithPrefix(test.ip, test.cidr); got != test.want {
				t.Errorf("IPWithPrefix(%q, %q) = %q, want %q", test.ip, test.cidr, got, test.want)
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
