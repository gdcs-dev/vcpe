package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
	"gopkg.in/yaml.v3"
)

type preflightTestRenderer struct{}

func (preflightTestRenderer) Name() string { return "preflight-test-renderer" }

func (preflightTestRenderer) Render(context.Context, render.Input) (render.Result, error) {
	return render.Result{}, nil
}

type unrestrictedReplicaType struct {
	typeregistry.BaseServiceType
	name string
}

func (serviceType unrestrictedReplicaType) Type() string                      { return serviceType.name }
func (unrestrictedReplicaType) ValidateConfig(yaml.Node) error                { return nil }
func (unrestrictedReplicaType) Renderer() render.Renderer                     { return preflightTestRenderer{} }
func (unrestrictedReplicaType) ExpectedRoles() []typeregistry.RoleRequirement { return nil }
func (unrestrictedReplicaType) Description() string                           { return "preflight test type" }
func (unrestrictedReplicaType) DefaultImage() string                          { return "example.invalid/test" }

type singletonReplicaType struct {
	unrestrictedReplicaType
}

func (singletonReplicaType) ValidateReplicas(replicas int) error {
	if replicas > 1 {
		return fmt.Errorf("embedded topology is singleton; got %d replicas", replicas)
	}
	return nil
}

func TestPreflightOptionalReplicaValidation(t *testing.T) {
	typeregistry.Register(unrestrictedReplicaType{name: "preflight-unrestricted"})
	typeregistry.Register(singletonReplicaType{unrestrictedReplicaType{name: "preflight-singleton"}})

	document := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{Services: []manifest.Service{{
			Name:     "subscriber",
			Type:     "preflight-unrestricted",
			Replicas: 2,
		}}},
	}
	if err := Preflight(document); err != nil {
		t.Fatalf("Preflight() unrestricted type error = %v", err)
	}

	document.Spec.Services[0].Type = "preflight-singleton"
	err := Preflight(document)
	if err == nil || !strings.Contains(err.Error(), `service "subscriber" replicas: embedded topology is singleton; got 2 replicas`) {
		t.Fatalf("Preflight() singleton error = %v", err)
	}
}

func TestPreflightWebConfigRequirements(t *testing.T) {
	types.Register()
	document := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Metadata:   manifest.Metadata{Name: "edge"},
		Spec: manifest.Spec{
			Networks: []manifest.Network{{Role: "mgmt"}},
			Services: []manifest.Service{{
				Name: "webconfig", Type: "webconfig", Replicas: 1,
			}},
		},
	}
	if err := Preflight(document); err == nil || !strings.Contains(err.Error(), `does not satisfy expected role "mgmt"`) {
		t.Fatalf("Preflight() missing management interface error = %v", err)
	}

	document.Spec.Services[0].Interfaces = []manifest.Interface{{Role: "mgmt"}}
	document.Spec.Services[0].Replicas = 2
	if err := Preflight(document); err == nil || !strings.Contains(err.Error(), "WebConfig is singleton; got 2 replicas") {
		t.Fatalf("Preflight() multiple WebConfig replicas error = %v", err)
	}
}
