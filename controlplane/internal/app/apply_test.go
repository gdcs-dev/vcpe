package app

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/health"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
)

// writeV1Manifest writes a minimal valid vcpe.dev/v1 Deployment manifest with a
// single bng service and returns its path. The name is the deployment identity.
func writeV1Manifest(t *testing.T, name string) string {
	t.Helper()
	return writeV1ManifestWithServices(t, name, []string{"bng"})
}

func writeV1ManifestWithServices(t *testing.T, name string, services []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")

	var b strings.Builder
	b.WriteString("apiVersion: vcpe.dev/v1\n")
	b.WriteString("kind: Deployment\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: " + name + "\n")
	b.WriteString("  labels:\n")
	b.WriteString("    customer: \"7\"\n")
	b.WriteString("spec:\n")
	b.WriteString("  networks:\n")
	b.WriteString("    - role: mgmt\n")
	b.WriteString("      ipv4: { cidr: 10.10.10.0/24, gateway: 10.10.10.1, pool: { start: 10.10.10.10, end: 10.10.10.250 } }\n")
	b.WriteString("    - role: wan\n")
	b.WriteString("      nat: true\n")
	b.WriteString("      ipv4: { cidr: 10.200.0.0/24, gateway: 10.200.0.1, pool: { start: 10.200.0.10, end: 10.200.0.250 } }\n")
	b.WriteString("    - role: cm\n")
	b.WriteString("      ipv4: { cidr: 10.201.0.0/24, gateway: 10.201.0.1, pool: { start: 10.201.0.10, end: 10.201.0.250 } }\n")
	b.WriteString("  services:\n")
	for _, svc := range services {
		b.WriteString("    - name: " + svc + "\n")
		b.WriteString("      type: " + svc + "\n")
		b.WriteString("      replicas: 1\n")
		b.WriteString("      image: { repository: ghcr.io/gdcs-dev/" + svc + ", tag: dev }\n")
		b.WriteString("      interfaces:\n")
		if svc == "gateway" {
			// Gateway requires explicit device names on all interfaces.
			b.WriteString("        - { role: mgmt, device: eth0 }\n")
			b.WriteString("        - { role: wan, device: erouter0, defaultRoute: true }\n")
			b.WriteString("        - { role: cm, device: wan0 }\n")
		} else {
			b.WriteString("        - { role: mgmt }\n")
			b.WriteString("        - { role: wan, defaultRoute: true }\n")
			b.WriteString("        - { role: cm }\n")
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func phaseLog(t *testing.T, ps *persist.Store, opID string) string {
	t.Helper()
	phases, err := ps.OperationPhases(opID)
	if err != nil {
		t.Fatalf("operation phases: %v", err)
	}
	joined := make([]string, 0, len(phases))
	for _, p := range phases {
		joined = append(joined, p.Phase+":"+p.Status)
	}
	return strings.Join(joined, "|")
}

func TestApplySucceedsWithoutRuntime(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")

	resp, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(resp.Message, "applied deployment \"edge\"") {
		t.Fatalf("unexpected apply message: %q", resp.Message)
	}

	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	defer ps.Close()

	leases, err := ps.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) == 0 {
		t.Fatal("expected IPAM leases after successful apply")
	}

	timeline, err := ps.RecentOperations(1)
	if err != nil {
		t.Fatalf("recent operations: %v", err)
	}
	if len(timeline) == 0 {
		t.Fatal("expected operation timeline entry")
	}
	log := phaseLog(t, ps, timeline[0].OperationID)
	for _, marker := range []string{"preflight:succeeded", "images:succeeded", "allocation:succeeded", "render:succeeded", "runtime-init:bng:succeeded", "lifecycle:succeeded"} {
		if !strings.Contains(log, marker) {
			t.Fatalf("expected phase %s in %s", marker, log)
		}
	}

	// Startup-contract artifacts in both operation and deployment dirs.
	opContract := filepath.Join(stateRoot, "artifacts", "v1", "operations", timeline[0].OperationID, "runtime", "startup-contracts", "bng.json")
	if _, err := os.Stat(opContract); err != nil {
		t.Fatalf("expected operation startup contract: %v", err)
	}
	depContract := filepath.Join(stateRoot, "artifacts", "v1", "deployments", "edge", "runtime", "startup-contracts", "bng.json")
	if _, err := os.Stat(depContract); err != nil {
		t.Fatalf("expected deployment startup contract: %v", err)
	}
}

func TestApplyPersistsStableHealthEndpointBeforeLifecycle(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")

	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open store after first apply: %v", err)
	}
	first, err := store.ListHealthEndpoints("edge")
	if err != nil {
		t.Fatalf("list endpoints after first apply: %v", err)
	}
	if len(first) != 1 || first[0].Service != "bng" || first[0].Replica != 0 {
		t.Fatalf("unexpected first endpoint set: %#v", first)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open store after second apply: %v", err)
	}
	defer store.Close()
	second, err := store.ListHealthEndpoints("edge")
	if err != nil {
		t.Fatalf("list endpoints after second apply: %v", err)
	}
	if len(second) != 1 || second[0] != first[0] {
		t.Fatalf("endpoint changed across non-disruptive apply: first=%#v second=%#v", first, second)
	}
}

func TestApplySkipsPrivateHealthNetworkWhenPodmanIPAMIsAvailable(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")

	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	operations, err := store.RecentOperations(1)
	store.Close()
	if err != nil || len(operations) != 1 {
		t.Fatalf("operations = %#v, err = %v", operations, err)
	}
	composePath := filepath.Join(stateRoot, "artifacts", "v1", "operations", operations[0].OperationID, "runtime", "bng", "compose.yaml")
	compose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read rendered compose: %v", err)
	}
	if strings.Contains(string(compose), "zz-health:") {
		t.Fatalf("rendered Compose unexpectedly added private health network:\n%s", compose)
	}
}

