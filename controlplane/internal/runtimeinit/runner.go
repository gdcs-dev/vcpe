package runtimeinit

import (
	"context"
	"fmt"
	"time"
)

type PhaseName string

const (
	PhaseInterfaceContract PhaseName = "interface_contract"
	PhaseInterfaceReady    PhaseName = "interface_ready"
	PhaseIPv6Ready         PhaseName = "ipv6_ready"
	PhaseRuntimeConfig     PhaseName = "runtime_config_applied"
	PhaseBootstrapSidefx   PhaseName = "bootstrap_side_effects"
	PhaseServiceExec       PhaseName = "service_exec"
)

type Phase struct {
	Name PhaseName
	Run  func(context.Context) error
}

type PhaseEvent struct {
	Time    time.Time `json:"time"`
	Phase   PhaseName `json:"phase"`
	Status  string    `json:"status"`
	Message string    `json:"message,omitempty"`
}

type Runner struct {
	Phases []Phase
	Emit   func(PhaseEvent)
}

func (r Runner) Run(ctx context.Context) error {
	for _, phase := range r.Phases {
		r.emit(PhaseEvent{Time: time.Now().UTC(), Phase: phase.Name, Status: "started"})
		if phase.Run == nil {
			err := fmt.Errorf("runtime-init phase %s has no handler", phase.Name)
			r.emit(PhaseEvent{Time: time.Now().UTC(), Phase: phase.Name, Status: "failed", Message: err.Error()})
			return err
		}
		if err := phase.Run(ctx); err != nil {
			wrapped := fmt.Errorf("runtime-init phase %s failed: %w", phase.Name, err)
			r.emit(PhaseEvent{Time: time.Now().UTC(), Phase: phase.Name, Status: "failed", Message: wrapped.Error()})
			return wrapped
		}
		r.emit(PhaseEvent{Time: time.Now().UTC(), Phase: phase.Name, Status: "succeeded"})
	}
	return nil
}

func (r Runner) emit(event PhaseEvent) {
	if r.Emit != nil {
		r.Emit(event)
	}
}

func StandardPhases(
	interfaceContract func(context.Context) error,
	interfaceReady func(context.Context) error,
	ipv6Ready func(context.Context) error,
	runtimeConfigApplied func(context.Context) error,
	bootstrapSideEffects func(context.Context) error,
	serviceExec func(context.Context) error,
) []Phase {
	return []Phase{
		{Name: PhaseInterfaceContract, Run: interfaceContract},
		{Name: PhaseInterfaceReady, Run: interfaceReady},
		{Name: PhaseIPv6Ready, Run: ipv6Ready},
		{Name: PhaseRuntimeConfig, Run: runtimeConfigApplied},
		{Name: PhaseBootstrapSidefx, Run: bootstrapSideEffects},
		{Name: PhaseServiceExec, Run: serviceExec},
	}
}
