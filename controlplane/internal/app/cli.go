// Package app is the operator entrypoint for the vcpe control plane. It owns the
// CLI surface (argument parsing, command dispatch) and the apply orchestrator
// that turns a validated v1 manifest into reconciled Podman state through a
// journaled, rollback-capable pipeline.
//
// The deployment identity is metadata.name; commands that target an existing
// deployment select it with --name. There is no customer concept and no profile
// command surface.
package app

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

// Options is the fully parsed invocation. It is the single value threaded
// through dispatch and the local executor.
type Options struct {
	Command     string
	CommandArgs []string

	ManifestPath string
	StateRoot    string
	SocketPath   string
	ConfigPath   string

	// Name selects a target deployment (metadata.name) for down/destroy/logs/
	// status/service commands.
	Name string

	AllowDisruptive bool
	NoCache         bool
	Force           bool
	OutputJSON      bool
	Platforms       []string
	Backend         string
}

// topLevelCommands are the public operator commands.
var topLevelCommands = map[string]struct{}{
	"init":     {},
	"build":    {},
	"push":     {},
	"up":       {},
	"apply":    {},
	"down":     {},
	"destroy":  {},
	"plan":     {},
	"list":     {},
	"manifest": {},
	"status":   {},
	"logs":     {},
	"config":   {},
	"state":    {},
}

// retiredWrappers maps a legacy bash-wrapper command to the canonical vcpe
// command that replaces it, so users running the old grammar get an actionable
// migration hint instead of a silent failure.
var retiredWrappers = map[string]string{
	"bng":     "vcpe up --manifest <path>",
	"gateway": "vcpe up --manifest <path>",
	"webpa":   "vcpe up --manifest <path>",
	"routerd": "vcpe up --manifest <path>",
	"xb10":    "vcpe up --manifest <path>",
	"client":  "vcpe up --manifest <path>",
}

// extractHelpCommand scans args for -h or --help anywhere in the argument list.
// It extracts the resolved primary command name (first non-flag, non-value token,
// with aliases resolved) and returns (command, true) when help is requested.
// Values for --state-root, --config, and --socket are skipped with a one-step
// lookahead so they are not mistaken for a command token.
func extractHelpCommand(args []string) (string, bool) {
	hasHelp := false
	for _, a := range args {
		if a == "-h" || a == "--help" {
			hasHelp = true
			break
		}
	}
	if !hasHelp {
		return "", false
	}

	// Walk args to find the first token that is a primary or alias command.
	// Skip global flags and their values.
	valueFlags := map[string]struct{}{
		"--state-root": {},
		"--config":     {},
		"--socket":     {},
	}
	// canonical maps aliases to primary command names.
	aliasMap := map[string]string{
		"apply":   "up",
		"destroy": "down",
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if _, skip := valueFlags[a]; skip {
			i++ // skip value
			continue
		}
		if a == "-h" || a == "--help" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		// a is a candidate command token.
		if primary, ok := aliasMap[a]; ok {
			return primary, true
		}
		if _, ok := topLevelCommands[a]; ok {
			return a, true
		}
		// Not a recognised command — treat as no command found.
		break
	}
	return "", true
}

// isManifestCommand reports whether cmd requires a manifest to operate and
// should participate in manifest auto-discovery when --manifest is omitted.
func isManifestCommand(cmd string) bool {
	switch cmd {
	case "build", "push", "up", "apply", "plan":
		return true
	}
	return false
}

// resolveManifestPath populates opts.ManifestPath when it is empty and the
// command is manifest-consuming. The resolution algorithm is:
//
//  1. If the value looks like a path (contains "/" or ends in ".yaml"):
//     os.Stat; if found use it, otherwise return file-not-found.
//  2. If the value is a bare name (no "/" and no ".yaml" suffix):
//     search discovery directories for <name>.yaml.
//  3. If the value is empty:
//     discover all manifests; auto-select on exactly one, error otherwise.
//
// This function is called before validateCommandShape, so validateCommandShape
// can continue to require a non-empty ManifestPath unchanged.
func resolveManifestPath(opts *Options) error {
	if !isManifestCommand(opts.Command) {
		return nil
	}

	dirs := manifest.SearchDirs(os.Executable)

	switch {
	case opts.ManifestPath == "":
		// Auto-discover
		entries, err := manifest.FindAll(dirs)
		if err != nil {
			return fmt.Errorf("manifest discovery failed: %w", err)
		}
		switch len(entries) {
		case 0:
			return fmt.Errorf("no manifests found in search path; provide --manifest or run `vcpe manifest list`")
		case 1:
			opts.ManifestPath = entries[0].Path
		default:
			names := make([]string, len(entries))
			for i, e := range entries {
				names[i] = e.Name
			}
			return fmt.Errorf("multiple manifests found: %s; specify --manifest <name> or run `vcpe manifest list`",
				strings.Join(names, ", "))
		}

	case strings.Contains(opts.ManifestPath, "/") || strings.HasSuffix(opts.ManifestPath, ".yaml"):
		// Looks like a path — stat it; no fallback to name search
		if _, err := os.Stat(opts.ManifestPath); err != nil {
			return fmt.Errorf("manifest file not found: %s", opts.ManifestPath)
		}

	default:
		// Bare name — search discovery dirs
		path, err := manifest.Resolve(opts.ManifestPath, dirs)
		if err != nil {
			return fmt.Errorf("no manifest named %q found; run `vcpe manifest list` to see available manifests", opts.ManifestPath)
		}
		opts.ManifestPath = path
	}

	return nil
}

