package servicecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit"
	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/contract"
)

type Config struct {
	Service     string
	DefaultExec []string
	RunCommand  func(context.Context, []string) error
}

func Run(ctx context.Context, cfg Config, args []string) error {
	if strings.TrimSpace(cfg.Service) == "" {
		return errors.New("runtime-init service name is required")
	}
	command, err := resolveCommand(cfg.DefaultExec, args)
	if err != nil {
		return err
	}

	runner := runtimeinit.Runner{
		Phases: runtimeinit.StandardPhases(
			func(context.Context) error { return validateStartupContract(cfg.Service) },
			func(context.Context) error { return nil },
			func(context.Context) error { return nil },
			func(context.Context) error { return validateRuntimeConfig(cfg.Service) },
			func(context.Context) error { return nil },
			func(inner context.Context) error {
				runnerFn := cfg.RunCommand
				if runnerFn == nil {
					runnerFn = runCommand
				}
				return runnerFn(inner, command)
			},
		),
	}
	return runner.Run(ctx)
}

func validateStartupContract(service string) error {
	contractPath := strings.TrimSpace(os.Getenv("VCPE_STARTUP_CONTRACT"))
	if contractPath == "" {
		if os.Getenv("VCPE_REQUIRE_STARTUP_CONTRACT") == "1" {
			return fmt.Errorf("runtime-init service %s missing VCPE_STARTUP_CONTRACT", service)
		}
		return nil
	}
	loaded, err := contract.Load(contractPath)
	if err != nil {
		return fmt.Errorf("runtime-init service %s startup contract %s: %w", service, contractPath, err)
	}
	if loaded.Service != service {
		return fmt.Errorf("runtime-init service %s startup contract service mismatch: %s", service, loaded.Service)
	}
	return nil
}

func validateRuntimeConfig(service string) error {
	runtimeConfig := strings.TrimSpace(os.Getenv("VCPE_RUNTIME_CONFIG_PATH"))
	if runtimeConfig == "" {
		return nil
	}
	if _, err := os.Stat(runtimeConfig); err != nil {
		return fmt.Errorf("runtime-init service %s runtime config %s: %w", service, runtimeConfig, err)
	}
	return nil
}

func resolveCommand(defaultExec, args []string) ([]string, error) {
	if len(args) > 0 {
		return append([]string(nil), args...), nil
	}
	if envExec := strings.TrimSpace(os.Getenv("VCPE_RUNTIME_INIT_EXEC")); envExec != "" {
		fields := strings.Fields(envExec)
		if len(fields) == 0 {
			return nil, errors.New("VCPE_RUNTIME_INIT_EXEC is set but empty")
		}
		return fields, nil
	}
	if len(defaultExec) > 0 {
		return append([]string(nil), defaultExec...), nil
	}
	return nil, errors.New("no runtime-init service command configured")
}

func runCommand(ctx context.Context, argv []string) error {
	// Resolve the executable path so syscall.Exec can find it.
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("exec %s: %w", argv[0], err)
	}
	// Replace the current process (PID 1 in a container) with the target
	// command. This is required for init-style processes (systemd, /sbin/init)
	// which refuse to start unless they are PID 1.
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		return fmt.Errorf("exec %s: %w", strings.Join(argv, " "), err)
	}
	// syscall.Exec never returns on success.
	return nil
}
