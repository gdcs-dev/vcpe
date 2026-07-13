package servicecmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/contract"
)

func TestRunUsesDefaultCommand(t *testing.T) {
	var got []string
	err := Run(context.Background(), Config{
		Service:     "bng",
		DefaultExec: []string{"/bin/echo", "ok"},
		RunCommand: func(_ context.Context, argv []string) error {
			got = append([]string(nil), argv...)
			return nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("run default command: %v", err)
	}
	if len(got) != 2 || got[0] != "/bin/echo" || got[1] != "ok" {
		t.Fatalf("unexpected default command: %v", got)
	}
}

func TestRunUsesArgsCommandOverride(t *testing.T) {
	var got []string
	err := Run(context.Background(), Config{
		Service:     "gateway",
		DefaultExec: []string{"/bin/false"},
		RunCommand: func(_ context.Context, argv []string) error {
			got = append([]string(nil), argv...)
			return nil
		},
	}, []string{"/bin/echo", "service"})
	if err != nil {
		t.Fatalf("run args command: %v", err)
	}
	if len(got) != 2 || got[0] != "/bin/echo" || got[1] != "service" {
		t.Fatalf("unexpected args command: %v", got)
	}
}

func TestRunRequiresServiceName(t *testing.T) {
	err := Run(context.Background(), Config{DefaultExec: []string{"/bin/echo"}}, nil)
	if err == nil {
		t.Fatal("expected missing service error")
	}
	if !strings.Contains(err.Error(), "service name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPropagatesCommandFailure(t *testing.T) {
	err := Run(context.Background(), Config{
		Service:     "routerd",
		DefaultExec: []string{"/bin/echo"},
		RunCommand: func(context.Context, []string) error {
			return errors.New("boom")
		},
	}, nil)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "service_exec") {
		t.Fatalf("expected phase error, got %v", err)
	}
}

func TestRunValidatesStartupContractServiceMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "startup.json")
	payload := contract.Document{
		Version:    contract.SupportedVersion,
		Service:    "webpa",
		Deployment: "edge",
		Interfaces: []contract.InterfaceBinding{{Role: "mgmt", Name: "eth0", MAC: "02:10:00:00:00:07"}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	t.Setenv("VCPE_STARTUP_CONTRACT", path)

	err = Run(context.Background(), Config{
		Service:     "bng",
		DefaultExec: []string{"/bin/echo", "ok"},
		RunCommand:  func(context.Context, []string) error { return nil },
	}, nil)
	if err == nil {
		t.Fatal("expected startup contract mismatch failure")
	}
	if !strings.Contains(err.Error(), "service mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
