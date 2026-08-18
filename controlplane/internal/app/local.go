package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/gdcs-dev/vcpe/controlplane/internal/health"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/state"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
	"gopkg.in/yaml.v3"
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
	case "plan":
		return runPlan(opts)
	case "down", "destroy":
		return runDown(opts)
	case "list":
		return runList(opts)
	case "manifest":
		return runManifest(opts)
	case "service":
		return runService(opts)
	case "status":
		return runStatus(opts)
	case "diagnose":
		return runDiagnose(opts)
	case "logs":
		return runLogs(opts)
	case "config":
		return runConfig(opts)
	case "state":
		return runState(opts)
	default:
		return dispatchDeveloperCommand(opts)
	}
}

type diagnosticOutcomeError struct{ outcome diagnostic.Outcome }

var newDiagnosticClient = diagnostic.NewClient

func (err diagnosticOutcomeError) Error() string {
	return "diagnostic result: " + string(err.outcome)
}

func runDiagnose(opts Options) (daemon.CommandResponse, error) {
	store, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer store.Close()
	if opts.Name == "" {
		names, err := store.ListKnownDeployments()
		if err != nil {
			return daemon.CommandResponse{}, err
		}
		switch len(names) {
		case 0:
			return daemon.CommandResponse{}, fmt.Errorf("no active deployments")
		case 1:
			opts.Name = names[0]
		default:
			return daemon.CommandResponse{}, fmt.Errorf(
				"multiple deployments active; specify one with --name:\n  %s",
				strings.Join(names, "\n  "),
			)
		}
	}
	result, err := diagnostic.Diagnose(context.Background(), store, diagnostic.DefaultRegistry(), newDiagnosticClient(10*time.Second), diagnostic.ResolveRequest{
		Deployment:          opts.Name,
		Source:              opts.From,
		Target:              opts.To,
		Replica:             opts.Replica,
		ClientService:       opts.ClientService,
		Subscriber:          opts.Subscriber,
		SubscriberReplica:   opts.SubscriberReplica,
		AllowActiveCallback: opts.AllowActiveCallback,
		AllowActiveEvent:    opts.AllowActiveEvent,
		Event:               opts.Event,
		DeviceID:            opts.DeviceID,
	})
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	var output string
	if opts.OutputJSON {
		output, err = diagnostic.RenderJSON(result)
	} else {
		output, err = diagnostic.RenderASCII(result)
	}
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	outcome, err := diagnostic.Classify(result)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	response := daemon.CommandResponse{Message: output}
	if outcome != diagnostic.OutcomePassed {
		return response, diagnosticOutcomeError{outcome: outcome}
	}
	return response, nil
}

type statusHealthObservation struct {
	Deployment string         `json:"deployment"`
	Service    string         `json:"service"`
	Replica    int            `json:"replica"`
	State      string         `json:"state"`
	ObservedAt string         `json:"observedAt"`
	Checks     []health.Check `json:"checks,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func collectStatusHealth(ps *persist.Store, deployment string) ([]statusHealthObservation, error) {
	if deployment == "" {
		return nil, nil
	}
	endpoints, err := ps.ListHealthEndpoints(deployment)
	if err != nil {
		return nil, err
	}
	targets := make([]health.Target, 0, len(endpoints))
	for _, endpoint := range endpoints {
		targets = append(targets, health.Target{Deployment: endpoint.Deployment, Service: endpoint.Service, Replica: endpoint.Replica, Host: "127.0.0.1", Port: endpoint.HostPort})
	}
	observations := health.NewCollector(0, 4).Collect(context.Background(), targets)
	views := make([]statusHealthObservation, 0, len(observations))
	for _, observation := range observations {
		view := statusHealthObservation{
			Deployment: observation.Target.Deployment,
			Service:    observation.Target.Service,
			Replica:    observation.Target.Replica,
			State:      observation.State,
			ObservedAt: observation.ObservedAt.Format(time.RFC3339Nano),
			Error:      observation.Error,
		}
		if observation.Response != nil {
			view.Checks = observation.Response.Checks
		}
		views = append(views, view)
	}
	observedInstances := make(map[string]struct{}, len(views))
	for _, view := range views {
		observedInstances[fmt.Sprintf("%s/%d", view.Service, view.Replica)] = struct{}{}
	}
	if snapshot, ok, err := ps.LatestDesiredSnapshot(deployment); err == nil && ok {
		var document manifest.Document
		if yaml.Unmarshal(snapshot, &document) == nil {
			for _, service := range document.Spec.Services {
				st, registered := typeregistry.Lookup(service.Type)
				if !registered || !st.Health().Valid() {
					continue
				}
				replicas := service.Replicas
				if replicas < 1 {
					replicas = 1
				}
				for replica := 0; replica < replicas; replica++ {
					if _, observed := observedInstances[fmt.Sprintf("%s/%d", service.Name, replica)]; observed {
						continue
					}
					views = append(views, statusHealthObservation{Deployment: deployment, Service: service.Name, Replica: replica, State: "not-configured", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)})
				}
			}
		}
	}
	sort.SliceStable(views, func(left, right int) bool {
		if views[left].Service != views[right].Service {
			return views[left].Service < views[right].Service
		}
		return views[left].Replica < views[right].Replica
	})
	return views, nil
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

	// With no --name, default to the single active deployment when there is
	// exactly one; leave it unset (deployment-agnostic output) otherwise.
	if opts.Name == "" {
		if names, err := ps.ListKnownDeployments(); err == nil && len(names) == 1 {
			opts.Name = names[0]
		}
	}

	metrics, err := ps.Metrics()
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	timeline, err := ps.RecentOperations(10)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	healthObservations, err := collectStatusHealth(ps, opts.Name)
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
			"health":   healthObservations,
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
		for _, observation := range healthObservations {
			fmt.Fprintf(&b, "health %s/%d: %s", observation.Service, observation.Replica, observation.State)
			if observation.Error != "" {
				fmt.Fprintf(&b, " (%s)", observation.Error)
			}
			b.WriteByte('\n')
		}
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
