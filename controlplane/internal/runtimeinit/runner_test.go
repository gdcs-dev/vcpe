package runtimeinit

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRunnerExecutesPhasesInDeterministicOrder(t *testing.T) {
	order := []PhaseName{}
	events := []PhaseEvent{}

	mark := func(name PhaseName) func(context.Context) error {
		return func(context.Context) error {
			order = append(order, name)
			return nil
		}
	}

	runner := Runner{
		Phases: StandardPhases(
			mark(PhaseInterfaceContract),
			mark(PhaseInterfaceReady),
			mark(PhaseIPv6Ready),
			mark(PhaseRuntimeConfig),
			mark(PhaseBootstrapSidefx),
			mark(PhaseServiceExec),
		),
		Emit: func(event PhaseEvent) {
			events = append(events, event)
		},
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("run phases: %v", err)
	}

	expectedOrder := []PhaseName{
		PhaseInterfaceContract,
		PhaseInterfaceReady,
		PhaseIPv6Ready,
		PhaseRuntimeConfig,
		PhaseBootstrapSidefx,
		PhaseServiceExec,
	}
	if !reflect.DeepEqual(order, expectedOrder) {
		t.Fatalf("unexpected execution order: want %v got %v", expectedOrder, order)
	}

	if len(events) != len(expectedOrder)*2 {
		t.Fatalf("expected start+success events per phase, got %d events", len(events))
	}
}

func TestRunnerStopsOnTerminalPhaseFailure(t *testing.T) {
	order := []PhaseName{}

	mark := func(name PhaseName) func(context.Context) error {
		return func(context.Context) error {
			order = append(order, name)
			return nil
		}
	}

	fail := func(context.Context) error {
		order = append(order, PhaseIPv6Ready)
		return errors.New("tentative address timeout")
	}

	runner := Runner{
		Phases: StandardPhases(
			mark(PhaseInterfaceContract),
			mark(PhaseInterfaceReady),
			fail,
			mark(PhaseRuntimeConfig),
			mark(PhaseBootstrapSidefx),
			mark(PhaseServiceExec),
		),
	}

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected runtime-init failure")
	}
	if !strings.Contains(err.Error(), "runtime-init phase ipv6_ready failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedOrder := []PhaseName{PhaseInterfaceContract, PhaseInterfaceReady, PhaseIPv6Ready}
	if !reflect.DeepEqual(order, expectedOrder) {
		t.Fatalf("expected execution to stop after failure: want %v got %v", expectedOrder, order)
	}
}
