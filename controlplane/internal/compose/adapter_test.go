package compose

import (
	"reflect"
	"testing"
)

func TestCommandArgsForComposeOperations(t *testing.T) {
	req := Request{ProjectName: "podman-bng-20", EnvFile: "runtime/20/compose.env", ComposeFile: "services/bng/compose.yaml"}
	up, err := commandArgs("up", req)
	if err != nil {
		t.Fatalf("up args: %v", err)
	}
	if !reflect.DeepEqual(up, []string{"-p", "podman-bng-20", "--env-file", "runtime/20/compose.env", "-f", "services/bng/compose.yaml", "up", "-d"}) {
		t.Fatalf("unexpected up args: %#v", up)
	}
	// --remove-orphans is added when RemoveOrphans is set.
	upOrphans, err := commandArgs("up", Request{ProjectName: "podman-bng-20", EnvFile: "runtime/20/compose.env", ComposeFile: "services/bng/compose.yaml", RemoveOrphans: true})
	if err != nil {
		t.Fatalf("up orphans args: %v", err)
	}
	if !reflect.DeepEqual(upOrphans, []string{"-p", "podman-bng-20", "--env-file", "runtime/20/compose.env", "-f", "services/bng/compose.yaml", "up", "-d", "--remove-orphans"}) {
		t.Fatalf("unexpected up orphans args: %#v", upOrphans)
	}
	// Service-scoped up passes service names after --remove-orphans.
	upScoped, err := commandArgs("up", Request{ProjectName: "podman-bng-20", EnvFile: "runtime/20/compose.env", ComposeFile: "services/bng/compose.yaml", RemoveOrphans: true, Services: []string{"client-2"}})
	if err != nil {
		t.Fatalf("scoped up args: %v", err)
	}
	if !reflect.DeepEqual(upScoped, []string{"-p", "podman-bng-20", "--env-file", "runtime/20/compose.env", "-f", "services/bng/compose.yaml", "up", "-d", "--remove-orphans", "client-2"}) {
		t.Fatalf("unexpected scoped up args: %#v", upScoped)
	}
	down, err := commandArgs("down", req)
	if err != nil {
		t.Fatalf("down args: %v", err)
	}
	if !reflect.DeepEqual(down, []string{"-p", "podman-bng-20", "--env-file", "runtime/20/compose.env", "-f", "services/bng/compose.yaml", "down"}) {
		t.Fatalf("unexpected down args: %#v", down)
	}
	ps, err := commandArgs("ps", req)
	if err != nil {
		t.Fatalf("ps args: %v", err)
	}
	if !reflect.DeepEqual(ps, []string{"-p", "podman-bng-20", "--env-file", "runtime/20/compose.env", "-f", "services/bng/compose.yaml", "ps"}) {
		t.Fatalf("unexpected ps args: %#v", ps)
	}
}

func TestCommandArgsValidation(t *testing.T) {
	if _, err := commandArgs("up", Request{}); err == nil {
		t.Fatalf("expected project-name validation failure")
	}
}

func TestGeneratedInputs(t *testing.T) {
	inputs := generatedInputs(Request{EnvFile: "env", ComposeFile: "compose.yaml"})
	if !reflect.DeepEqual(inputs, []string{"env", "compose.yaml"}) {
		t.Fatalf("unexpected generated inputs: %#v", inputs)
	}
}

func TestRequestRequiresProjectNameInAllOperations(t *testing.T) {
	adapter := New()
	if _, err := adapter.Up(t.Context(), Request{}); err == nil {
		t.Fatalf("expected up preflight validation failure")
	}
	if _, err := adapter.Down(t.Context(), Request{}); err == nil {
		t.Fatalf("expected down preflight validation failure")
	}
	if _, err := adapter.Status(t.Context(), Request{}); err == nil {
		t.Fatalf("expected status preflight validation failure")
	}
}
