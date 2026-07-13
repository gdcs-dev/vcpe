package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/config"
	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
	"github.com/gdcs-dev/vcpe/controlplane/internal/state"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types"
)

// ExecuteCLI is the process entrypoint for the vcpe binary.
// It parses the invocation, resolves the state root, and runs the command
// locally. The daemon path is opt-in via VCPE_DAEMON_SOCKET for environments
// that front the control plane with a long-running server.
func ExecuteCLI(prog string, args []string, version string) error {
	types.Register()

	opts, err := parseArgs(prog, args)
	if errors.Is(err, flag.ErrHelp) {
		if opts.Command == "" {
			fmt.Print(GlobalHelp())
		} else {
			fmt.Print(HelpFor(opts.Command))
		}
		return nil
	}
	if err != nil {
		return err
	}

	// Handle version before config/state/daemon resolution — it needs none of them.
	if opts.Command == "version" {
		fmt.Println(version)
		return nil
	}

	fileCfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	if opts.StateRoot == "" {
		opts.StateRoot = fileCfg.StateRoot
	}

	stateRoot, err := state.ResolveStateRoot(opts.StateRoot)
	if err != nil {
		return err
	}
	opts.StateRoot = stateRoot

	if socket := os.Getenv("VCPE_DAEMON_SOCKET"); socket != "" {
		return executeViaDaemon(socket, opts)
	}

	resp, err := executeLocal(opts)
	if err != nil {
		return err
	}
	if resp.Message != "" {
		fmt.Println(resp.Message)
	}
	return nil
}

// executeViaDaemon forwards a parsed invocation to a running daemon and prints
// its response. The --name selector is carried across the wire so the daemon
// targets the same deployment.
func executeViaDaemon(socket string, opts Options) error {
	client := daemon.NewClient(socket)
	resp, err := client.Execute(context.Background(), daemon.CommandRequest{
		Command:         opts.Command,
		ManifestPath:    opts.ManifestPath,
		Name:            opts.Name,
		AllowDisruptive: opts.AllowDisruptive,
		NoCache:         opts.NoCache,
		Force:           opts.Force,
		OutputJSON:      opts.OutputJSON,
	})
	if err != nil {
		return err
	}
	if resp.Message != "" {
		fmt.Println(resp.Message)
	}
	return nil
}
