package plan_test

import (
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

func TestCanonicalMACIsStableAndLocallyAdministered(t *testing.T) {
	a := plan.CanonicalMAC("edge", "bng", "wan", 0)
	b := plan.CanonicalMAC("edge", "bng", "wan", 0)
	if a != b {
		t.Fatalf("CanonicalMAC is not stable: %q != %q", a, b)
	}
	if !strings.HasPrefix(a, "02:") {
		t.Fatalf("expected locally-administered 02: prefix, got %q", a)
	}
	if len(a) != len("02:00:00:00:00:00") {
		t.Fatalf("unexpected MAC length: %q", a)
	}
}

func TestCanonicalMACReplicaIndexDiffers(t *testing.T) {
	base := plan.CanonicalMAC("edge", "bng", "wan", 0)
	idx1 := plan.CanonicalMAC("edge", "bng", "wan", 1)
	idx2 := plan.CanonicalMAC("edge", "bng", "wan", 2)
	if base == idx1 || idx1 == idx2 || base == idx2 {
		t.Fatalf("expected distinct replica MACs, got %q,%q,%q", base, idx1, idx2)
	}
}

func TestCanonicalMACVariesByKeyComponent(t *testing.T) {
	if plan.CanonicalMAC("edge", "bng", "wan", 0) == plan.CanonicalMAC("edge", "bng", "lan", 0) {
		t.Fatal("expected different roles to yield different MACs")
	}
	if plan.CanonicalMAC("edge", "bng", "wan", 0) == plan.CanonicalMAC("edge", "gateway", "wan", 0) {
		t.Fatal("expected different services to yield different MACs")
	}
	if plan.CanonicalMAC("a", "bng", "wan", 0) == plan.CanonicalMAC("b", "bng", "wan", 0) {
		t.Fatal("expected different deployments to yield different MACs")
	}
}

func TestDeriveBridgeNameShortNameUnchanged(t *testing.T) {
	name, truncated := plan.DeriveBridgeName("edge", "wan")
	if truncated {
		t.Fatalf("did not expect truncation for %q", name)
	}
	if name != "edge-wan" {
		t.Fatalf("expected edge-wan, got %q", name)
	}
}

func TestDeriveBridgeNameTruncatesOverflow(t *testing.T) {
	name, truncated := plan.DeriveBridgeName("very-long-deployment-name", "lan-port-1")
	if !truncated {
		t.Fatal("expected overflow to be flagged as truncated")
	}
	if len(name) > 15 {
		t.Fatalf("expected name within IFNAMSIZ (15), got %d (%q)", len(name), name)
	}
	// Deterministic: same input yields same output.
	again, _ := plan.DeriveBridgeName("very-long-deployment-name", "lan-port-1")
	if name != again {
		t.Fatalf("expected deterministic truncation, got %q != %q", name, again)
	}
}

func TestRadioIdentityIsStableBoundedAndDomainSeparated(t *testing.T) {
	name := plan.RadioManagerName("edge", "gateway", 0, "home-ap")
	if name != plan.RadioManagerName("edge", "gateway", 0, "home-ap") {
		t.Fatal("radio manager name is not stable")
	}
	if !strings.HasPrefix(name, "vcpe-") || len(name) > 31 {
		t.Fatalf("manager name %q violates manager contract", name)
	}
	mac := plan.CanonicalRadioMAC("edge", "gateway", 0, "home-ap")
	if mac != plan.CanonicalRadioMAC("edge", "gateway", 0, "home-ap") {
		t.Fatal("radio MAC is not stable")
	}
	if mac == plan.CanonicalMAC("edge", "gateway", "home-ap", 0) {
		t.Fatalf("radio MAC %q was not domain-separated from interface identity", mac)
	}
	if mac[1] != '2' && mac[1] != '6' && mac[1] != 'a' && mac[1] != 'e' {
		t.Fatalf("radio MAC %q is not locally administered unicast", mac)
	}
}

func TestRadioIdentityVariesByReplicaAndLogicalName(t *testing.T) {
	base := plan.RadioManagerName("edge", "gateway", 0, "home")
	if base == plan.RadioManagerName("edge", "gateway", 1, "home") || base == plan.RadioManagerName("edge", "gateway", 0, "guest") {
		t.Fatal("expected distinct radio manager names")
	}
	if plan.CanonicalRadioMAC("edge", "gateway", 0, "home") == plan.CanonicalRadioMAC("edge", "gateway", 1, "home") {
		t.Fatal("expected distinct replica radio MACs")
	}
}

func TestVAPIdentityIsStablePerSlotAndReplica(t *testing.T) {
	if got, want := plan.CanonicalVAPBSSID("edge", "gateway", 0, "home", 0), plan.CanonicalRadioMAC("edge", "gateway", 0, "home"); got != want {
		t.Fatalf("slot 0 BSSID = %q, want primary radio MAC %q", got, want)
	}
	devices := map[string]bool{}
	bssids := map[string]bool{plan.CanonicalRadioMAC("edge", "gateway", 0, "home"): true}
	for slot := 1; slot < 8; slot++ {
		device := plan.VAPDeviceName("edge", "gateway", 0, "home", slot)
		bssid := plan.CanonicalVAPBSSID("edge", "gateway", 0, "home", slot)
		if device != plan.VAPDeviceName("edge", "gateway", 0, "home", slot) || bssid != plan.CanonicalVAPBSSID("edge", "gateway", 0, "home", slot) {
			t.Fatalf("slot %d identity is not stable", slot)
		}
		if len(device) > 15 || devices[device] {
			t.Fatalf("slot %d device %q exceeds IFNAMSIZ or repeats", slot, device)
		}
		if bssids[bssid] || (bssid[1] != '2' && bssid[1] != '6' && bssid[1] != 'a' && bssid[1] != 'e') {
			t.Fatalf("slot %d BSSID %q repeats or is not locally administered unicast", slot, bssid)
		}
		devices[device], bssids[bssid] = true, true
	}
	if plan.VAPDeviceName("edge", "gateway", 0, "home", 1) == plan.VAPDeviceName("edge", "gateway", 1, "home", 1) ||
		plan.CanonicalVAPBSSID("edge", "gateway", 0, "home", 1) == plan.CanonicalVAPBSSID("edge", "gateway", 1, "home", 1) {
		t.Fatal("replica scaling reused a secondary VAP identity")
	}
}

func TestContainerNameUsesStableIndexedIdentity(t *testing.T) {
	if got := plan.InstanceName("gateway", 0); got != "gateway-1" {
		t.Fatalf("InstanceName() = %q", got)
	}
	if got := plan.ContainerName("edge", "gateway", 0); got != "edge-gateway-1" {
		t.Fatalf("ContainerName() = %q", got)
	}
}
