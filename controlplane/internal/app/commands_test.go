package app

import (
	"os"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

func TestStatusOutputModes(t *testing.T) {
	stateRoot := t.TempDir()

	human, err := executeLocal(Options{Command: "status", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("human status: %v", err)
	}
	if !strings.HasPrefix(human.Message, "vCPE status") {
		t.Fatalf("expected human status, got %q", human.Message)
	}

	jsonOut, err := executeLocal(Options{Command: "status", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("json status: %v", err)
	}
	for _, key := range []string{"\"metrics\"", "\"desired\"", "\"planned\"", "\"observed\"", "\"runtimeInitDiagnostics\""} {
		if !strings.Contains(jsonOut.Message, key) {
			t.Fatalf("expected %s in json status, got %q", key, jsonOut.Message)
		}
	}
}

func TestLogsOutputModes(t *testing.T) {
	stateRoot := t.TempDir()

	human, err := executeLocal(Options{Command: "logs", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(human.Message, "logs unavailable without --name") {
		t.Fatalf("expected usage hint, got %q", human.Message)
	}

	jsonOut, err := executeLocal(Options{Command: "logs", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("logs json: %v", err)
	}
	if !strings.Contains(jsonOut.Message, "\"timeline\"") || !strings.Contains(jsonOut.Message, "\"runtimeInitDiagnostics\"") {
		t.Fatalf("expected timeline + diagnostics, got %q", jsonOut.Message)
	}
}

func TestServiceStatusRoutesThroughCanonicalStatus(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "service", CommandArgs: []string{"bng", "status"}, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("service status: %v", err)
	}
	if !strings.Contains(resp.Message, "service=bng") || !strings.Contains(resp.Message, "vCPE status") {
		t.Fatalf("expected service marker + canonical status, got %q", resp.Message)
	}
}

func TestServiceLogsWithName(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "service", CommandArgs: []string{"bng", "logs"}, Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("service logs: %v", err)
	}
	if !strings.Contains(resp.Message, "deployment=edge") || !strings.Contains(resp.Message, "service=bng") {
		t.Fatalf("expected deployment+service markers, got %q", resp.Message)
	}
}

func TestServiceJSONEmbedsServiceField(t *testing.T) {
	stateRoot := t.TempDir()

	statusResp, err := executeLocal(Options{Command: "service", CommandArgs: []string{"bng", "status"}, OutputJSON: true, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("service status json: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(statusResp.Message), "{") || !strings.Contains(statusResp.Message, "\"service\": \"bng\"") {
		t.Fatalf("expected valid json with service field, got %q", statusResp.Message)
	}

	logsResp, err := executeLocal(Options{Command: "service", CommandArgs: []string{"bng", "logs"}, OutputJSON: true, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("service logs json: %v", err)
	}
	if !strings.Contains(logsResp.Message, "\"service\": \"bng\"") || !strings.Contains(logsResp.Message, "\"timeline\"") {
		t.Fatalf("expected json with service + timeline, got %q", logsResp.Message)
	}
}

func TestConfigShow(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "config", CommandArgs: []string{"show"}, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	if !strings.Contains(resp.Message, "VCPE_STATE_ROOT=") {
		t.Fatalf("expected state root in config show, got %q", resp.Message)
	}
}

func TestStateResetReinitializes(t *testing.T) {
	stateRoot := t.TempDir()

	// Seed a lease, then reset, then confirm it is cleared.
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"}}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	ps.Close()

	resp, err := executeLocal(Options{Command: "state", CommandArgs: []string{"reset"}, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("state reset: %v", err)
	}
	if !strings.Contains(resp.Message, "state reset complete") {
		t.Fatalf("expected reset confirmation, got %q", resp.Message)
	}

	ps2, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("reopen persist: %v", err)
	}
	defer ps2.Close()
	leases, err := ps2.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected reset to clear leases, got %#v", leases)
	}
}

func TestDownUnknownNameFails(t *testing.T) {
	stateRoot := t.TempDir()
	_, err := executeLocal(Options{Command: "down", Name: "ghost", StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "unknown deployment") {
		t.Fatalf("expected unknown deployment error, got %v", err)
	}
}

func TestPreflightRejectsUnsupportedType(t *testing.T) {
	stateRoot := t.TempDir()
	dir := t.TempDir()
	path := dir + "/m.yaml"
	content := "apiVersion: vcpe.dev/v1\n" +
		"kind: Deployment\n" +
		"metadata: { name: edge }\n" +
		"spec:\n" +
		"  networks:\n" +
		"    - role: wan\n" +
		"      ipv4: { cidr: 10.0.0.0/24 }\n" +
		"  services:\n" +
		"    - name: ghost\n" +
		"      type: not-a-real-type\n" +
		"      replicas: 1\n" +
		"      image: { repository: ghcr.io/x/ghost }\n" +
		"      interfaces:\n" +
		"        - { role: wan }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := executeLocal(Options{Command: "plan", ManifestPath: path, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestClassifyDisruptiveCIDRChange(t *testing.T) {
	stateRoot := t.TempDir()
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	defer ps.Close()
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"}}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}

	manifestPath := writeV1Manifest(t, "edge")
	// Mutate the wan CIDR so it differs from the seeded lease.
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	mutated := strings.Replace(string(data), "10.200.0.0/24", "10.250.0.0/24", 1)
	if err := os.WriteFile(manifestPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	doc, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	disruptive, reasons, err := classifyDisruptive(ps, doc)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !disruptive {
		t.Fatal("expected disruptive classification for CIDR change")
	}
	if len(reasons) == 0 || !strings.Contains(strings.Join(reasons, "\n"), "CIDR changes") {
		t.Fatalf("expected CIDR change reason, got %v", reasons)
	}
}

// TestBuildReportsSummary exercises runBuild end-to-end without a container
// runtime by activating the noopImageBackend via VCPE_SKIP_IMAGE=1.
func TestBuildReportsSummary(t *testing.T) {
	t.Setenv("VCPE_SKIP_IMAGE", "1")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")

	resp, err := executeLocal(Options{Command: "build", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.HasPrefix(resp.Message, "build complete for deployment") {
		t.Fatalf("expected build summary, got %q", resp.Message)
	}
}

// TestPlanShowsNetworksAndServices asserts that plan output contains the
// "networks:" and "services:" counts.
func TestPlanShowsNetworksAndServices(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")

	resp, err := executeLocal(Options{Command: "plan", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(resp.Message, "networks:") {
		t.Fatalf("expected networks in plan output, got %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "services:") {
		t.Fatalf("expected services in plan output, got %q", resp.Message)
	}
}

// TestPlanDisruptiveGate seeds a WAN CIDR lease and uses a manifest with a
// different WAN CIDR, expecting the plan to report "disruptive: yes".
func TestPlanDisruptiveGate(t *testing.T) {
	stateRoot := t.TempDir()
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{
		{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"},
	}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	ps.Close()

	manifestPath := writeV1Manifest(t, "edge")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	// Replace the entire WAN prefix so gateway and pool remain consistent with
	// the new subnet while only the CIDR differs from the seeded lease.
	mutated := strings.ReplaceAll(string(data), "10.200.0.", "10.250.0.")
	if err := os.WriteFile(manifestPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	resp, err := executeLocal(Options{Command: "plan", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.Contains(resp.Message, "disruptive: yes") {
		t.Fatalf("expected disruptive: yes in plan output, got %q", resp.Message)
	}
}

// TestDownClearsLeases seeds a lease and a desired snapshot, then calls down
// and asserts that all IPAM leases are cleared.
func TestDownClearsLeases(t *testing.T) {
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	stateRoot := t.TempDir()

	// Seed a lease so down has something to tear down.
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	if err := ps.ReplaceCustomerLeases("edge", []persist.IPAMLease{
		{CustomerID: "edge", Role: "wan", CIDR: "10.200.0.0/24"},
	}); err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	// Stamp a minimal desired snapshot so CustomerExists returns true.
	if err := ps.SaveDesiredSnapshot("edge", []byte("{}")); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	ps.Close()

	_, err = executeLocal(Options{Command: "down", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("down: %v", err)
	}

	ps2, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("reopen persist: %v", err)
	}
	defer ps2.Close()
	leases, err := ps2.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected leases cleared after down, got %#v", leases)
	}
}

// TestLogsWithNameShowsDeployment asserts that logs with --name includes the
// deployment name in the output.
func TestLogsWithNameShowsDeployment(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "logs", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(resp.Message, "deployment=edge") {
		t.Fatalf("expected deployment=edge in logs output, got %q", resp.Message)
	}
}
