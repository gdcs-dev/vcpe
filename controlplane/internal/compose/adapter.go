package compose

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Adapter struct {
	binary string
}

type Request struct {
	ComposeGroup string
	ProjectName  string
	WorkingDir   string
	ComposeFile  string
	EnvFile      string
	Timeout      time.Duration
	// Services, when non-empty, restricts the compose up command to the named
	// compose services. When empty, all services in the compose file are targeted.
	// Only valid for generated (non-curated) compose files.
	Services []string
	// RemoveOrphans, when true, adds --remove-orphans to compose up so that
	// containers for services removed from the compose file are cleaned up.
	// Only set this for generated compose files where service names are
	// controlled; curated compose files have fixed service names.
	RemoveOrphans bool
}

type OperationRecord struct {
	ComposeGroup     string   `json:"composeGroup"`
	ProjectName      string   `json:"projectName"`
	Command          []string `json:"command"`
	GeneratedInputs  []string `json:"generatedInputs"`
	RollbackEligible bool     `json:"rollbackEligible"`
}

func New() Adapter {
	return Adapter{binary: "podman-compose"}
}

func (a Adapter) Up(ctx context.Context, req Request) (OperationRecord, error) {
	args, err := commandArgs("up", req)
	if err != nil {
		return OperationRecord{}, err
	}
	if err := a.run(ctx, req, args); err != nil {
		return OperationRecord{}, err
	}
	return OperationRecord{ComposeGroup: req.ComposeGroup, ProjectName: req.ProjectName, Command: append([]string{a.binary}, args...), GeneratedInputs: generatedInputs(req), RollbackEligible: true}, nil
}

func (a Adapter) Down(ctx context.Context, req Request) (OperationRecord, error) {
	args, err := commandArgs("down", req)
	if err != nil {
		return OperationRecord{}, err
	}
	if err := a.run(ctx, req, args); err != nil {
		return OperationRecord{}, err
	}
	return OperationRecord{ComposeGroup: req.ComposeGroup, ProjectName: req.ProjectName, Command: append([]string{a.binary}, args...), GeneratedInputs: generatedInputs(req), RollbackEligible: false}, nil
}

func (a Adapter) Status(ctx context.Context, req Request) (OperationRecord, error) {
	args, err := commandArgs("ps", req)
	if err != nil {
		return OperationRecord{}, err
	}
	if err := a.run(ctx, req, args); err != nil {
		return OperationRecord{}, err
	}
	return OperationRecord{ComposeGroup: req.ComposeGroup, ProjectName: req.ProjectName, Command: append([]string{a.binary}, args...), GeneratedInputs: generatedInputs(req), RollbackEligible: false}, nil
}

func (a Adapter) run(ctx context.Context, req Request, args []string) error {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, a.binary, args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compose %s %s failed: %w (%s)", req.ComposeGroup, req.ProjectName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func commandArgs(command string, req Request) ([]string, error) {
	if req.ProjectName == "" {
		return nil, fmt.Errorf("compose project name is required")
	}
	args := []string{"-p", req.ProjectName}
	if req.EnvFile != "" {
		args = append(args, "--env-file", req.EnvFile)
	}
	if req.ComposeFile != "" {
		args = append(args, "-f", req.ComposeFile)
	}
	args = append(args, command)
	if command == "up" {
		args = append(args, "-d")
		if req.RemoveOrphans {
			args = append(args, "--remove-orphans")
		}
		// Append specific service names when provided (scale-up optimisation).
		args = append(args, req.Services...)
	}
	return args, nil
}

func generatedInputs(req Request) []string {
	inputs := []string{}
	if req.EnvFile != "" {
		inputs = append(inputs, req.EnvFile)
	}
	if req.ComposeFile != "" {
		inputs = append(inputs, req.ComposeFile)
	}
	return inputs
}
