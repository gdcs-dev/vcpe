package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/backend/podman"
	"github.com/gdcs-dev/vcpe/controlplane/internal/compose"
	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/hostnet"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
	"github.com/gdcs-dev/vcpe/controlplane/internal/ipam"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/observability"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"github.com/gdcs-dev/vcpe/controlplane/internal/render"
	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/contract"
	"github.com/gdcs-dev/vcpe/controlplane/internal/secrets"
	"github.com/gdcs-dev/vcpe/controlplane/internal/state"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
)

// failPhase reports whether the named phase is configured to fail via
// VCPE_FAIL_PHASE. It exists so integration tests can deterministically drive
// rollback paths without a Podman runtime.
func failPhase(name string) bool {
	return os.Getenv("VCPE_FAIL_PHASE") == name
}

// runApply executes the full reconcile pipeline for a v1 manifest. It is the
// single mutating path; phases are journaled and a failure after allocation
// triggers a bounded reverse-order rollback.
func runApply(opts Options) (daemon.CommandResponse, error) {
	if opts.ManifestPath == "" {
		return daemon.CommandResponse{}, fmt.Errorf("apply requires --manifest <path>")
	}

	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}

	lock, err := state.AcquireWriterLock(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer lock.Release()

	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	name := doc.Metadata.Name

	// Active-deployment cap: count distinct active deployments and reject a new
	// one that would exceed maxActiveDeployments.
	if err := enforceActiveDeploymentCap(ps, doc); err != nil {
		return daemon.CommandResponse{}, err
	}

	resolved, err := planner.Build(doc)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	// Guard: disruptive changes (CIDR modifications) are blocked unless the
	// operator explicitly opts in with --allow-disruptive.
	disruptive, reasons, err := classifyDisruptive(ps, doc)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if disruptive && !opts.AllowDisruptive {
		return daemon.CommandResponse{}, fmt.Errorf("apply blocked: disruptive changes detected (%s); re-run with --allow-disruptive to proceed", strings.Join(reasons, "; "))
	}

	opID, err := ps.StartOperation("apply", opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	secretValues, err := secrets.Resolve(doc.Spec.Secrets)
	if err != nil {
		_ = ps.FinishOperation(opID, "failed", "secret resolution failed")
		return daemon.CommandResponse{}, err
	}

	ctx := context.Background()
	allocated := false
	fail := func(phase string, cause error) (daemon.CommandResponse, error) {
		_ = ps.RecordPhase(opID, phase, "failed", cause.Error())
		if allocated {
			rollback(ps, opID, name)
		}
		_ = ps.FinishOperation(opID, "failed", cause.Error())
		observability.Log(observability.Event{Level: "ERROR", OperationID: opID, CustomerID: name, Phase: phase, Result: "failed", Message: cause.Error()})
		return daemon.CommandResponse{}, fmt.Errorf("apply %s: %s phase failed: %w", name, phase, cause)
	}

	// Phase: host-network preflight.
	intents := hostIntents(resolved)
	if os.Getenv("VCPE_SKIP_HOSTNET_PREFLIGHT") != "1" {
		if err := hostnet.New().Preflight(intents); err != nil {
			return fail("preflight", err)
		}
	}
	if failPhase("preflight") {
		return fail("preflight", fmt.Errorf("VCPE_FAIL_PHASE=preflight"))
	}
	_ = ps.RecordPhase(opID, "preflight", "succeeded", fmt.Sprintf("%d host-network intent(s)", len(intents)))

	// Phase: image lifecycle.
	if failPhase("images") {
		return fail("images", fmt.Errorf("VCPE_FAIL_PHASE=images"))
	}
	imgMgr := image.New(newImageBackend())
	if _, err := imgMgr.EnsureForApply(ctx, doc); err != nil && !skipRuntime() {
		return fail("images", err)
	}
	_ = ps.RecordPhase(opID, "images", "succeeded", "image lifecycle resolved")

	// Phase: IPAM allocation. This is the first persistent mutation; failures
	// after this point roll back the leases.
	conflicts, err := ipam.NewStore(ps).CheckConflicts(name, doc.Spec.Networks)
	if err != nil {
		return fail("allocation", err)
	}
	if err := ipam.NewStore(ps).Apply(name, doc.Spec.Networks); err != nil {
		return fail("allocation", err)
	}
	allocated = true
	if err := ipam.AllocateInterfaces(&resolved); err != nil {
		return fail("allocation", err)
	}
	if failPhase("allocation") {
		return fail("allocation", fmt.Errorf("VCPE_FAIL_PHASE=allocation"))
	}
	_ = ps.RecordPhase(opID, "allocation", "succeeded", fmt.Sprintf("%v", conflicts))

	// Phase: render typed artifacts.
	if failPhase("render") {
		return fail("render", fmt.Errorf("VCPE_FAIL_PHASE=render"))
	}
	if err := renderAll(ctx, opts.StateRoot, opID, resolved, secretValues); err != nil {
		return fail("render", err)
	}
	_ = ps.RecordPhase(opID, "render", "succeeded", "typed artifacts rendered")

	// Phase: runtime-init contract generation + verification per service.
	contracts := contract.BuildForDeployment(opID, resolved)
	if err := writeStartupContracts(opts.StateRoot, opID, name, contracts); err != nil {
		return fail("runtime-init", err)
	}
	for _, svcName := range sortedKeys(contracts) {
		if err := contract.Validate(contracts[svcName]); err != nil {
			return fail("runtime-init", fmt.Errorf("service %s: %w", svcName, err))
		}
		_ = ps.RecordPhase(opID, "runtime-init:"+svcName, "succeeded", "startup contract generated")
	}
	if failPhase("runtime-init-verify") {
		return fail("runtime-init-verify", fmt.Errorf("VCPE_FAIL_PHASE=runtime-init-verify"))
	}
	_ = ps.RecordPhase(opID, "runtime-init-verify", "succeeded", "startup contracts verified")

	// Phase: compose lifecycle.
	if failPhase("lifecycle") {
		return fail("lifecycle", fmt.Errorf("VCPE_FAIL_PHASE=lifecycle"))
	}
	if !skipRuntime() {
		if err := applyComposeLifecycle(ctx, opts.StateRoot, opID, resolved); err != nil {
			return fail("lifecycle", err)
		}
	}
	_ = ps.RecordPhase(opID, "lifecycle", "succeeded", "compose lifecycle applied")

	// Record desired snapshot keyed by deployment name and finish.
	raw, _ := os.ReadFile(opts.ManifestPath)
	if err := ps.SaveDesiredSnapshot(name, raw); err != nil {
		return fail("record", err)
	}
	if err := ps.FinishOperation(opID, "succeeded", "apply converged"); err != nil {
		return daemon.CommandResponse{}, err
	}
	observability.Log(observability.Event{OperationID: opID, CustomerID: name, Phase: "operation", Result: "succeeded", Message: "apply converged"})

	return daemon.CommandResponse{Message: fmt.Sprintf("applied deployment %q (operation %s)", name, opID)}, nil
}

// rollback reverses the allocation phase. The journal/operation semantics are
// preserved from the existing engine; only the deployment key (metadata.name)
// changed.
func rollback(ps *persist.Store, opID, name string) {
	_ = ps.ReplaceCustomerLeases(name, nil)
	_ = ps.RecordPhase(opID, "rollback", "succeeded", "reverted allocation")
}

// enforceActiveDeploymentCap rejects a new deployment that would exceed
// spec.maxActiveDeployments distinct active metadata.name values.
func enforceActiveDeploymentCap(ps *persist.Store, doc manifest.Document) error {
	cap := doc.Spec.MaxActiveDeployments
	if cap <= 0 {
		return nil
	}
	known, err := ps.CountKnownCustomers()
	if err != nil {
		return err
	}
	exists, err := ps.CustomerExists(doc.Metadata.Name)
	if err != nil {
		return err
	}
	if !exists && known+1 > cap {
		return fmt.Errorf("applying deployment %q would exceed maxActiveDeployments %d (currently %d active)", doc.Metadata.Name, cap, known)
	}
	return nil
}

// hostIntents derives host-network intents from the resolved networks.
func hostIntents(dep plan.Deployment) []hostnet.Intent {
	intents := make([]hostnet.Intent, 0, len(dep.Networks))
	for _, n := range dep.Networks {
		intents = append(intents, hostnet.Intent{
			Role:             n.Role,
			Bridge:           n.Bridge,
			RequiresNAT:      n.NAT,
			RequiresFirewall: n.Firewall,
		})
	}
	return intents
}

// renderAll dispatches each service to its registered renderer, writes every
// artifact to <opArtifactsDir>/runtime/<serviceName>/<key>, and mirrors a copy
// to the deployment artifacts dir so the env files survive beyond the operation.
func renderAll(ctx context.Context, stateRoot, opID string, dep plan.Deployment, secretValues map[string]string) error {
	opDir := state.OperationArtifactsDir(stateRoot, opID)
	depDir := state.DeploymentArtifactsDir(stateRoot, dep.Name)
	for _, svc := range dep.Services {
		if len(svc.Instances) == 0 {
			continue
		}
		st, ok := typeregistry.Lookup(svc.Type)
		if !ok {
			return fmt.Errorf("service %q has unregistered type %q", svc.Name, svc.Type)
		}
		result, err := st.Renderer().Render(ctx, render.Input{Deployment: dep, Service: svc, Secrets: secretValues})
		if err != nil {
			return fmt.Errorf("render service %q: %w", svc.Name, err)
		}
		for _, artifact := range result.Artifacts {
			for _, base := range []string{opDir, depDir} {
				dst := filepath.Join(base, "runtime", svc.Name, artifact.Key)
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
				}
				if err := os.WriteFile(dst, []byte(artifact.Content), 0o644); err != nil {
					return fmt.Errorf("write artifact %s: %w", dst, err)
				}
			}
		}
	}
	return nil
}

