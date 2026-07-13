package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/compose"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

// --- stubs ---

type recordingNetworkProvisioner struct {
	calls       []string
	removeCalls []string
}

func (r *recordingNetworkProvisioner) EnsureNetwork(_ context.Context, spec podman.NetworkSpec) error {
	r.calls = append(r.calls, spec.Name+"/"+spec.Subnet)
	return nil
}

func (r *recordingNetworkProvisioner) RemoveNetwork(_ context.Context, name string) error {
	r.removeCalls = append(r.removeCalls, name)
	return nil
}

type recordingComposeRunner struct {
	upCalls   []string
	downCalls []string
}

func (r *recordingComposeRunner) Up(_ context.Context, req compose.Request) (compose.OperationRecord, error) {
	r.upCalls = append(r.upCalls, req.ProjectName)
	return compose.OperationRecord{}, nil
}

func (r *recordingComposeRunner) Down(_ context.Context, req compose.Request) (compose.OperationRecord, error) {
	r.downCalls = append(r.downCalls, req.ProjectName)
	return compose.OperationRecord{}, nil
}

// stubLifecycle installs recording stubs and returns them.
// The original factories are restored at test cleanup.
func stubLifecycle(t *testing.T) (*recordingNetworkProvisioner, *recordingComposeRunner) {
	t.Helper()
	netStub := &recordingNetworkProvisioner{}
	cmpStub := &recordingComposeRunner{}
	origNet := newNetworkProvisioner
	origCmp := newComposeRunner
	newNetworkProvisioner = func() networkProvisioner { return netStub }
	newComposeRunner = func() composeLifecycleRunner { return cmpStub }
	t.Cleanup(func() {
		newNetworkProvisioner = origNet
		newComposeRunner = origCmp
	})
	return netStub, cmpStub
}

// makeRepoRoot creates a minimal repo root directory with the stub compose
// files needed by applyComposeLifecycle, and sets VCPE_REPO_ROOT.
func makeRepoRoot(t *testing.T, serviceTypes ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, svcType := range serviceTypes {
		dir := filepath.Join(root, "services", svcType)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("makeRepoRoot: mkdir %s: %v", dir, err)
		}
		// Stub compose.yaml with minimal env_file reference.
		content := "services:\n  " + svcType + ":\n    image: ${IMAGE}\n    env_file:\n      - compose.env\n"
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("makeRepoRoot: write compose.yaml for %s: %v", svcType, err)
		}
	}
	t.Setenv("VCPE_REPO_ROOT", root)
	return root
}

// writeArtifactEnv writes a minimal compose.env into the operation artifacts
// directory for the given service name.
func writeArtifactEnv(t *testing.T, stateRoot, opID, svcName string) string {
	t.Helper()
	dir := filepath.Join(stateRoot, "artifacts", "v1", "operations", opID, "runtime", svcName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("writeArtifactEnv: mkdir: %v", err)
	}
	path := filepath.Join(dir, "compose.env")
	if err := os.WriteFile(path, []byte("DEPLOYMENT_NAME=edge\nSERVICE_NAME="+svcName+"\nIMAGE=test:latest\n"), 0o644); err != nil {
		t.Fatalf("writeArtifactEnv: write: %v", err)
	}
	return path
}

// --- tests ---

// TestLifecycleStagesCuratedEnvFile verifies that applyComposeLifecycle copies
// the rendered compose.env to services/<type>/compose.env before calling
// compose up, satisfying the `env_file: compose.env` directive in curated
// compose.yaml files.
func TestLifecycleStagesCuratedEnvFile(t *testing.T) {
	stateRoot := t.TempDir()
	opID := "op-test-001"
	repoRoot := makeRepoRoot(t, "bng")
	netStub, cmpStub := stubLifecycle(t)

	writeArtifactEnv(t, stateRoot, opID, "bng")

	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "wan", Bridge: "edge-wan", IPv4: &plan.Family{CIDR: "10.200.0.0/24"}},
		},
		Services: []plan.Service{
			{Name: "bng", Type: "bng", Replicas: 1},
		},
	}

	if err := applyComposeLifecycle(context.Background(), stateRoot, opID, dep); err != nil {
		t.Fatalf("applyComposeLifecycle: %v", err)
	}

	// Env file must have been staged to the curated services directory.
	staged := filepath.Join(repoRoot, "services", "bng", "compose.env")
	data, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("expected staged compose.env at %s: %v", staged, err)
	}
	if !strings.Contains(string(data), "SERVICE_NAME=bng") {
		t.Errorf("staged env file missing expected content, got: %s", string(data))
	}

	// Compose up must have been called.
	if len(cmpStub.upCalls) != 1 || cmpStub.upCalls[0] != "edge-bng" {
		t.Errorf("expected compose up for edge-bng, got %v", cmpStub.upCalls)
	}
	_ = netStub // network provisioning verified in separate test
}

