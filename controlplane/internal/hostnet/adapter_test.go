package hostnet

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDelegatedHostnetModeAutoDetectsOnNonLinuxWithPodmanMachine(t *testing.T) {
	origCheck := podmanMachineInfoCheck
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		podmanMachineInfoCheck = origCheck
		runtimeGOOS = origGOOS
	})
	t.Setenv("VCPE_HOSTNET_DELEGATED", "")

	runtimeGOOS = "darwin"
	podmanMachineInfoCheck = func() error { return nil }

	if !delegatedHostnetMode(true) {
		t.Fatalf("expected delegated hostnet mode to auto-enable on non-linux with podman machine")
	}
}

func TestDelegatedHostnetModeDisabledOnLinuxWithoutEnvOverride(t *testing.T) {
	origCheck := podmanMachineInfoCheck
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		podmanMachineInfoCheck = origCheck
		runtimeGOOS = origGOOS
	})
	t.Setenv("VCPE_HOSTNET_DELEGATED", "")

	runtimeGOOS = "linux"
	podmanMachineInfoCheck = func() error { return nil }

	if delegatedHostnetMode(true) {
		t.Fatalf("expected delegated hostnet mode disabled by default on linux")
	}
}

func TestDelegatedHostnetModeHonorsEnvOverride(t *testing.T) {
	origCheck := podmanMachineInfoCheck
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		podmanMachineInfoCheck = origCheck
		runtimeGOOS = origGOOS
	})
	runtimeGOOS = "linux"
	podmanMachineInfoCheck = func() error { return os.ErrNotExist }
	t.Setenv("VCPE_HOSTNET_DELEGATED", "1")

	if !delegatedHostnetMode(false) {
		t.Fatalf("expected delegated hostnet mode with env override")
	}
}

func TestPreflightChecksCapabilities(t *testing.T) {
	adapter := Adapter{capabilities: func() CapabilityReport {
		return CapabilityReport{HasIPCmd: true, HasIPTablesCmd: false, HasPodmanCmd: true, CanConfigure: false}
	}}
	err := adapter.Preflight([]Intent{{Role: "wan", Bridge: "wan-7", RequiresNAT: true}})
	if err == nil {
		t.Fatalf("expected preflight failure for missing iptables/capabilities")
	}
}

func TestPreflightPassesWhenCapabilitiesAvailable(t *testing.T) {
	adapter := Adapter{capabilities: func() CapabilityReport {
		return CapabilityReport{HasIPCmd: true, HasIPTablesCmd: true, HasPodmanCmd: true, CanConfigure: true}
	}}
	if err := adapter.Preflight([]Intent{{Role: "wan", Bridge: "wan-7", RequiresNAT: true, RequiresFirewall: true}}); err != nil {
		t.Fatalf("preflight: %v", err)
	}
}

func TestPreflightFailsWhenIPCommandMissing(t *testing.T) {
	adapter := Adapter{capabilities: func() CapabilityReport {
		return CapabilityReport{HasIPCmd: false, HasIPTablesCmd: true, HasPodmanCmd: true, CanConfigure: true}
	}}
	if err := adapter.Preflight([]Intent{{Role: "mgmt", Bridge: "mgmt"}}); err == nil {
		t.Fatalf("expected preflight failure for missing ip command")
	}
}

func TestPreflightDelegatedRequiresPodman(t *testing.T) {
	adapter := Adapter{capabilities: func() CapabilityReport {
		return CapabilityReport{Delegated: true, HasPodmanCmd: false}
	}}
	if err := adapter.Preflight([]Intent{{Role: "wan", Bridge: "wan-7", RequiresNAT: true}}); err == nil {
		t.Fatalf("expected delegated preflight failure without podman")
	}
}

func TestPreflightDelegatedPassesWithoutLocalIPTables(t *testing.T) {
	adapter := Adapter{capabilities: func() CapabilityReport {
		return CapabilityReport{Delegated: true, HasPodmanCmd: true, HasIPCmd: false, HasIPTablesCmd: false, CanConfigure: false}
	}}
	if err := adapter.Preflight([]Intent{{Role: "wan", Bridge: "wan-7", RequiresNAT: true, RequiresFirewall: true}}); err != nil {
		t.Fatalf("expected delegated preflight success: %v", err)
	}
}

func TestHostNetworkValidation(t *testing.T) {
	adapter := New()
	if err := adapter.EnsureBridge(context.Background(), ""); err == nil {
		t.Fatalf("expected bridge validation failure")
	}
	if err := adapter.EnsureNAT(context.Background(), ""); err == nil {
		t.Fatalf("expected nat validation failure")
	}
	if err := adapter.EnsureFirewallForward(context.Background(), ""); err == nil {
		t.Fatalf("expected firewall validation failure")
	}
}

func TestReconcileAppliesIntentsWithNATAndFirewall(t *testing.T) {
	commands := []string{}
	runner := func(_ context.Context, name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if name == "ip" && len(args) >= 3 && args[0] == "link" && args[1] == "show" {
			return fmt.Errorf("missing bridge")
		}
		if name == "iptables" && len(args) >= 2 && args[0] == "-t" && args[1] == "nat" && len(args) >= 3 && args[2] == "-C" {
			return fmt.Errorf("missing nat rule")
		}
		if name == "iptables" && len(args) >= 1 && args[0] == "-C" {
			return fmt.Errorf("missing firewall rule")
		}
		return nil
	}
	adapter := Adapter{capabilities: func() CapabilityReport {
		return CapabilityReport{HasIPCmd: true, HasIPTablesCmd: true, CanConfigure: true}
	}, runCmd: runner}

	intents := []Intent{{Role: "wan", Bridge: "wan-7", RequiresNAT: true, RequiresFirewall: true}}
	roleCIDRs := map[string]string{"wan": "10.7.200.0/24"}
	if err := adapter.Reconcile(context.Background(), intents, roleCIDRs); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	joined := strings.Join(commands, "\n")
	for _, expected := range []string{
		"ip link show wan-7",
		"ip link add wan-7 type bridge",
		"ip link set wan-7 up",
		"iptables -t nat -C POSTROUTING -s 10.7.200.0/24 -j MASQUERADE",
		"iptables -t nat -A POSTROUTING -s 10.7.200.0/24 -j MASQUERADE",
		"iptables -C FORWARD -i wan-7 -j ACCEPT",
		"iptables -A FORWARD -i wan-7 -j ACCEPT",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected command %q in run log:\n%s", expected, joined)
		}
	}
}

func TestReconcileRequiresCIDRForNATRole(t *testing.T) {
	adapter := Adapter{runCmd: func(_ context.Context, _ string, _ ...string) error { return nil }}
	err := adapter.Reconcile(context.Background(), []Intent{{Role: "wan", Bridge: "wan-7", RequiresNAT: true}}, map[string]string{})
	if err == nil {
		t.Fatalf("expected missing cidr error")
	}
	if !strings.Contains(err.Error(), "missing source CIDR") {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
}