func TestApplyReservesGatewayHealthEndpointOnlyWhenHealthUpstreamDeclared(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		healthUpstream string
		wantEndpoints  int
	}{
		{name: "undeclared", wantEndpoints: 0},
		{name: "declared", healthUpstream: ", healthUpstream: true", wantEndpoints: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			stateRoot := t.TempDir()
			manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
			content := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge\nspec:\n  networks:\n    - role: wan\n      ipamDriver: none\n      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1 }\n  services:\n    - name: gateway\n      type: gateway\n      replicas: 1\n      image: { repository: ghcr.io/gdcs-dev/gateway, tag: dev }\n      interfaces:\n        - { role: wan, device: erouter0, ipv4: \"10.7.200.10\"" + testCase.healthUpstream + " }\n"
			if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
			t.Setenv("VCPE_SKIP_RUNTIME", "1")
			if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
				t.Fatalf("apply: %v", err)
			}
			store, err := persist.Open(stateRoot)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			defer store.Close()
			endpoints, err := store.ListHealthEndpoints("edge")
			if err != nil {
				t.Fatalf("list health endpoints: %v", err)
			}
			if len(endpoints) != testCase.wantEndpoints {
				t.Fatalf("endpoints = %#v, want %d", endpoints, testCase.wantEndpoints)
			}
			response, err := executeLocal(Options{Command: "status", Name: "edge", StateRoot: stateRoot})
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if testCase.wantEndpoints == 0 && !strings.Contains(response.Message, "health gateway/0: not-configured") {
				t.Fatalf("expected not-configured gateway health, got: %s", response.Message)
			}
		})
	}
}

func TestApplyReservesGenericHealthEndpointOnlyWhenConfigured(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		healthConfig  string
		wantEndpoints int
	}{
		{name: "unconfigured", wantEndpoints: 0},
		{name: "configured", healthConfig: "        health:\n          command:\n            command: test -f /ready\n          timeoutSeconds: 2\n", wantEndpoints: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			stateRoot := t.TempDir()
			manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
			content := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge\nspec:\n  networks:\n    - role: mgmt\n      ipv4: { cidr: 10.10.10.0/24, gateway: 10.10.10.1, pool: { start: 10.10.10.10, end: 10.10.10.250 } }\n  services:\n    - name: client\n      type: generic-container\n      replicas: 1\n      image: { repository: docker.io/library/alpine, tag: \"3.19\" }\n      interfaces:\n        - { role: mgmt }\n      config:\n" + testCase.healthConfig
			if testCase.healthConfig == "" {
				content += "        command: [sleep, infinity]\n"
			}
			if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
			t.Setenv("VCPE_SKIP_RUNTIME", "1")
			if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
				t.Fatalf("apply: %v", err)
			}
			store, err := persist.Open(stateRoot)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			defer store.Close()
			endpoints, err := store.ListHealthEndpoints("edge")
			if err != nil {
				t.Fatalf("list health endpoints: %v", err)
			}
			if len(endpoints) != testCase.wantEndpoints {
				t.Fatalf("endpoints = %#v, want %d", endpoints, testCase.wantEndpoints)
			}
		})
	}
}

