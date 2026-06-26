package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"gopkg.in/yaml.v3"
)

// runBuild resolves image actions for the manifest's services without applying
// runtime changes.
func runBuild(opts Options) (daemon.CommandResponse, error) {
	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}
	mgr := image.New(newImageBackend())
	summary, err := mgr.BuildWithOptions(context.Background(), doc, image.BuildOptions{NoCache: opts.NoCache})
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "build complete for deployment %q\n", doc.Metadata.Name)
	for _, action := range summary.Actions {
		fmt.Fprintf(&b, "  %s (%s): %s\n", action.Service, action.Type, action.Action)
	}
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// runPlan validates and resolves a manifest, reporting the intended deployment
// shape without mutating runtime resources or persisted state.
func runPlan(opts Options) (daemon.CommandResponse, error) {
	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}
	resolved, err := planner.Build(doc)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	disruptive, reasons, err := classifyDisruptive(ps, doc)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "plan for deployment %q\n", doc.Metadata.Name)
	fmt.Fprintf(&b, "  networks: %d\n", len(resolved.Networks))
	fmt.Fprintf(&b, "  services: %d\n", len(resolved.Services))
	for _, w := range UnusedNetworkWarnings(doc) {
		fmt.Fprintf(&b, "  warning: %s\n", w)
	}
	if disruptive {
		b.WriteString("  disruptive: yes\n")
		for _, r := range reasons {
			fmt.Fprintf(&b, "    - %s\n", r)
		}
		if !opts.AllowDisruptive {
			b.WriteString("  (re-run apply with --allow-disruptive to proceed)\n")
		}
	} else {
		b.WriteString("  disruptive: no\n")
	}
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// runList enumerates all known deployments in the state store.
func runList(opts Options) (daemon.CommandResponse, error) {
	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	names, err := ps.ListKnownDeployments()
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	if opts.OutputJSON {
		type listJSON struct {
			Deployments []string `json:"deployments"`
		}
		out := listJSON{Deployments: names}
		if out.Deployments == nil {
			out.Deployments = []string{}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("marshal list output: %w", err)
		}
		return daemon.CommandResponse{Message: string(b)}, nil
	}

	if len(names) == 0 {
		return daemon.CommandResponse{Message: "no deployments"}, nil
	}
	return daemon.CommandResponse{Message: strings.Join(names, "\n")}, nil
}

// runDown tears down a deployment selected by --name. Without a known
// deployment it reports an error identifying the unknown name.
func runDown(opts Options) (daemon.CommandResponse, error) {
	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	exists, err := ps.CustomerExists(opts.Name)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if !exists {
		return daemon.CommandResponse{}, fmt.Errorf("unknown deployment %q", opts.Name)
	}

	// Derive service names from the last desired snapshot so compose.Down
	// targets the exact projects that were brought up.
	serviceNames, err := serviceNamesFromSnapshot(ps, opts.Name)
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	opID, err := ps.StartOperation(opts.Command, "")
	if err != nil {
		return daemon.CommandResponse{}, err
	}

	// Tear down containers first, then clear leases so state reflects reality.
	if !skipRuntime() {
		if err := teardownComposeLifecycle(context.Background(), opts.StateRoot, opts.Name, serviceNames); err != nil {
			_ = ps.FinishOperation(opID, "failed", err.Error())
			return daemon.CommandResponse{}, err
		}
	}

	if err := ps.ReplaceCustomerLeases(opts.Name, nil); err != nil {
		_ = ps.FinishOperation(opID, "failed", err.Error())
		return daemon.CommandResponse{}, err
	}
	if err := ps.DeleteDeploymentSnapshot(opts.Name); err != nil {
		_ = ps.FinishOperation(opID, "failed", err.Error())
		return daemon.CommandResponse{}, err
	}
	if err := ps.FinishOperation(opID, "succeeded", "deployment torn down"); err != nil {
		return daemon.CommandResponse{}, err
	}
	return daemon.CommandResponse{Message: fmt.Sprintf("tore down deployment %q (operation %s)", opts.Name, opID)}, nil
}

// serviceNamesFromSnapshot returns the ordered service names from the latest
// desired snapshot for a deployment. Falls back to an empty slice so the caller
// can proceed even if no snapshot is recorded yet.
func serviceNamesFromSnapshot(ps *persist.Store, name string) ([]string, error) {
	raw, ok, err := ps.LatestDesiredSnapshot(name)
	if err != nil || !ok {
		return nil, err
	}
	var doc manifest.Document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse snapshot for %q: %w", name, err)
	}
	names := make([]string, 0, len(doc.Spec.Services))
	for _, svc := range doc.Spec.Services {
		if svc.Name != "" {
			names = append(names, svc.Name)
		}
	}
	return names, nil
}
