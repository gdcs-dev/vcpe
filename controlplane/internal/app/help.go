package app

import (
	"fmt"
	"strings"
)

// FlagHelp describes a single flag in help output.
type FlagHelp struct {
	Name        string // e.g. "--manifest"
	Arg         string // e.g. "<path>"; empty for boolean flags
	Description string
}

// CommandHelp holds all help data for a single command.
type CommandHelp struct {
	Synopsis      string     // one-line description for GlobalHelp table
	Description   string     // 1-2 sentence body for per-command help
	Positionals   []string   // positional argument names, e.g. ["<service>", "<subcommand>"]
	RequiredFlags []FlagHelp // flags the command requires to function
	OptionalFlags []FlagHelp // flags the command accepts but does not require
	Examples      []string   // example invocations
}

// commandHelp is the single source of truth for per-command help content.
// TestHelpCoverage enforces that every key in topLevelCommands has an entry.
var commandHelp = map[string]CommandHelp{
	"init": {
		Synopsis:      "Initialize the vCPE state root",
		Description:   "Creates or verifies the state root directory structure and stamps the schema version. Safe to re-run on an existing state root.",
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
		},
		Examples: []string{
			"vcpe init",
			"vcpe init --state-root /var/lib/vcpe",
		},
	},
	"build": {
		Synopsis:    "Build or pull service images from a manifest",
		Description: "Resolves image actions (build, pull, or noop) for all services in the manifest without starting any containers. Respects the image pull policy declared in the manifest. Defaults to building a multi-arch OCI manifest list for linux/amd64 and linux/arm64; requires QEMU emulation on the Podman machine for cross-arch targets.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime for image operations (default: podman). With --backend docker, multi-arch builds use `docker buildx build --push` and push to the registry during build. Note: use pullPolicy: always-pull or missing in the manifest when running `vcpe up` after a Docker build."},
			{Name: "--platform", Arg: "<csv>", Description: "Comma-separated OS/arch targets (default: linux/amd64,linux/arm64)"},
			{Name: "--no-cache", Description: "Disable layer cache when building images"},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--json", Description: "Emit structured JSON output"},
		},
		Examples: []string{
			"vcpe build --manifest ./manifest-bng-7.yaml",
			"vcpe build --manifest ./manifest.yaml --backend docker",
			"vcpe build --manifest ./manifest.yaml --platform linux/amd64",
			"vcpe build --manifest ./manifest.yaml --no-cache",
		},
	},
	"push": {
		Synopsis:    "Push service images from a manifest to their registries",
		Description: "Pushes all service images referenced in the manifest to their registries. The registry is derived from each service's image repository. Run `podman login <registry>` before pushing to authenticated registries.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime for push operations (default: podman). With --backend docker, this is a re-push; multi-arch images are already pushed during `vcpe build --backend docker`."},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
		},
		Examples: []string{
			"vcpe push --manifest ./manifest-bng-7.yaml",
			"vcpe push --manifest ./manifest-bng-7.yaml --backend docker",
		},
	},
	"up": {
		Synopsis:    "Bring up a deployment from a manifest",
		Description: "Reconciles networks, images, IPAM allocation, and compose lifecycle in a single journaled operation. Alias: apply",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--allow-disruptive", Description: "Permit CIDR changes and scale-to-zero operations that would otherwise be blocked"},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--socket", Arg: "<path>", Description: "Override the daemon socket path"},
			{Name: "--json", Description: "Emit structured JSON output"},
		},
		Examples: []string{
			"vcpe up --manifest ./manifest-bng-7.yaml",
			"vcpe up --manifest ./manifest.yaml --allow-disruptive",
		},
	},
	"plan": {
		Synopsis:    "Show planned changes without applying",
		Description: "Validates a manifest and reports the intended deployment shape, network count, service count, and whether any changes are disruptive. Does not mutate any state.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--json", Description: "Emit structured JSON output"},
		},
		Examples: []string{
			"vcpe plan --manifest ./manifest-bng-7.yaml",
		},
	},
	"down": {
		Synopsis:    "Tear down a named deployment",
		Description: "Stops compose services and releases all IPAM leases for the named deployment. Alias: destroy (destroy also requires --force).",
		RequiredFlags: []FlagHelp{
			{Name: "--name", Arg: "<deployment>", Description: "Name of the deployment to tear down (metadata.name from the manifest)"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--socket", Arg: "<path>", Description: "Override the daemon socket path"},
		},
		Examples: []string{
			"vcpe down --name bng-7",
		},
	},
	"status": {
		Synopsis:      "Show control-plane status",
		Description:   "Reports reconcile metrics, active IPAM leases, and recent operation history. With --name, shows the desired state snapshot for that deployment.",
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{
			{Name: "--name", Arg: "<deployment>", Description: "Filter output to a specific deployment"},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--json", Description: "Emit structured JSON with metrics, timeline, desired, planned, observed, and runtimeInitDiagnostics keys"},
		},
		Examples: []string{
			"vcpe status",
			"vcpe status --name bng-7",
			"vcpe status --json",
		},
	},
	"logs": {
		Synopsis:      "Show operation timeline and deployment logs",
		Description:   "Surfaces the recent operation timeline. With --name, includes per-deployment log context from the container runtime.",
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{
			{Name: "--name", Arg: "<deployment>", Description: "Show logs scoped to the named deployment"},
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--json", Description: "Emit structured JSON with timeline and runtimeInitDiagnostics keys"},
		},
		Examples: []string{
			"vcpe logs",
			"vcpe logs --name bng-7",
			"vcpe logs --name bng-7 --json",
		},
	},
	"config": {
		Synopsis:      "Show effective configuration",
		Description:   "Displays the resolved configuration values that vcpe will use, including the effective state root, socket path, and any environment overrides.",
		Positionals:   []string{"<subcommand>"},
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
		},
		Examples: []string{
			"vcpe config show",
		},
	},
	"state": {
		Synopsis:      "Manage persisted control-plane state",
		Description:   "Provides subcommands for inspecting or resetting the persisted state. Use `state reset` to clear all IPAM leases and deployment snapshots when recovering from schema migrations.",
		Positionals:   []string{"<subcommand>"},
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
		},
		Examples: []string{
			"vcpe state reset",
		},
	},
	"list": {
		Synopsis:      "List known deployments",
		Description:   "Prints the name of every deployment that has ever been applied, drawn from persisted IPAM leases and desired-state snapshots.",
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{
			{Name: "--state-root", Arg: "<path>", Description: "Override the default state root directory"},
			{Name: "--json", Description: `Emit {"deployments":[...]} JSON`},
		},
		Examples: []string{
			"vcpe list",
			"vcpe list --json",
		},
	},
	"manifest": {
		Synopsis:      "Manage and discover manifest files",
		Description:   "Subcommands for working with deployment manifest files: `list` discovers available manifests; `build` runs an interactive wizard to create or update a manifest.",
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{},
		Examples: []string{
			"vcpe manifest list",
			"vcpe manifest list --json",
			"vcpe manifest build",
			"vcpe manifest build --manifest existing.yaml",
			"vcpe manifest build --manifest existing.yaml --output new.yaml",
		},
	},
	"version": {
		Synopsis:      "Print the vcpe version",
		Description:   "Prints the embedded version string and exits. Builds without -ldflags override report \"dev\".",
		RequiredFlags: []FlagHelp{},
		OptionalFlags: []FlagHelp{},
		Examples: []string{
			"vcpe version",
		},
	},
	"release": {
		Synopsis:    "Stamp manifest, commit, tag, push git, then build and push images",
		Description: "Requires --version <vX.Y.Z>. Sequence: (1) validate not on a non-main branch and that the tag doesn't exist; (2) stamp first-party image tags in the manifest; (3) git add + commit + tag + push to origin; (4) build all first-party service images as multi-arch OCI manifest lists with both the versioned tag and :latest and push to registry. Always defaults to the Docker backend. Third-party images (no buildContext) are never modified.",
		RequiredFlags: []FlagHelp{
			{Name: "--manifest", Arg: "<path>", Description: "Path to deployment manifest YAML (will be stamped in place)"},
			{Name: "--version", Arg: "<vX.Y.Z>", Description: "Release version tag to create (e.g. v0.2.0); must not already exist in git"},
		},
		OptionalFlags: []FlagHelp{
			{Name: "--backend", Arg: "<podman|docker>", Description: "Container runtime backend (default: docker)"},
			{Name: "--platform", Arg: "<os/arch,...>", Description: "Target platforms (default: linux/amd64,linux/arm64)"},
		},
		Examples: []string{
			"vcpe release --manifest manifests/example.yaml --version v0.2.0",
			"vcpe release --manifest manifests/example.yaml --version v0.2.0 --platform linux/amd64",
		},
	},
}