func TestAppliedDeploymentStatusCollectsGenericHealthOverHTTP(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
	manifest := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge\nspec:\n  networks:\n    - role: mgmt\n      ipv4: { cidr: 10.10.10.0/24, gateway: 10.10.10.1, pool: { start: 10.10.10.10, end: 10.10.10.250 } }\n  services:\n    - name: client\n      type: generic-container\n      replicas: 1\n      image: { repository: docker.io/library/alpine, tag: \"3.19\" }\n      interfaces:\n        - { role: mgmt }\n      config:\n        health:\n          command:\n            command: test -f /ready\n          timeoutSeconds: 2\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	store, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	for index := 0; index < 3; index++ {
		if _, err := store.ReserveHealthEndpoint("seed", "service", index); err != nil {
			store.Close()
			t.Fatalf("reserve seed endpoint %d: %v", index, err)
		}
	}
	store.Close()
	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	store, err = persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	endpoints, err := store.ListHealthEndpoints("edge")
	store.Close()
	if err != nil || len(endpoints) != 1 {
		t.Fatalf("endpoints = %#v, err = %v", endpoints, err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(endpoints[0].HostPort))
	if err != nil {
		t.Fatalf("listen on reserved health port: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(health.Response{SchemaVersion: health.SchemaVersion, Status: health.StatusHealthy, ObservedAt: time.Now().UTC(), Checks: []health.Check{{Name: "configured", Status: health.StatusHealthy}}})
	})}
	defer server.Close()
	go server.Serve(listener)

	response, err := executeLocal(Options{Command: "status", Name: "edge", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(response.Message, "health client/0: healthy") {
		t.Fatalf("status did not collect health over HTTP: %s", response.Message)
	}
}

func TestApplyRollsBackOnRenderFailure(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	t.Setenv("VCPE_FAIL_PHASE", "render")

	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err == nil {
		t.Fatal("expected apply to fail at render phase")
	}

	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	defer ps.Close()

	leases, err := ps.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected rollback to clear leases, got %#v", leases)
	}

	timeline, err := ps.RecentOperations(1)
	if err != nil {
		t.Fatalf("recent operations: %v", err)
	}
	log := phaseLog(t, ps, timeline[0].OperationID)
	for _, marker := range []string{"allocation:succeeded", "render:failed", "rollback:succeeded"} {
		if !strings.Contains(log, marker) {
			t.Fatalf("expected phase %s in %s", marker, log)
		}
	}
}

func TestApplyRollsBackOnRuntimeInitVerifyFailure(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1Manifest(t, "edge")
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	t.Setenv("VCPE_FAIL_PHASE", "runtime-init-verify")

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err == nil {
			t.Fatalf("expected apply failure on attempt %d", attempt+1)
		}
	}

	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	defer ps.Close()

	leases, err := ps.ListIPAMLeases()
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected rollback to clear leases, got %#v", leases)
	}

	timeline, err := ps.RecentOperations(1)
	if err != nil {
		t.Fatalf("recent operations: %v", err)
	}
	log := phaseLog(t, ps, timeline[0].OperationID)
	for _, marker := range []string{"runtime-init-verify:failed", "rollback:succeeded"} {
		if !strings.Contains(log, marker) {
			t.Fatalf("expected phase %s in %s", marker, log)
		}
	}
}

func TestApplyMultiServiceCanonicalPath(t *testing.T) {
	stateRoot := t.TempDir()
	manifestPath := writeV1ManifestWithServices(t, "edge", []string{"bng", "gateway"})
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")

	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("apply with bng+gateway failed: %v", err)
	}
}

