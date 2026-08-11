package genericcontainer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/genericcontainer"
	"gopkg.in/yaml.v3"
)

func TestGenericContainerGeneratesComposeAndEnv(t *testing.T) {
	genericcontainer.Register()
	st, ok := typeregistry.Lookup("generic-container")
	if !ok {
		t.Fatal("generic-container not registered")
	}

	var cfg yaml.Node
	src := "command: [/bin/sleep, infinity]\nenv: { FOO: bar }\nports: [\"8080:80\"]\n"
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "lan", Bridge: "edge-lan"}}}
	svc := plan.Service{
		Name:   "client",
		Type:   "generic-container",
		Image:  manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render generic-container: %v", err)
	}

	artifacts := map[string]string{}
	for _, a := range result.Artifacts {
		artifacts[a.Key] = a.Content
	}

	env := artifacts["compose.env"]
	compose := artifacts["compose.yaml"]
	entrypoint := artifacts["entrypoint.sh"]

	if env == "" || compose == "" {
		t.Fatalf("expected both compose.env and compose.yaml; got env=%q compose=%q", env, compose)
	}
	if entrypoint == "" {
		t.Fatal("expected entrypoint.sh artifact")
	}
	if _, ok := artifacts["resolv.conf"]; ok {
		t.Fatal("unexpected resolv.conf artifact: renderer should not emit resolv.conf")
	}

	for _, frag := range []string{"DEPLOYMENT_NAME=edge", "SERVICE_NAME=client", "FOO=bar"} {
		if !strings.Contains(env, frag) {
			t.Fatalf("compose.env missing %q:\n%s", frag, env)
		}
	}
	for _, frag := range []string{
		"client-1:",
		"image: docker.io/library/alpine:3.19",
		"8080:80",
		"entrypoint.sh:/run/vcpe/entrypoint.sh:ro",
		"entrypoint:",
	} {
		if !strings.Contains(compose, frag) {
			t.Fatalf("compose.yaml missing %q:\n%s", frag, compose)
		}
	}
	if strings.Contains(compose, "resolv.conf") {
		t.Fatalf("compose.yaml must not reference resolv.conf:\n%s", compose)
	}
	if !strings.Contains(entrypoint, "exec \"$@\"") {
		t.Fatalf("entrypoint.sh missing exec:\n%s", entrypoint)
	}
}

// TestGenericContainerStaticRole verifies that the entrypoint.sh is emitted and
// compose is well-formed when VCPE_INIT_STATIC_ROLE is used via config.env.
func TestGenericContainerStaticRole(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")

	var cfg yaml.Node
	src := "command: [\"/bin/sh\"]\nenv: { VCPE_INIT_STATIC_ROLE: lan }\n"
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:   "client",
		Type:   "generic-container",
		Image:  manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Network: "edge-lan", Device: "eth0", MAC: "02:00:00:00:00:0a",
				IPv4: "192.168.1.10/24", Gateway4: "192.168.1.1", DefaultRoute: true},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	artifacts := map[string]string{}
	for _, a := range result.Artifacts {
		artifacts[a.Key] = a.Content
	}

	if artifacts["entrypoint.sh"] == "" {
		t.Fatal("expected entrypoint.sh")
	}
	if _, ok := artifacts["resolv.conf"]; ok {
		t.Fatal("unexpected resolv.conf artifact")
	}
	env := artifacts["compose.env"]
	if !strings.Contains(env, "IFACE_LAN_DEFAULT_ROUTE=1") {
		t.Fatalf("compose.env missing IFACE_LAN_DEFAULT_ROUTE=1:\n%s", env)
	}
	if !strings.Contains(env, "VCPE_INIT_STATIC_ROLE=lan") {
		t.Fatalf("compose.env missing VCPE_INIT_STATIC_ROLE=lan:\n%s", env)
	}
}

// TestGenericContainerInitVarsPassThrough verifies that VCPE_INIT_* identity vars
// in config.env appear in compose.env and are not consumed at render time.
func TestGenericContainerInitVarsPassThrough(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")

	var cfg yaml.Node
	src := "env:\n  VCPE_INIT_HOSTNAME: phone-01\n  VCPE_INIT_MAC_ROLE: lan\n  VCPE_INIT_SLEEP: \"2\"\n  DEVICE_SERIAL: XB-001\n"
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal cfg: %v", err)
	}
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}

	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name:   "client",
		Type:   "generic-container",
		Image:  manifest.Image{Repository: "docker.io/library/alpine", Tag: "3.19"},
		Config: node,
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "lan", Device: "eth0", MAC: "02:00:00:00:00:0a"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	env := ""
	for _, a := range result.Artifacts {
		if a.Key == "compose.env" {
			env = a.Content
		}
	}
	for _, want := range []string{
		"VCPE_INIT_HOSTNAME=phone-01",
		"VCPE_INIT_MAC_ROLE=lan",
		"VCPE_INIT_SLEEP=2",
		"DEVICE_SERIAL=XB-001",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("compose.env missing %q:\n%s", want, env)
		}
	}
}

func TestGenericContainerRejectsUnknownConfig(t *testing.T) {
	genericcontainer.Register()
	st, _ := typeregistry.Lookup("generic-container")
	var cfg yaml.Node
	_ = yaml.Unmarshal([]byte("notafield: 1\n"), &cfg)
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}
