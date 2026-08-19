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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
)

// Options is the fully parsed invocation. It is the single value threaded
// through dispatch and the local executor.
type Options struct {
	Command     string
	CommandArgs []string

	ManifestPath  string
	ManifestPaths []string // repeatable --manifest with glob expansion; used by stamp and release
	StateRoot     string
	SocketPath    string
	ConfigPath    string

	// Name selects a target deployment (metadata.name) for down/destroy/logs/
	// status/service/diagnose commands. Down, status, and diagnose select the
	// sole active deployment when this is omitted.
	Name                string
	From                string
	To                  string
	ClientService       string
	Subscriber          string
	AllowActiveCallback bool
	AllowActiveEvent    bool
	Event               string
	DeviceID            string
	// Replica selects a zero-based source replica for diagnose. Nil means the
	// flag was omitted and permits automatic selection of a single replica.
	Replica *int
	// SubscriberReplica selects the zero-based event-sink replica for callback
	// diagnostics. Nil means automatic selection of a single replica.
	SubscriberReplica *int

	AllowDisruptive bool
	NoCache         bool
	Force           bool
	OutputJSON      bool
	Platforms       []string
	Backend         string
	OutputPath      string
	Version         string // release version tag (e.g. v0.2.0); required for release/stamp
}

// topLevelCommands are the public operator commands.
var topLevelCommands = map[string]struct{}{
	"init":     {},
	"up":       {},
	"apply":    {},
	"down":     {},
	"destroy":  {},
	"plan":     {},
	"list":     {},
	"manifest": {},
	"service":  {},
	"status":   {},
	"diagnose": {},
	"diag":     {},
	"logs":     {},
	"config":   {},
	"state":    {},
	"version":  {},
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
		"--version":    {},
	}
	// canonical maps aliases to primary command names.
	aliasMap := map[string]string{
		"apply":   "up",
		"destroy": "down",
		"diag":    "diagnose",
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
// stamp and release use ManifestPaths (repeatable + glob) instead and handle
// their own manifest resolution, so they are excluded here.
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
	if command == "diag" {
		command = "diagnose"
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
			// stamp and release: accumulate all --manifest values with glob expansion.
			// All other commands: keep single-path semantics.
			if command == "stamp" || command == "release" {
				expanded, err := expandManifestGlob(val)
				if err != nil {
					return Options{}, err
				}
				opts.ManifestPaths = append(opts.ManifestPaths, expanded...)
			} else {
				opts.ManifestPath = val
			}
			i = next
		case arg == "--name":
			val, next, err := takeValue(rest, i, "--name")
			if err != nil {
				return Options{}, err
			}
			opts.Name = val
			i = next
		case arg == "--from":
			val, next, err := takeValue(rest, i, "--from")
			if err != nil {
				return Options{}, err
			}
			opts.From = val
			i = next
		case arg == "--to":
			val, next, err := takeValue(rest, i, "--to")
			if err != nil {
				return Options{}, err
			}
			opts.To = val
			i = next
		case arg == "--client-service":
			val, next, err := takeValue(rest, i, "--client-service")
			if err != nil {
				return Options{}, err
			}
			opts.ClientService = val
			i = next
		case arg == "--subscriber":
			val, next, err := takeValue(rest, i, "--subscriber")
			if err != nil {
				return Options{}, err
			}
			opts.Subscriber = val
			i = next
		case arg == "--allow-active-callback":
			opts.AllowActiveCallback = true
		case arg == "--allow-active-event":
			opts.AllowActiveEvent = true
		case arg == "--event":
			val, next, err := takeValue(rest, i, "--event")
			if err != nil {
				return Options{}, err
			}
			opts.Event = val
			i = next
		case arg == "--device-id":
			val, next, err := takeValue(rest, i, "--device-id")
			if err != nil {
				return Options{}, err
			}
			opts.DeviceID = val
			i = next
		case arg == "--replica":
			val, next, err := takeValue(rest, i, "--replica")
			if err != nil {
				return Options{}, err
			}
			replica, err := strconv.Atoi(val)
			if err != nil || replica < 0 {
				return Options{}, fmt.Errorf("--replica must be a non-negative integer")
			}
			opts.Replica = &replica
			i = next
		case arg == "--subscriber-replica":
			val, next, err := takeValue(rest, i, "--subscriber-replica")
			if err != nil {
				return Options{}, err
			}
			replica, err := strconv.Atoi(val)
			if err != nil || replica < 0 {
				return Options{}, fmt.Errorf("--subscriber-replica must be a non-negative integer")
			}
			opts.SubscriberReplica = &replica
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
		case arg == "--output":
			val, next, err := takeValue(rest, i, "--output")
			if err != nil {
				return Options{}, err
			}
			opts.OutputPath = val
			i = next
		case arg == "--version":
			val, next, err := takeValue(rest, i, "--version")
			if err != nil {
				return Options{}, err
			}
			opts.Version = val
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
	if len(opts.Platforms) > 0 && command != "build" && command != "release" {
		return Options{}, fmt.Errorf("--platform is only supported for build and release")
	}
	if opts.Backend != "" && command != "build" && command != "push" && command != "release" {
		return Options{}, fmt.Errorf("--backend is only supported for build, push, and release")
	}
	if opts.Backend != "" && opts.Backend != "podman" && opts.Backend != "docker" {
		return Options{}, fmt.Errorf("unknown backend %q: must be podman or docker", opts.Backend)
	}
	if opts.Version != "" && command != "release" && command != "stamp" {
		return Options{}, fmt.Errorf("--version is only supported for release and stamp")
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
	case "release":
		if opts.Version == "" {
			return fmt.Errorf("release requires --version <vX.Y.Z>; run `vcpe release --help` for usage")
		}
		// --manifest is optional: auto-detect via git diff when omitted.
	case "stamp":
		if len(opts.ManifestPaths) == 0 {
			return fmt.Errorf("stamp requires at least one --manifest <path>; run `vcpe stamp --help` for usage")
		}
		if opts.Version == "" {
			return fmt.Errorf("stamp requires --version <vX.Y.Z>; run `vcpe stamp --help` for usage")
		}
	case "down", "destroy":
		// --name is optional: if omitted, runDown auto-selects the single active
		// deployment or lists names when multiple exist.
		if opts.Command == "destroy" && !opts.Force {
			return fmt.Errorf("destroy requires --force to confirm teardown; run `vcpe down --help` for usage")
		}
	case "diagnose":
		if opts.From == "" || opts.To == "" {
			return fmt.Errorf("diagnose requires --from <service> and --to <webpa|webhook|webhooks|callback|parodus>; run `vcpe diagnose --help` for usage")
		}
		switch opts.To {
		case "webpa":
			if opts.ClientService == "" {
				return fmt.Errorf("diagnose --to webpa requires --client-service <name>")
			}
			if err := (diagnostic.Invocation{ClientService: opts.ClientService}).ValidateFor(diagnostic.JourneyCPEWebPA); err != nil {
				return fmt.Errorf("invalid --client-service: %w", err)
			}
			if err := (diagnostic.Invocation{ClientService: opts.ClientService, AllowActiveCallback: opts.AllowActiveCallback, Event: opts.Event, DeviceID: opts.DeviceID}).ValidateFor(diagnostic.JourneyCPEWebPA); err != nil {
				return fmt.Errorf("invalid webpa diagnose options: %w", err)
			}
		case "webhook":
			if opts.ClientService != "" {
				return fmt.Errorf("--client-service is valid only for --to webpa")
			}
			if err := (diagnostic.Invocation{AllowActiveCallback: opts.AllowActiveCallback, Event: opts.Event, DeviceID: opts.DeviceID}).ValidateFor(diagnostic.JourneyWebhook); err != nil {
				return fmt.Errorf("invalid webhook diagnose options: %w", err)
			}
		case "callback":
			if opts.Subscriber == "" {
				return fmt.Errorf("diagnose --to callback requires --subscriber <service>")
			}
			// The source invocation requires an opaque correlation ID, but the
			// orchestrator generates it only after passive prerequisites pass.
			if err := (diagnostic.Invocation{ClientService: opts.ClientService, Subscriber: opts.Subscriber, AllowActiveCallback: opts.AllowActiveCallback, AllowActiveEvent: opts.AllowActiveEvent, Event: opts.Event, DeviceID: opts.DeviceID, CorrelationID: strings.Repeat("0", diagnostic.MaxCorrelationIDLength)}).ValidateFor(diagnostic.JourneyCPEWebPACallback); err != nil {
				return fmt.Errorf("invalid callback diagnose options: %w", err)
			}
		case "parodus":
			if opts.SubscriberReplica != nil {
				return fmt.Errorf("--subscriber-replica is valid only for --to callback")
			}
			if err := (diagnostic.Invocation{ClientService: opts.ClientService, Subscriber: opts.Subscriber, AllowActiveCallback: opts.AllowActiveCallback, AllowActiveEvent: opts.AllowActiveEvent, Event: opts.Event, DeviceID: opts.DeviceID}).ValidateFor(diagnostic.JourneyParodusClients); err != nil {
				return fmt.Errorf("invalid Parodus diagnose options: %w", err)
			}
		case "webhooks":
			if opts.SubscriberReplica != nil {
				return fmt.Errorf("--subscriber-replica is valid only for --to callback")
			}
			if err := (diagnostic.Invocation{ClientService: opts.ClientService, Subscriber: opts.Subscriber, AllowActiveCallback: opts.AllowActiveCallback, AllowActiveEvent: opts.AllowActiveEvent, Event: opts.Event, DeviceID: opts.DeviceID}).ValidateFor(diagnostic.JourneyArgusWebhooks); err != nil {
				return fmt.Errorf("invalid Argus webhook inventory options: %w", err)
			}
		default:
			return fmt.Errorf("diagnose --to must be webpa, webhook, webhooks, callback, or parodus")
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

// expandManifestGlob expands a single --manifest value into one or more file
// paths. Values containing glob metacharacters (*?[) are expanded with
// filepath.Glob; literal paths are stat-validated. Returns an error if no
// files match a glob or a literal path does not exist.
func expandManifestGlob(val string) ([]string, error) {
	isGlob := strings.ContainsAny(val, "*?[")
	if isGlob {
		matches, err := filepath.Glob(val)
		if err != nil {
			return nil, fmt.Errorf("--manifest glob %q: %w", val, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("--manifest glob %q matched no files", val)
		}
		return matches, nil
	}
	// Literal path: validate it exists.
	if _, err := os.Stat(val); err != nil {
		return nil, fmt.Errorf("--manifest %q: %w", val, err)
	}
	return []string{val}, nil
}