// applyComposeLifecycle brings up each service's Podman Compose project in
// dependency order. Curated types (bng, gateway, webpa) use the checked-in
// services/<type>/compose.yaml; services that render their own compose.yaml
// (generic-container) use the artifact from the render phase.
func applyComposeLifecycle(ctx context.Context, stateRoot, opID string, dep plan.Deployment) error {
	repoRoot, err := resolveRepoRoot()
	if err != nil {
		return err
	}

	// Provision Podman networks for every network role in the deployment. The
	// compose.yaml references these as external networks; they must exist before
	// podman-compose up is called. podman.EnsureNetwork is idempotent.
	podmanAdapter := newNetworkProvisioner()
	for _, net := range dep.Networks {
		cidr := ""
		if net.IPv4 != nil {
			cidr = net.IPv4.CIDR
		}
		if err := podmanAdapter.EnsureNetwork(ctx, net.Bridge, cidr, net.HostBridgeGateway, net.PodmanDNS); err != nil {
			return fmt.Errorf("ensure podman network %s: %w", net.Bridge, err)
		}
	}

	adapter := newComposeRunner()
	opDir := state.OperationArtifactsDir(stateRoot, opID)
	for _, svc := range dep.Services {
		envFile := filepath.Join(opDir, "runtime", svc.Name, "compose.env")
		// If the renderer produced a compose.yaml artifact, use it; otherwise
		// fall back to the curated services/<type>/compose.yaml.
		generated := filepath.Join(opDir, "runtime", svc.Name, "compose.yaml")
		composeFile := generated
		isCurated := false
		if _, err := os.Stat(generated); errors.Is(err, os.ErrNotExist) {
			composeFile = filepath.Join(repoRoot, "services", svc.Type, "compose.yaml")
			isCurated = true
		}
		// Curated compose.yaml files expect:
		//   - compose.env in the same directory as compose.yaml (env_file: compose.env)
		//   - runtime config tree at services/<type>/runtime/ (volume: ./runtime:/runtime-config)
		// Stage both from the rendered artifacts directory.
		if isCurated {
			curatedEnvDst := filepath.Join(repoRoot, "services", svc.Type, "compose.env")
			if data, err := os.ReadFile(envFile); err == nil {
				_ = os.WriteFile(curatedEnvDst, data, 0o644)
			}
			runtimeSrc := filepath.Join(opDir, "runtime", svc.Name)
			runtimeDst := filepath.Join(repoRoot, "services", svc.Type, "runtime")
			_ = stageRuntimeTree(runtimeSrc, runtimeDst)
		}
		req := compose.Request{
			ComposeGroup: svc.Type,
			ProjectName:  dep.Name + "-" + svc.Name,
			WorkingDir:   repoRoot,
			ComposeFile:  composeFile,
			EnvFile:      envFile,
			Timeout:      2 * time.Minute,
		}
		if _, err := adapter.Up(ctx, req); err != nil {
			return fmt.Errorf("compose up %s: %w", svc.Name, err)
		}
	}
	return nil
}

