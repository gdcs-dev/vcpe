package wizard

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/hostnet"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"gopkg.in/yaml.v3"
)

// Opts configures the wizard run.
type Opts struct {
	// Stdin/Stdout for interactive prompts. Defaults to os.Stdin/os.Stderr.
	Stdin  io.Reader
	Stdout io.Writer
	// ExistingPath loads an existing manifest for update mode.
	ExistingPath string
	// OutputPath overrides the default output path.
	OutputPath string
}

// Run orchestrates the four-phase manifest builder wizard.
// Returns the path of the written manifest.
func Run(ctx context.Context, opts Opts) (string, error) {
	r := opts.Stdin
	if r == nil {
		r = os.Stdin
	}
	w := opts.Stdout
	if w == nil {
		w = os.Stderr
	}

	var existing manifest.Document
	if opts.ExistingPath != "" {
		doc, err := manifest.Load(opts.ExistingPath)
		if err != nil {
			return "", fmt.Errorf("load manifest %s: %w", opts.ExistingPath, err)
		}
		existing = doc
		fmt.Fprintf(w, "Updating manifest: %s\n", opts.ExistingPath)
	}

	if opts.ExistingPath == "" {
		fmt.Fprintln(w, "vcpe manifest builder — press Enter to accept defaults")
	}

	// Phase 1: Identity.
	fmt.Fprintln(w, "═══ Phase 1: Identity ═══")
	doc := manifest.Document{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
	}
	doc.Metadata.Name = Prompt(w, r, "Deployment name", orDefault(existing.Metadata.Name, "example"))

	maxReplicas := existing.Spec.MaxReplicasPerService
	if maxReplicas == 0 {
		maxReplicas = 3
	}
	maxDeploys := existing.Spec.MaxActiveDeployments
	if maxDeploys == 0 {
		maxDeploys = 10
	}
	promptInt := func(label string, def int) int {
		s := Prompt(w, r, label, fmt.Sprintf("%d", def))
		n := def
		fmt.Sscanf(s, "%d", &n)
		return n
	}
	doc.Spec.MaxReplicasPerService = promptInt("Max replicas per service", maxReplicas)
	doc.Spec.MaxActiveDeployments = promptInt("Max active deployments", maxDeploys)

	// Phase 2: Networks.
	fmt.Fprintln(w, "\n═══ Phase 2: Networks ═══")
	ha := hostnet.New()
	networks, netLookup := AskNetworks(ctx, r, w, existing.Spec.Networks, &ha)
	doc.Spec.Networks = networks

	// Phase 3: Services.
	fmt.Fprintln(w, "\n═══ Phase 3: Services ═══")
	types := typeregistry.Registered()
	services := AskServices(r, w, existing.Spec.Services, netLookup, types)
	doc.Spec.Services = services

	// Phase 4: Output.
	fmt.Fprintln(w, "\n═══ Phase 4: Output ═══")
	outPath := opts.OutputPath
	if outPath == "" {
		outPath = defaultOutputPath(opts.ExistingPath, doc.Metadata.Name)
	}
	return AskOutput(r, w, outPath, doc)
}

// loadExistingConfig decodes an existing yaml.Node config into the given target.
func loadExistingConfig(node yaml.Node, target interface{}) {
	if node.Kind == 0 {
		return
	}
	_ = node.Decode(target)
}