// parseArgs parses a vcpe invocation into Options. It validates flag/command
// combinations up front so the executor can assume a well-formed request.
// Global flags (--state-root/--socket/--config) may appear before or after the
// command; everything else is command-scoped.
func parseArgs(_ string, args []string) (Options, error) {
	// Upfront help scan — runs before any validation so that e.g.
	// `vcpe up --help` never produces a "requires --manifest" error.
	if cmd, ok := extractHelpCommand(args); ok {
		return Options{Command: cmd}, flag.ErrHelp
	}

	if len(args) == 0 {
		return Options{}, fmt.Errorf("a command is required; try `vcpe status`")
	}

	opts := Options{}

	// Consume leading global flags that precede the command.
	idx, err := consumeGlobalFlags(&opts, args)
	if err != nil {
		return Options{}, err
	}
	if idx >= len(args) {
		return Options{}, fmt.Errorf("a command is required; try `vcpe status`")
	}

	command := args[idx]
	rest := args[idx+1:]

	if command == "net" {
		return Options{}, fmt.Errorf("`vcpe net` has been removed; use vcpe up (apply) and vcpe status for verification")
	}
	if replacement, ok := retiredWrappers[command]; ok {
		return Options{}, fmt.Errorf("`vcpe %s` is no longer a top-level command; use %s", command, replacement)
	}
	if _, ok := topLevelCommands[command]; !ok {
		return Options{}, fmt.Errorf("unknown command %q", command)
	}

	opts.Command = command
	positional := []string{}

	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--manifest":
			val, next, err := takeValue(rest, i, "--manifest")
			if err != nil {
				return Options{}, err
			}
			opts.ManifestPath = val
			i = next
		case arg == "--name":
			val, next, err := takeValue(rest, i, "--name")
			if err != nil {
				return Options{}, err
			}
			opts.Name = val
			i = next
		case arg == "--state-root":
			val, next, err := takeValue(rest, i, "--state-root")
			if err != nil {
				return Options{}, err
			}
			opts.StateRoot = val
			i = next
		case arg == "--socket":
			val, next, err := takeValue(rest, i, "--socket")
			if err != nil {
				return Options{}, err
			}
			opts.SocketPath = val
			i = next
		case arg == "--config":
			val, next, err := takeValue(rest, i, "--config")
			if err != nil {
				return Options{}, err
			}
			opts.ConfigPath = val
			i = next
		case arg == "--allow-disruptive":
			opts.AllowDisruptive = true
		case arg == "--no-cache":
			opts.NoCache = true
		case arg == "--platform":
			val, next, err := takeValue(rest, i, "--platform")
			if err != nil {
				return Options{}, err
			}
			opts.Platforms = strings.Split(val, ",")
			i = next
		case arg == "--backend":
			val, next, err := takeValue(rest, i, "--backend")
			if err != nil {
				return Options{}, err
			}
			opts.Backend = val
			i = next
		case arg == "--force":
			opts.Force = true
		case arg == "--json":
			opts.OutputJSON = true
		case strings.HasPrefix(arg, "--"):
			return Options{}, fmt.Errorf("unknown flag %q for command %q", arg, command)
		default:
			positional = append(positional, arg)
		}
	}

	opts.CommandArgs = positional

	if opts.NoCache && command != "build" {
		return Options{}, fmt.Errorf("--no-cache is only supported for build")
	}
	if len(opts.Platforms) > 0 && command != "build" {
		return Options{}, fmt.Errorf("--platform is only supported for build")
	}
	if opts.Backend != "" && command != "build" && command != "push" {
		return Options{}, fmt.Errorf("--backend is only supported for build and push")
	}
	if opts.Backend != "" && opts.Backend != "podman" && opts.Backend != "docker" {
		return Options{}, fmt.Errorf("unknown backend %q: must be podman or docker", opts.Backend)
	}

	// Resolve --manifest (auto-discovery when omitted; bare-name lookup when set)
	// before validateCommandShape so that validation always sees a populated path.
	if err := resolveManifestPath(&opts); err != nil {
		return Options{}, err
	}

	if err := validateCommandShape(&opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

// validateCommandShape enforces per-command positional/flag grammar.
func validateCommandShape(opts *Options) error {
	switch opts.Command {
	case "up", "apply", "build", "plan", "push":
		if opts.ManifestPath == "" {
			return fmt.Errorf("%s requires --manifest <path>; run `vcpe %s --help` for usage", opts.Command, opts.Command)
		}
	case "down", "destroy":
		// --name is optional: if omitted, runDown auto-selects the single active
		// deployment or lists names when multiple exist.
		if opts.Command == "destroy" && !opts.Force {
			return fmt.Errorf("destroy requires --force to confirm teardown; run `vcpe down --help` for usage")
		}
	}
	return nil
}

// consumeGlobalFlags reads leading global flags (--state-root/--socket/--config)
// that precede the command and returns the index of the first non-global token.
func consumeGlobalFlags(opts *Options, args []string) (int, error) {
	idx := 0
	for idx < len(args) {
		switch args[idx] {
		case "--state-root":
			val, next, err := takeValue(args, idx, "--state-root")
			if err != nil {
				return 0, err
			}
			opts.StateRoot = val
			idx = next + 1
		case "--socket":
			val, next, err := takeValue(args, idx, "--socket")
			if err != nil {
				return 0, err
			}
			opts.SocketPath = val
			idx = next + 1
		case "--config":
			val, next, err := takeValue(args, idx, "--config")
			if err != nil {
				return 0, err
			}
			opts.ConfigPath = val
			idx = next + 1
		default:
			return idx, nil
		}
	}
	return idx, nil
}

// takeValue returns the value following a flag at index i, the new loop index,
// and an error when the value is missing.
func takeValue(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("flag %s requires a value", flag)
	}
	return args[i+1], i + 1, nil
}
