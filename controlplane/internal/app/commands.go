package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/app/wizard"
	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/image"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"gopkg.in/yaml.v3"
)

// osExecutable is a package-level variable so tests can inject a fake.
var osExecutable = os.Executable

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
	platforms := opts.Platforms
	if len(platforms) == 0 {
		platforms = []string{"linux/amd64", "linux/arm64"}
	}
	mgr := image.New(newImageBackend(opts.Backend))
	summary, err := mgr.BuildWithOptions(context.Background(), doc, image.BuildOptions{NoCache: opts.NoCache, Platforms: platforms, ForceBuild: true})
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "build complete for deployment %q (platforms: %s)\n", doc.Metadata.Name, strings.Join(platforms, ","))
	for _, action := range summary.Actions {
		fmt.Fprintf(&b, "  %s (%s): %s\n", action.Service, action.Type, action.Action)
	}
	return daemon.CommandResponse{Message: strings.TrimRight(b.String(), "\n")}, nil
}

// runPush pushes all service images from the manifest to their registries.
func runPush(opts Options) (daemon.CommandResponse, error) {
	doc, err := manifest.Load(opts.ManifestPath)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	if err := Preflight(doc); err != nil {
		return daemon.CommandResponse{}, err
	}
	backend := newImageBackend(opts.Backend)
	var b strings.Builder
	fmt.Fprintf(&b, "push complete for deployment %q\n", doc.Metadata.Name)
	for _, svc := range doc.Spec.Services {
		ref := image.ImageReference(svc.Image)
		if err := backend.PushImage(context.Background(), image.PushRequest{Reference: ref}); err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("push %s (%s): %w", svc.Name, ref, err)
		}
		fmt.Fprintf(&b, "  %s (%s): pushed\n", svc.Name, ref)
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

// runManifest dispatches vcpe manifest <subcommand>.
func runManifest(opts Options) (daemon.CommandResponse, error) {
	if len(opts.CommandArgs) == 0 {
		return daemon.CommandResponse{}, fmt.Errorf("manifest requires a subcommand; try `vcpe manifest list`")
	}
	switch opts.CommandArgs[0] {
	case "list":
		return runManifestList(opts)
	case "build":
		return runManifestBuild(opts)
	default:
		return daemon.CommandResponse{}, fmt.Errorf("unknown manifest subcommand %q; run `vcpe manifest --help`", opts.CommandArgs[0])
	}
}

// runManifestBuild runs the interactive manifest builder wizard.
func runManifestBuild(opts Options) (daemon.CommandResponse, error) {
	path, err := wizard.Run(context.Background(), wizard.Opts{
		ExistingPath: opts.ManifestPath,
		OutputPath:   opts.OutputPath,
	})
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	return daemon.CommandResponse{Message: "manifest written to " + path}, nil
}

// runManifestList discovers and prints all available manifest files.
func runManifestList(opts Options) (daemon.CommandResponse, error) {
	dirs := manifest.SearchDirs(osExecutable)
	entries, err := manifest.FindAll(dirs)
	if err != nil {
		return daemon.CommandResponse{}, fmt.Errorf("manifest discovery: %w", err)
	}

	if opts.OutputJSON {
		type entry struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Description string `json:"description"`
		}
		out := make([]entry, len(entries))
		for i, e := range entries {
			out[i] = entry{Name: e.Name, Path: e.Path, Description: e.Description}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return daemon.CommandResponse{}, fmt.Errorf("marshal manifest list: %w", err)
		}
		return daemon.CommandResponse{Message: string(b)}, nil
	}

	if len(entries) == 0 {
		return daemon.CommandResponse{Message: "no manifests found in search path"}, nil
	}

	// Build aligned table: NAME  PATH  DESCRIPTION
	const minPad = 2
	maxName, maxPath := len("NAME"), len("PATH")
	for _, e := range entries {
		if len(e.Name) > maxName {
			maxName = len(e.Name)
		}
		if len(e.Path) > maxPath {
			maxPath = len(e.Path)
		}
	}
	colName := maxName + minPad
	colPath := maxPath + minPad

	var b strings.Builder
	fmt.Fprintf(&b, "%-*s%-*s%s\n", colName, "NAME", colPath, "PATH", "DESCRIPTION")
	for _, e := range entries {
		desc := e.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(&b, "%-*s%-*s%s\n", colName, e.Name, colPath, e.Path, desc)
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

// runDown tears down a deployment selected by --name. If --name is omitted and
// exactly one deployment exists it is selected automatically. If multiple
// deployments exist the names are listed and the user is asked to re-run with
// --name.
func runDown(opts Options) (daemon.CommandResponse, error) {
	ps, err := persist.Open(opts.StateRoot)
	if err != nil {
		return daemon.CommandResponse{}, err
	}
	defer ps.Close()

	if opts.Name == "" {
		names, err := ps.ListKnownDeployments()
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

	// Tear down containers first, then networks, then clear leases so state
	// reflects reality in dependency order.
	if !skipRuntime() {
		if err := teardownComposeLifecycle(context.Background(), opts.StateRoot, opts.Name, serviceNames); err != nil {
			_ = ps.FinishOperation(opID, "failed", err.Error())
			return daemon.CommandResponse{}, err
		}
		// Remove Podman networks after all containers are stopped. Failures are
		// best-effort — a warning is logged and teardown continues.
		teardownNetworks(context.Background(), ps, opts.Name, newNetworkProvisioner())
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
