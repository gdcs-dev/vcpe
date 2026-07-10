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
	"strings"
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
}

// topLevelCommands are the public operator commands.
var topLevelCommands = map[string]struct{}{
	"init":    {},
	"build":   {},
	"up":      {},
	"apply":   {},
	"down":    {},
	"destroy": {},
	"plan":    {},
	"list":    {},
	"status":  {},
	"logs":    {},
	"config":  {},
	"state":   {},
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

	if err := validateCommandShape(&opts); err != nil {
		return Options{}, err
	}
	return opts, nil
}

// validateCommandShape enforces per-command positional/flag grammar.
func validateCommandShape(opts *Options) error {
	switch opts.Command {
	case "up", "apply", "build", "plan":
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