// stageRuntimeTree copies rendered runtime artifacts from src (the operation
// artifact directory for a service) into dst (services/<type>/runtime/),
// preserving the relative subdirectory layout. Only regular files are copied;
// compose.env is skipped (it is staged separately alongside compose.yaml).
func stageRuntimeTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if rel == "compose.env" || rel == "compose.yaml" {
			return nil
		}
		dstPath := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
}

// teardownComposeLifecycle stops all Compose projects for a deployment in
// reverse dependency order using the service names from the last snapshot.
func teardownComposeLifecycle(ctx context.Context, stateRoot, depName string, serviceNames []string) error {
	repoRoot, err := resolveRepoRoot()
	if err != nil {
		return err
	}
	adapter := newComposeRunner()
	depDir := state.DeploymentArtifactsDir(stateRoot, depName)
	// Tear down in reverse order.
	for i := len(serviceNames) - 1; i >= 0; i-- {
		svcName := serviceNames[i]
		envFile := filepath.Join(depDir, "runtime", svcName, "compose.env")
		generated := filepath.Join(depDir, "runtime", svcName, "compose.yaml")
		composeFile := generated
		if _, statErr := os.Stat(generated); errors.Is(statErr, os.ErrNotExist) {
			// We don't know the type anymore from just the name; try the env file
			// neighbour to infer the curated path, or skip gracefully.
			if _, statErr2 := os.Stat(envFile); statErr2 != nil {
				continue
			}
			// Infer compose file by scanning curated services dirs.
			for _, candidate := range []string{"bng", "gateway", "webpa", "routerd", "xb10"} {
				cf := filepath.Join(repoRoot, "services", candidate, "compose.yaml")
				if _, statErr3 := os.Stat(cf); statErr3 == nil {
					// Match service name suffix against candidate type.
					if svcName == candidate || len(svcName) > len(candidate) {
						composeFile = cf
						break
					}
				}
			}
		}
		req := compose.Request{
			ProjectName: depName + "-" + svcName,
			WorkingDir:  repoRoot,
			ComposeFile: composeFile,
			EnvFile:     envFile,
			Timeout:     2 * time.Minute,
		}
		if _, err := adapter.Down(ctx, req); err != nil {
			// Best-effort: log and continue so remaining services are torn down.
			fmt.Fprintf(os.Stderr, "warn: compose down %s: %v\n", svcName, err)
		}
	}
	return nil
}

