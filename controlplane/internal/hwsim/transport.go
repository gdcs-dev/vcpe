package hwsim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const managerPath = "/usr/libexec/vcpe/vcpe-hwsim"

type commandExecutor func(context.Context, string, ...string) (CommandResult, error)

type CommandTransport struct {
	command string
	prefix  []string
	run     commandExecutor
}

func NewCommandTransport(ctx context.Context) (*CommandTransport, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("podman"); err != nil {
			return nil, fmt.Errorf("wireless requires podman for machine delegation: %w", err)
		}
		return newCommandTransport("darwin", 0, false, false, true, false, runCommand)
	case "linux":
		wsl := isWSL()
		if wsl {
			return nil, fmt.Errorf("wireless radio attachments are not supported on WSL")
		}
		if os.Geteuid() != 0 {
			return nil, fmt.Errorf("wireless on Linux requires rootful Podman and root privileges")
		}
		if _, err := exec.LookPath("podman"); err != nil {
			return nil, fmt.Errorf("wireless requires rootful podman: %w", err)
		}
		rootless, err := podmanRootless(ctx)
		if err != nil {
			return nil, err
		}
		_, managerErr := exec.LookPath(managerPath)
		return newCommandTransport("linux", os.Geteuid(), false, rootless, true, managerErr == nil, runCommand)
	default:
		return nil, fmt.Errorf("wireless radio attachments are not supported on %s", runtime.GOOS)
	}
}

func newCommandTransport(goos string, euid int, wsl, rootless, hasPodman, hasManager bool, run commandExecutor) (*CommandTransport, error) {
	if run == nil {
		return nil, fmt.Errorf("hwsim command executor is not configured")
	}
	switch goos {
	case "darwin":
		if !hasPodman {
			return nil, fmt.Errorf("wireless requires podman for machine delegation")
		}
		return &CommandTransport{
			command: "podman",
			prefix:  []string{"machine", "ssh", "--", "sudo", managerPath},
			run:     run,
		}, nil
	case "linux":
		if wsl {
			return nil, fmt.Errorf("wireless radio attachments are not supported on WSL")
		}
		if euid != 0 || rootless {
			return nil, fmt.Errorf("wireless on Linux requires rootful Podman and root privileges")
		}
		if !hasPodman {
			return nil, fmt.Errorf("wireless requires rootful podman")
		}
		if !hasManager {
			return nil, fmt.Errorf("wireless manager %s is not installed", managerPath)
		}
		return &CommandTransport{command: managerPath, run: run}, nil
	default:
		return nil, fmt.Errorf("wireless radio attachments are not supported on %s", goos)
	}
}

func (t *CommandTransport) Execute(ctx context.Context, args ...string) (CommandResult, error) {
	if t == nil || t.run == nil || t.command == "" {
		return CommandResult{}, fmt.Errorf("hwsim command transport is not configured")
	}
	commandArgs := append(append([]string(nil), t.prefix...), args...)
	return t.run(ctx, t.command, commandArgs...)
}

func runCommand(ctx context.Context, name string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func podmanRootless(ctx context.Context) (bool, error) {
	result, err := runCommand(ctx, "podman", "info", "--format", "{{.Host.Security.Rootless}}")
	if err != nil {
		return false, fmt.Errorf("inspect podman mode: %w", err)
	}
	if result.ExitCode != 0 {
		return false, fmt.Errorf("inspect podman mode: exit %d (%s)", result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	switch strings.TrimSpace(string(result.Stdout)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("inspect podman mode: unexpected rootless value %q", strings.TrimSpace(string(result.Stdout)))
	}
}

func isWSL() bool {
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(release)), "microsoft")
}