func TestApplyEnforcesMaxActiveDeployments(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")

	// First deployment with cap 1 succeeds.
	first := writeV1ManifestWithCap(t, "edge-a", 1)
	if _, err := executeLocal(Options{Command: "apply", ManifestPath: first, StateRoot: stateRoot}); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	// Second distinct deployment exceeds the cap.
	second := writeV1ManifestWithCap(t, "edge-b", 1)
	_, err := executeLocal(Options{Command: "apply", ManifestPath: second, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "maxActiveDeployments") {
		t.Fatalf("expected cap violation, got %v", err)
	}
}

func writeV1ManifestWithCap(t *testing.T, name string, cap int) string {
	t.Helper()
	base := writeV1Manifest(t, name)
	data, err := os.ReadFile(base)
	if err != nil {
		t.Fatalf("read base manifest: %v", err)
	}
	// Insert the cap into spec by appending a top-level spec field. Simplest is
	// to rewrite with the field present.
	out := strings.Replace(string(data), "spec:\n", "spec:\n  maxActiveDeployments: "+itoa(cap)+"\n", 1)
	if err := os.WriteFile(base, []byte(out), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}
	return base
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestApplyAllowDisruptiveGate seeds a WAN lease, writes a manifest with a
// different WAN CIDR, and confirms that apply rejects it when AllowDisruptive
// is not set. The error message should mention "disruptive".
func TestApplyAllowDisruptiveGate(t *testing.T) {
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
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
	// Stamp a desired snapshot so CustomerExists returns true (otherwise down
	// path could short-circuit differently).
	if err := ps.SaveDesiredSnapshot("edge", []byte("{}")); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	ps.Close()

	manifestPath := writeV1Manifest(t, "edge")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	// Replace entire WAN prefix so gateway/pool remain consistent with new
	// subnet. The CIDR itself differs from the seeded lease (disruptive).
	mutated := strings.ReplaceAll(string(data), "10.200.0.", "10.250.0.")
	if err := os.WriteFile(manifestPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	_, err = executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err == nil {
		t.Fatal("expected apply to fail due to disruptive change")
	}
	if !strings.Contains(err.Error(), "disruptive") {
		t.Fatalf("expected error to mention disruptive, got: %v", err)
	}
}

// TestApplyStatusJSONKeys confirms that the status command's JSON output
// contains all expected top-level keys.
func TestApplyStatusJSONKeys(t *testing.T) {
	stateRoot := t.TempDir()
	resp, err := executeLocal(Options{Command: "status", StateRoot: stateRoot, OutputJSON: true})
	if err != nil {
		t.Fatalf("status json: %v", err)
	}
	for _, key := range []string{`"metrics"`, `"timeline"`, `"desired"`, `"planned"`, `"observed"`, `"runtimeInitDiagnostics"`} {
		if !strings.Contains(resp.Message, key) {
			t.Errorf("expected key %s in status JSON, got:\n%s", key, resp.Message)
		}
	}
}

func TestStatusDefaultsToSingleActiveDeployment(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	manifestPath := writeV1Manifest(t, "edge")
	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	resp, err := executeLocal(Options{Command: "status", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(resp.Message, "deployment=edge") {
		t.Fatalf("expected status to default to the single active deployment, got:\n%s", resp.Message)
	}
}

func TestStatusOmitsDeploymentWhenMultipleActive(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1")
	t.Setenv("VCPE_SKIP_RUNTIME", "1")
	if _, err := executeLocal(Options{Command: "apply", ManifestPath: writeV1Manifest(t, "edge-a"), StateRoot: stateRoot}); err != nil {
		t.Fatalf("apply edge-a: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
	content := "apiVersion: vcpe.dev/v1\nkind: Deployment\nmetadata:\n  name: edge-b\nspec:\n  networks:\n    - role: mgmt\n      ipv4: { cidr: 10.20.10.0/24, gateway: 10.20.10.1, pool: { start: 10.20.10.10, end: 10.20.10.250 } }\n    - role: wan\n      nat: true\n      ipv4: { cidr: 10.20.200.0/24, gateway: 10.20.200.1, pool: { start: 10.20.200.10, end: 10.20.200.250 } }\n    - role: cm\n      ipv4: { cidr: 10.20.201.0/24, gateway: 10.20.201.1, pool: { start: 10.20.201.10, end: 10.20.201.250 } }\n  services:\n    - name: bng\n      type: bng\n      replicas: 1\n      image: { repository: ghcr.io/gdcs-dev/bng, tag: dev }\n      interfaces:\n        - { role: mgmt }\n        - { role: wan, defaultRoute: true }\n        - { role: cm }\n"
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot}); err != nil {
		t.Fatalf("apply edge-b: %v", err)
	}

	resp, err := executeLocal(Options{Command: "status", StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(resp.Message, "deployment=") {
		t.Fatalf("expected no default deployment selection with multiple active deployments, got:\n%s", resp.Message)
	}
}