// resolveRepoRoot walks parent directories from the working directory looking
// for the repository root, identified by the presence of services/bng/compose.yaml.
func resolveRepoRoot() (string, error) {
	if root := os.Getenv("VCPE_REPO_ROOT"); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "services", "bng", "compose.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", errors.New("unable to locate repo root; set VCPE_REPO_ROOT")
}

// writeStartupContracts persists each service's startup contract to both the
// operation artifacts dir and the deployment artifacts dir (keyed by
// metadata.name), matching the integration-test artifact layout.
func writeStartupContracts(stateRoot, opID, name string, contracts map[string]contract.Document) error {
	opDir := filepath.Join(state.OperationArtifactsDir(stateRoot, opID), "runtime", "startup-contracts")
	depDir := filepath.Join(state.DeploymentArtifactsDir(stateRoot, name), "runtime", "startup-contracts")
	for _, dir := range []string{opDir, depDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create startup-contract dir: %w", err)
		}
	}
	for svcName, doc := range contracts {
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal startup contract %s: %w", svcName, err)
		}
		for _, dir := range []string{opDir, depDir} {
			if err := os.WriteFile(filepath.Join(dir, svcName+".json"), data, 0o644); err != nil {
				return fmt.Errorf("write startup contract %s: %w", svcName, err)
			}
		}
	}
	return nil
}

func sortedKeys(m map[string]contract.Document) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// networkProvisioner provisions Podman networks before compose lifecycle.
type networkProvisioner interface {
	EnsureNetwork(ctx context.Context, name, subnet, hostGateway, podmanDNS string) error
}

// composeLifecycleRunner runs podman-compose up/down for a service.
type composeLifecycleRunner interface {
	Up(ctx context.Context, req compose.Request) (compose.OperationRecord, error)
	Down(ctx context.Context, req compose.Request) (compose.OperationRecord, error)
}

// newNetworkProvisioner and newComposeRunner are package-level factories so
// tests can substitute stubs without a container runtime.
var newNetworkProvisioner = func() networkProvisioner { return podman.New() }
var newComposeRunner = func() composeLifecycleRunner { return compose.New() }

// skipRuntime reports whether Podman-dependent phases (images, lifecycle)
// should be treated as no-ops. Set VCPE_SKIP_RUNTIME=1 in tests that exercise
// the command layer without a container runtime.
// VCPE_SKIP_HOSTNET_PREFLIGHT=1 only skips the host-network capability check;
// it does NOT skip the compose lifecycle.
func skipRuntime() bool {
	return os.Getenv("VCPE_SKIP_RUNTIME") == "1"
}
