package routerd_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/routerd"
	"gopkg.in/yaml.v3"
)

func artifact(result render.Result, key string) (string, bool) {
	for _, a := range result.Artifacts {
		if a.Key == key {
			return a.Content, true
		}
	}
	return "", false
}

func lookupRouterd(t *testing.T) typeregistry.ServiceType {
	t.Helper()
	routerd.Register()
	st, ok := typeregistry.Lookup(routerd.TypeName)
	if !ok {
		t.Fatal("routerd type not registered")
	}
	return st
}

func TestRouterdBridgedLANPlusDHCPWan(t *testing.T) {
	st := lookupRouterd(t)
	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "wan", IPv4: &plan.Family{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"}},
		},
	}
	svc := plan.Service{
		Name:  "routerd",
		Type:  routerd.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/routerd", Tag: "dev"},
		Instances: []plan.Instance{
			{
				Index: 0,
				Interfaces: []plan.Interface{
					{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:aa:bb:cc:dd:01", Addressing: "dhcp"},
					{Role: "lan-p1", Device: "eth1", MAC: "02:aa:bb:cc:dd:02", Bridge: "brlan0"},
					{Role: "lan-p2", Device: "eth2", MAC: "02:aa:bb:cc:dd:03", Bridge: "brlan0"},
				},
			},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render routerd: %v", err)
	}

	content, ok := artifact(result, "etc/routerd/config.json")
	if !ok {
		t.Fatal("expected etc/routerd/config.json artifact")
	}

	var doc struct {
		Resources []struct {
			APIVersion string              `json:"api_version"`
			Kind       string              `json:"kind"`
			Metadata   struct{ ID string } `json:"metadata"`
			Spec       json.RawMessage     `json:"spec"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("unmarshal rendered config: %v\n%s", err, content)
	}

	var (
		interfaces []string
		bridges    []string
		wanPolicy  []string
	)
	for _, r := range doc.Resources {
		if r.APIVersion != "net.routerd/v1" {
			t.Fatalf("resource %s has unexpected api_version %q", r.Metadata.ID, r.APIVersion)
		}
		switch r.Kind {
		case "Interface":
			interfaces = append(interfaces, r.Metadata.ID)
		case "Bridge":
			bridges = append(bridges, r.Metadata.ID)
			if !strings.Contains(string(r.Spec), `"eth1"`) || !strings.Contains(string(r.Spec), `"eth2"`) {
				t.Fatalf("bridge %s spec missing expected members:\n%s", r.Metadata.ID, r.Spec)
			}
		case "WanPolicy":
			wanPolicy = append(wanPolicy, r.Metadata.ID)
			if !strings.Contains(string(r.Spec), `"kind": "dhcp"`) {
				t.Fatalf("wan policy spec expected dhcp source:\n%s", r.Spec)
			}
			if !strings.Contains(string(r.Spec), `"eth0"`) {
				t.Fatalf("wan policy spec expected to reference eth0:\n%s", r.Spec)
			}
		}
	}

	if len(interfaces) != 3 {
		t.Fatalf("expected 3 Interface resources, got %v", interfaces)
	}
	if len(bridges) != 1 || bridges[0] != "brlan0" {
		t.Fatalf("expected one brlan0 Bridge resource, got %v", bridges)
	}
	if len(wanPolicy) != 1 {
		t.Fatalf("expected exactly one WanPolicy resource (bridged LAN members must not get one), got %v", wanPolicy)
	}
}

func TestRouterdStaticWanPolicy(t *testing.T) {
	st := lookupRouterd(t)
	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "wan", IPv4: &plan.Family{CIDR: "10.7.200.0/24", Gateway: "10.7.200.1"}},
		},
	}
	svc := plan.Service{
		Name:  "routerd",
		Type:  routerd.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/routerd", Tag: "dev"},
		Instances: []plan.Instance{
			{
				Index: 0,
				Interfaces: []plan.Interface{
					{Role: "wan", Network: "edge-wan", Device: "eth0", MAC: "02:aa:bb:cc:dd:01", IPv4: "10.7.200.50", Gateway4: "10.7.200.1", Addressing: "static"},
				},
			},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render routerd: %v", err)
	}
	content, _ := artifact(result, "etc/routerd/config.json")
	for _, frag := range []string{`"kind": "static"`, `"address": "10.7.200.50"`, `"prefix_len": 24`, `"gateway": "10.7.200.1"`} {
		if !strings.Contains(content, frag) {
			t.Fatalf("expected static WanPolicy to contain %q:\n%s", frag, content)
		}
	}
}

func TestRouterdUnbridgedInterfaceHasNoBridgeResource(t *testing.T) {
	st := lookupRouterd(t)
	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:  "routerd",
		Type:  routerd.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/routerd", Tag: "dev"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{
				{Role: "wan", Device: "eth0", Addressing: "dhcp"},
			}},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render routerd: %v", err)
	}
	content, _ := artifact(result, "etc/routerd/config.json")
	if strings.Contains(content, `"kind": "Bridge"`) {
		t.Fatalf("expected no Bridge resource when no interface declares a bridge:\n%s", content)
	}
}

func TestRouterdMultipleUnbridgedInterfacesGetDistinctPriority(t *testing.T) {
	st := lookupRouterd(t)
	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:  "routerd",
		Type:  routerd.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/routerd", Tag: "dev"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{
				{Role: "wan", Device: "eth0", Addressing: "dhcp", DefaultRoute: true},
				{Role: "cm", Device: "eth1", Addressing: "dhcp"},
			}},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render routerd: %v", err)
	}
	content, _ := artifact(result, "etc/routerd/config.json")

	var doc struct {
		Resources []struct {
			Kind     string              `json:"kind"`
			Metadata struct{ ID string } `json:"metadata"`
			Spec     json.RawMessage     `json:"spec"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("unmarshal rendered config: %v\n%s", err, content)
	}

	priorities := map[string]string{}
	for _, r := range doc.Resources {
		if r.Kind != "WanPolicy" {
			continue
		}
		priorities[r.Metadata.ID] = string(r.Spec)
	}
	if len(priorities) != 2 {
		t.Fatalf("expected 2 WanPolicy resources, got %v", priorities)
	}
	if !strings.Contains(priorities["eth0-wan-policy"], `"priority": 1`) {
		t.Fatalf("expected the defaultRoute interface to get priority 1:\n%s", priorities["eth0-wan-policy"])
	}
	if strings.Contains(priorities["eth1-wan-policy"], `"priority": 1`) {
		t.Fatalf("expected the non-defaultRoute interface to NOT get priority 1 (would conflict):\n%s", priorities["eth1-wan-policy"])
	}
}

func TestRouterdBridgeIPv4ProducesIpAddressResource(t *testing.T) {
	st := lookupRouterd(t)
	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:  "routerd",
		Type:  routerd.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/routerd", Tag: "dev"},
		Bridges: []manifest.BridgeSpec{
			{Name: "brlan0", IPv4: "10.0.0.1/24"},
		},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{
				{Role: "lan-p1", Device: "eth0", Bridge: "brlan0"},
				{Role: "lan-p2", Device: "eth1", Bridge: "brlan0"},
			}},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render routerd: %v", err)
	}
	content, _ := artifact(result, "etc/routerd/config.json")
	for _, frag := range []string{`"kind": "IpAddress"`, `"kind": "Bridge"`, `"id": "brlan0"`, `"address": "10.0.0.1"`, `"prefix_len": 24`} {
		if !strings.Contains(content, frag) {
			t.Fatalf("expected bridge IpAddress to contain %q:\n%s", frag, content)
		}
	}
}

func TestRouterdBridgeWithoutIPv4HasNoIpAddressResource(t *testing.T) {
	st := lookupRouterd(t)
	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:  "routerd",
		Type:  routerd.TypeName,
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/routerd", Tag: "dev"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{
				{Role: "lan-p1", Device: "eth0", Bridge: "brlan0"},
			}},
		},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render routerd: %v", err)
	}
	content, _ := artifact(result, "etc/routerd/config.json")
	if strings.Contains(content, `"kind": "IpAddress"`) {
		t.Fatalf("expected no IpAddress resource when the bridge declares no ipv4:\n%s", content)
	}
}

func TestRouterdRejectsUnknownConfigField(t *testing.T) {
	st := lookupRouterd(t)
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("bridges:\n  - name: brlan0\n"), &node); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = *node.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected an unknown config field (bridges is not routerd config; it belongs at services[].bridges) to be rejected")
	}
}
