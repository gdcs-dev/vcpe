package webpa_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/webpa"
	"gopkg.in/yaml.v3"
)

func TestWebPAGoldenComposeEnv(t *testing.T) {
	webpa.Register()
	st, ok := typeregistry.Lookup("webpa")
	if !ok {
		t.Fatal("webpa not registered")
	}

	dep := plan.Deployment{Name: "edge", Networks: []plan.Network{{Role: "mgmt", Bridge: "edge-mgmt"}}}
	svc := plan.Service{
		Name:  "webpa",
		Type:  "webpa",
		Image: manifest.Image{Repository: "ghcr.io/gdcs-dev/webpa", Tag: "dev"},
		Instances: []plan.Instance{{Interfaces: []plan.Interface{
			{Role: "mgmt", Network: "edge-mgmt", Device: "eth0", MAC: "02:00:00:00:00:09", IPv4: "10.10.10.5", Gateway4: "10.10.10.1"},
		}}},
	}

	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc})
	if err != nil {
		t.Fatalf("render webpa: %v", err)
	}

	artifacts := map[string]string{}
	for _, a := range result.Artifacts {
		artifacts[a.Key] = a.Content
	}

	// compose.env must contain all IFACE_* vars
	wantEnv := strings.Join([]string{
		"DEPLOYMENT_NAME=edge",
		"SERVICE_NAME=webpa",
		"IMAGE=ghcr.io/gdcs-dev/webpa:dev",
		"IFACE_MGMT_BRIDGE=",
		"IFACE_MGMT_DEVICE=eth0",
		"IFACE_MGMT_GATEWAY4=10.10.10.1",
		"IFACE_MGMT_GATEWAY6=",
		"IFACE_MGMT_IPV4=10.10.10.5",
		"IFACE_MGMT_IPV6=",
		"IFACE_MGMT_MAC=02:00:00:00:00:09",
		"IFACE_MGMT_NETWORK=edge-mgmt",
	}, "\n") + "\n"
	if artifacts["compose.env"] != wantEnv {
		t.Fatalf("webpa compose.env mismatch:\n--- got ---\n%s\n--- want ---\n%s", artifacts["compose.env"], wantEnv)
	}

	// compose.yaml must reference the mgmt network (not a hardcoded string)
	composeYAML := artifacts["compose.yaml"]
	if composeYAML == "" {
		t.Fatal("webpa renderer did not produce compose.yaml")
	}
	if !strings.Contains(composeYAML, "IFACE_MGMT_NETWORK") {
		t.Errorf("compose.yaml should reference IFACE_MGMT_NETWORK, got:\n%s", composeYAML)
	}
	if strings.Contains(composeYAML, "mgmt:") && strings.Contains(composeYAML, "external: true") {
		// make sure the external network name is driven by env var, not hardcoded
		if strings.Contains(composeYAML, "name: mgmt") || strings.Contains(composeYAML, "name: edge-mgmt") {
			t.Errorf("compose.yaml must use env-var for network name, got:\n%s", composeYAML)
		}
	}
}

func TestWebPARejectsConfig(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")
	var cfg yaml.Node
	_ = yaml.Unmarshal([]byte("anything: 1\n"), &cfg)
	node := cfg
	if cfg.Kind == yaml.DocumentNode && len(cfg.Content) == 1 {
		node = *cfg.Content[0]
	}
	if err := st.ValidateConfig(node); err == nil {
		t.Fatal("expected webpa to reject any config")
	}
}
