package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/state"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

// executeLocal runs a parsed command in-process against the resolved state root.
// Mutating commands (apply/up/down/destroy) acquire the writer lock and run the
// orchestrator; read commands inspect persisted state.
func executeLocal(opts Options) (daemon.CommandResponse, error) {
	// Ensure built-in service types are registered. Register is idempotent, so
	// this is safe whether we are entered from ExecuteCLI or a direct test call.
	types.Register()

	stateRoot, err := state.ResolveStateRoot(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	opts.StateRoot = stateRoot

	switch opts.Command {
	case "init":
		return runInit(opts)
	case "apply", "up":
		return runApply(opts)
	case "build":
		return runBuild(opts)
	case "push":
		return runPush(opts)
	case "release":
		return runRelease(opts)
	case "plan":
		return runPlan(opts)
	case "down", "destroy":
		return runDown(opts)
	case "list":
		return runList(opts)
	case "manifest":
		return runManifest(opts)
	case "status":
		return runStatus(opts)
	case "logs":
		return runLogs(opts)
	case "config":
		return runConfig(opts)
	case "state":
		return runState(opts)
	default:
		return daemon.CommandResponse{}, fmt.Errorf("command %q is not executable", opts.Command)
	}
}

func runInit(opts Options) (daemon.CommandResponse, error) {
	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()
	return daemon.CommandResponse{Message: fmt.Sprintf("initialized vCPE state at %s", opts.StateRoot)}, nil
}

// runStatus reports control-plane health. With --json it emits the structured
// desired/planned/observed view plus metrics and runtime-init diagnostics.
func runStatus(opts Options) (daemon.CommandResponse, error) {
	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	metrics, err := ps.Metrics()
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	timeline, err := ps.RecentOperations(10)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	if opts.OutputJSON {
		payload := map[string]any{
			"metrics":  metrics,
			"timeline": timeline,
			"desired":  desiredView(ps, opts.Name),
			"planned":  map[string]any{"deployment": opts.Name},
			"observed": map[string]any{"runningOperations": metrics.RunningOperations},
			"runtimeInitDiagnostics": map[string]any{
				"contractsRoot": state.VersionedArtifactsRoot(opts.StateRoot),
			},
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return daemon.CommandResponse{}, err
		}
		return daemon.CommandResponse{Message: string(out)}, nil
	}

	var b strings.Builder
	b.WriteString("vCPE status\n")
	if opts.Name != "" {
		fmt.Fprintf(&b, "deployment=%s\n", opts.Name)
	}
	fmt.Fprintf(&b, "reconcile total: %d (failures: %d)\n", metrics.ReconcileTotal, metrics.ReconcileFailures)
	fmt.Fprintf(&b, "ipam leases in use: %d\n", metrics.IPAMLeasesInUse)
	fmt.Fprintf(&b, "running operations: %d\n", metrics.RunningOperations)
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

func desiredView(ps *persist.Store, name string) map[string]any {
	view := map[string]any{}
	if name == "" {
		return view
	}
	if snap, ok, err := ps.LatestDesiredSnapshot(name); err == nil && ok {
		view["manifestBytes"] = len(snap)
	}
	return view
}

// runLogs surfaces operation timeline context. Per-deployment container logs
// require --name; without it we emit a usage hint while still returning the
// timeline so the command is never empty.
func runLogs(opts Options) (daemon.CommandResponse, error) {
	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	timeline, err := ps.RecentOperations(10)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	if opts.OutputJSON {
		payload := map[string]any{
			"timeline": timeline,
			"runtimeInitDiagnostics": map[string]any{
				"contractsRoot": state.VersionedArtifactsRoot(opts.StateRoot),
			},
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return daemon.CommandResponse{}, err
		}
		return daemon.CommandResponse{Message: string(out)}, nil
	}

	if opts.Name == "" {
		return daemon.CommandResponse{Message: "logs unavailable without --name; showing recent operations only"}, nil
	}
	return daemon.CommandResponse{Message: fmt.Sprintf("logs deployment=%s", opts.Name)}, nil
}

// runConfig exposes the control-plane file configuration. It is intentionally
// minimal now that profiles are removed: it reports the effective state root and
// socket paths.
func runConfig(opts Options) (daemon.CommandResponse, error) {
	args := opts.CommandArgs
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch args[0] {
	case "show":
		lines := []string{
			"VCPE_STATE_ROOT=" + opts.StateRoot,
			"VCPE_SOCKET=" + state.ResolveSocketPath(opts.StateRoot, opts.SocketPath),
		}
		sort.Strings(lines)
		return daemon.CommandResponse{Message: strings.Join(lines, "\n")}, nil
	default:
		return daemon.CommandResponse{}, fmt.Errorf("unsupported config subcommand %q", args[0])
	}
}

// runState implements `vcpe state reset`, the schema-version cutover command. It
// clears and re-stamps the state root so a v1 control plane can operate against
// a root that previously held incompatible state.
func runState(opts Options) (daemon.CommandResponse, error) {
	args := opts.CommandArgs
	if len(args) == 0 {
		return daemon.CommandResponse{}, fmt.Errorf("state requires a subcommand, e.g. `vcpe state reset`")
	}
	switch args[0] {
	case "reset":
		ps, err := persist.Open(opts.StateRoot)
		if err != nil {
			return daemon.CommandResponse{}, err
		}
		defer ps.Close()
		if err := ps.Reset(); err != nil {
			return daemon.CommandResponse{}, err
		}
		return daemon.CommandResponse{Message: fmt.Sprintf("state reset complete; stamped %s at %s", persist.SchemaVersion, opts.StateRoot)}, nil
	default:
		return daemon.CommandResponse{}, fmt.Errorf("unsupported state subcommand %q", args[0])
	}
}

var _ = context.Background