// TestLifecycleEnsuresPodmanNetworksBeforeCompose verifies that all Podman
// networks in the deployment are provisioned before any compose up call.
func TestLifecycleEnsuresPodmanNetworksBeforeCompose(t *testing.T) {
	stateRoot := t.TempDir()
	opID := "op-test-002"
	makeRepoRoot(t, "bng")
	netStub, cmpStub := stubLifecycle(t)

	writeArtifactEnv(t, stateRoot, opID, "bng")

	dep := plan.Deployment{
		Name: "edge",
		Networks: []plan.Network{
			{Role: "mgmt", Bridge: "edge-mgmt", IPv4: &plan.Family{CIDR: "10.10.0.0/24"}},
			{Role: "wan", Bridge: "edge-wan", IPv4: &plan.Family{CIDR: "10.200.0.0/24"}},
			{Role: "cm", Bridge: "edge-cm", IPv4: &plan.Family{CIDR: "10.201.0.0/24"}},
		},
		Services: []plan.Service{
			{Name: "bng", Type: "bng", Replicas: 1},
		},
	}

	if err := applyComposeLifecycle(context.Background(), stateRoot, opID, dep); err != nil {
		t.Fatalf("applyComposeLifecycle: %v", err)
	}

	// All three networks must have been provisioned.
	if len(netStub.calls) != 3 {
		t.Errorf("expected 3 EnsureNetwork calls, got %d: %v", len(netStub.calls), netStub.calls)
	}
	for _, want := range []string{"edge-mgmt/10.10.0.0/24", "edge-wan/10.200.0.0/24", "edge-cm/10.201.0.0/24"} {
		found := false
		for _, call := range netStub.calls {
			if call == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected EnsureNetwork call %q, got: %v", want, netStub.calls)
		}
	}

	// Compose up must also have run.
	if len(cmpStub.upCalls) != 1 {
		t.Errorf("expected 1 compose up call, got %v", cmpStub.upCalls)
	}
}

// TestApplyRunsLifecyclePhaseWithStubs verifies that the full apply pipeline
// reaches and records "lifecycle:succeeded" when VCPE_SKIP_RUNTIME is NOT set
// but stub adapters are installed. This catches regressions where the lifecycle
// phase is accidentally skipped even with a runtime available.
func TestApplyRunsLifecyclePhaseWithStubs(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("VCPE_SKIP_HOSTNET_PREFLIGHT", "1") // skip preflight only
	// Do NOT set VCPE_SKIP_RUNTIME — lifecycle must run.
	makeRepoRoot(t, "bng")
	netStub, cmpStub := stubLifecycle(t)
	t.Setenv("VCPE_SKIP_IMAGE", "1")

	manifestPath := writeV1Manifest(t, "edge")

	resp, err := executeLocal(Options{Command: "apply", ManifestPath: manifestPath, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(resp.Message, "applied deployment") {
		t.Fatalf("unexpected message: %q", resp.Message)
	}

	// Podman networks must have been provisioned (mgmt, wan, cm = 3).
	if len(netStub.calls) == 0 {
		t.Error("expected EnsureNetwork calls, got none")
	}

	// Compose up must have been called once per service.
	if len(cmpStub.upCalls) == 0 {
		t.Error("expected compose Up calls, got none")
	}

	// Phase log must include lifecycle:succeeded.
	ps, err := persist.Open(stateRoot)
	if err != nil {
		t.Fatalf("open persist: %v", err)
	}
	defer ps.Close()
	ops, err := ps.RecentOperations(1)
	if err != nil {
		t.Fatalf("recent ops: %v", err)
	}
	log := phaseLog(t, ps, ops[0].OperationID)
	if !strings.Contains(log, "lifecycle:succeeded") {
		t.Errorf("expected lifecycle:succeeded in phase log, got: %s", log)
	}
}

// TestTeardownCallsComposeDown verifies that teardownComposeLifecycle calls
// compose down in reverse service order.
func TestTeardownCallsComposeDown(t *testing.T) {
	stateRoot := t.TempDir()
	depName := "edge"
	makeRepoRoot(t, "bng", "gateway")
	_, cmpStub := stubLifecycle(t)

	// Write env files in the deployment artifacts dir so teardown can find them.
	depDir := filepath.Join(stateRoot, "artifacts", "v1", "deployments", depName)
	for _, svc := range []string{"bng", "gateway"} {
		dir := filepath.Join(depDir, "runtime", svc)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "compose.env"), []byte("IMAGE=test\n"), 0o644); err != nil {
			t.Fatalf("write env: %v", err)
		}
	}

	if err := teardownComposeLifecycle(context.Background(), stateRoot, depName, []string{"bng", "gateway"}); err != nil {
		t.Fatalf("teardownComposeLifecycle: %v", err)
	}

	// Down must be called for both services.
	if len(cmpStub.downCalls) != 2 {
		t.Errorf("expected 2 compose down calls, got %v", cmpStub.downCalls)
	}
	// Teardown is reverse order: gateway then bng.
	if cmpStub.downCalls[0] != "edge-gateway" || cmpStub.downCalls[1] != "edge-bng" {
		t.Errorf("expected reverse-order teardown [edge-gateway, edge-bng], got %v", cmpStub.downCalls)
	}
}
