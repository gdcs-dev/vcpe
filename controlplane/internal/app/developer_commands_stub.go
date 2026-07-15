//go:build homebrew

package app

import (
	"fmt"

	"github.com/gdcs-dev/vcpe/controlplane/internal/daemon"
)

// dispatchDeveloperCommand is the homebrew stub: developer commands (build,
// push, release) are not compiled into Homebrew installs. Any attempt to
// reach this path means the command was not in topLevelCommands and was
// already rejected by the CLI parser, but include a fallback for safety.
func dispatchDeveloperCommand(opts Options) (daemon.CommandResponse, error) {
	return daemon.CommandResponse{}, fmt.Errorf("command %q is not executable", opts.Command)
}
