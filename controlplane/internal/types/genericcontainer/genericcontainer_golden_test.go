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

	var env, compose string
	for _, a := range result.Artifacts {
		switch a.Key {
		case "compose.env":
			env = a.Content
		case "compose.yaml":
			compose = a.Content
		}
	}
	if env == "" || compose == "" {
		t.Fatalf("expected both compose.env and compose.yaml; got env=%q compose=%q", env, compose)
	}
	for _, frag := range []string{"DEPLOYMENT_NAME=edge", "SERVICE_NAME=client", "FOO=bar"} {
		if !strings.Contains(env, frag) {
			t.Fatalf("compose.env missing %q:\n%s", frag, env)
		}
	}
	for _, frag := range []string{"client:", "image: docker.io/library/alpine:3.19", "8080:80"} {
		if !strings.Contains(compose, frag) {
			t.Fatalf("compose.yaml missing %q:\n%s", frag, compose)
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
