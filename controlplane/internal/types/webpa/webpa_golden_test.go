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

	// compose.yaml must contain the resolved mgmt network for its instance.
	composeYAML := artifacts["compose.yaml"]
	if composeYAML == "" {
		t.Fatal("webpa renderer did not produce compose.yaml")
	}
	if !strings.Contains(composeYAML, "name: edge-mgmt") {
		t.Errorf("compose.yaml should contain the resolved mgmt network, got:\n%s", composeYAML)
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

func TestWebPAComposeReplicasHaveDistinctHealthPorts(t *testing.T) {
	webpa.Register()
	st, _ := typeregistry.Lookup("webpa")
	dep := plan.Deployment{Name: "edge"}
	svc := plan.Service{
		Name: "webpa", Type: "webpa", Image: manifest.Image{Repository: "webpa", Tag: "test"},
		Ports: []string{"8080:8080"},
		Instances: []plan.Instance{
			{Index: 0, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:01", IPv4: "10.0.0.2"}}},
			{Index: 1, Interfaces: []plan.Interface{{Role: "mgmt", Network: "edge-mgmt", MAC: "02:00:00:00:00:02", IPv4: "10.0.0.3"}}},
		},
	}
	result, err := st.Renderer().Render(context.Background(), render.Input{Deployment: dep, Service: svc, HealthPorts: map[int]int{0: 47000, 1: 47001}})
	if err != nil {
		t.Fatalf("render webpa replicas: %v", err)
	}
	var compose string
	for _, artifact := range result.Artifacts {
		if artifact.Key == "compose.yaml" {
			compose = artifact.Content
		}
	}
	for _, want := range []string{"webpa-1:", "webpa-2:", "127.0.0.1:47000:9878", "127.0.0.1:47001:9878", "instances/1/compose.env", "instances/2/compose.env"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml missing %q:\n%s", want, compose)
		}
	}
}
