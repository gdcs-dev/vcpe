package servicecmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		Service:     "webpa",
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

func TestRunAcceptsNamedGatewayStartupContract(t *testing.T) {
	path := writeRadioContract(t, "root")
	t.Setenv("VCPE_STARTUP_CONTRACT", path)
	t.Setenv("SERVICE_NAME", "root")
	err := Run(context.Background(), Config{
		Service:      "gateway",
		DefaultExec:  []string{"/bin/echo", "ok"},
		RunCommand:   func(context.Context, []string) error { return nil },
		DeviceExists: func(string) bool { return true },
	}, nil)
	if err != nil {
		t.Fatalf("run named Gateway: %v", err)
	}
}

func TestRunRejectsOtherNamedGatewayStartupContract(t *testing.T) {
	path := writeRadioContract(t, "remote")
	t.Setenv("VCPE_STARTUP_CONTRACT", path)
	t.Setenv("SERVICE_NAME", "root")
	err := Run(context.Background(), Config{
		Service:     "gateway",
		DefaultExec: []string{"/bin/echo", "ok"},
		RunCommand:  func(context.Context, []string) error { return nil },
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "service mismatch") {
		t.Fatalf("expected service mismatch, got %v", err)
	}
}

func TestRunWaitsForStartupContractRadios(t *testing.T) {
	path := writeRadioContract(t, "gateway")
	t.Setenv("VCPE_STARTUP_CONTRACT", path)
	checks := 0
	err := Run(context.Background(), Config{
		Service:                "gateway",
		DefaultExec:            []string{"/bin/echo", "ok"},
		RunCommand:             func(context.Context, []string) error { return nil },
		RadioReadyTimeout:      100 * time.Millisecond,
		RadioReadyPollInterval: time.Millisecond,
		DeviceExists: func(name string) bool {
			if name != "wlan0" {
				t.Fatalf("device = %q", name)
			}
			checks++
			return checks > 1
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if checks < 2 {
		t.Fatalf("device checks = %d, want at least 2", checks)
	}
}

func TestRunReportsMissingStartupContractRadios(t *testing.T) {
	path := writeRadioContract(t, "gateway")
	t.Setenv("VCPE_STARTUP_CONTRACT", path)
	err := Run(context.Background(), Config{
		Service:                "gateway",
		DefaultExec:            []string{"/bin/echo", "ok"},
		RunCommand:             func(context.Context, []string) error { return nil },
		RadioReadyTimeout:      5 * time.Millisecond,
		RadioReadyPollInterval: time.Millisecond,
		DeviceExists:           func(string) bool { return false },
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "interface_ready") || !strings.Contains(err.Error(), "missing: wlan0") {
		t.Fatalf("error = %v", err)
	}
}

func writeRadioContract(t *testing.T, service string) string {
	t.Helper()
	document := contract.Document{
		Version:    contract.SupportedVersion,
		Service:    service,
		Deployment: "edge",
		Radios: []contract.RadioBinding{{
			Name: "home-ap", Device: "wlan0", MAC: "02:00:00:00:00:01", Mode: "ap",
			Network: "home", SSID: "vcpe-lab", Channel: 1, Bridge: "brlan0",
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "startup.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
