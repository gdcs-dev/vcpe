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