// GlobalHelp returns the top-level help string listing all public commands.
func GlobalHelp() string {
	var b strings.Builder
	b.WriteString("Usage: vcpe <command> [flags]\n\n")
	b.WriteString("Commands:\n")

	// Fixed column width for aligned synopsis column.
	const synopsisCol = 10
	order := []string{"init", "build", "push", "release", "up", "plan", "down", "list", "manifest", "status", "logs", "config", "state", "version"}
	for _, cmd := range order {
		h := commandHelp[cmd]
		padding := synopsisCol - len(cmd)
		if padding < 2 {
			padding = 2
		}
		fmt.Fprintf(&b, "  %s%s%s\n", cmd, strings.Repeat(" ", padding), h.Synopsis)
	}
	b.WriteString("\nAliases:\n")
	b.WriteString("  apply    alias for up\n")
	b.WriteString("  destroy  alias for down (also requires --force)\n")

	b.WriteString("\nGlobal flags:\n")
	b.WriteString("  --state-root <path>  Override state root directory\n")
	b.WriteString("  --config <path>      Config file path\n")
	b.WriteString("  --socket <path>      Daemon socket path\n")

	b.WriteString("\nRun `vcpe <command> --help` for command-specific help.\n")
	return b.String()
}

// HelpFor returns the per-command help string for the given command name.
// Aliases produce a one-line redirect to the primary command.
func HelpFor(command string) string {
	switch command {
	case "apply":
		return "apply is an alias for up — run `vcpe up --help` for usage\n"
	case "destroy":
		return "destroy is an alias for down (also requires --force) — run `vcpe down --help` for usage\n"
	}

	h, ok := commandHelp[command]
	if !ok {
		return fmt.Sprintf("unknown command %q — run `vcpe --help` for a list of commands\n", command)
	}

	var b strings.Builder

	// Usage line
	b.WriteString("Usage: vcpe ")
	b.WriteString(command)
	for _, req := range h.RequiredFlags {
		if req.Arg != "" {
			fmt.Fprintf(&b, " %s %s", req.Name, req.Arg)
		} else {
			fmt.Fprintf(&b, " %s", req.Name)
		}
	}
	for _, p := range h.Positionals {
		fmt.Fprintf(&b, " %s", p)
	}
	b.WriteString(" [flags]\n\n")

	b.WriteString(h.Description)
	b.WriteString("\n")

	if len(h.RequiredFlags) > 0 {
		b.WriteString("\nRequired flags:\n")
		for _, f := range h.RequiredFlags {
			line := "  " + f.Name
			if f.Arg != "" {
				line += " " + f.Arg
			}
			// pad to align descriptions
			const descCol = 26
			padding := descCol - len(line)
			if padding < 2 {
				padding = 2
			}
			fmt.Fprintf(&b, "%s%s%s\n", line, strings.Repeat(" ", padding), f.Description)
		}
	}

	if len(h.OptionalFlags) > 0 {
		b.WriteString("\nOptional flags:\n")
		for _, f := range h.OptionalFlags {
			line := "  " + f.Name
			if f.Arg != "" {
				line += " " + f.Arg
			}
			const descCol = 26
			padding := descCol - len(line)
			if padding < 2 {
				padding = 2
			}
			fmt.Fprintf(&b, "%s%s%s\n", line, strings.Repeat(" ", padding), f.Description)
		}
	}

	if len(h.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, ex := range h.Examples {
			fmt.Fprintf(&b, "  %s\n", ex)
		}
	}

	return b.String()
}
