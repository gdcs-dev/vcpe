package diagnostic

import (
	"testing"
	"time"
)

func TestApplyCausalityStateMatrix(t *testing.T) {
	edges := []Edge{{ID: "one", BlocksFollowing: true}, {ID: "two", BlocksFollowing: true}, {ID: "three", BlocksFollowing: true}}
	tests := []struct {
		name             string
		states           []State
		want             []State
		wantFirstFailure string
		wantErr          bool
	}{
		{name: "all pass", states: []State{StatePassed, StatePassed, StatePassed}, want: []State{StatePassed, StatePassed, StatePassed}},
		{name: "first fails", states: []State{StateFailed, StatePassed, StatePassed}, want: []State{StateFailed, StateSkipped, StateSkipped}, wantFirstFailure: "one"},
		{name: "second fails", states: []State{StatePassed, StateFailed, StateUnknown}, want: []State{StatePassed, StateFailed, StateSkipped}, wantFirstFailure: "two"},
		{name: "first unknown", states: []State{StateUnknown, StateFailed, StatePassed}, want: []State{StateUnknown, StateSkipped, StateSkipped}},
		{name: "second unknown", states: []State{StatePassed, StateUnknown, StatePassed}, want: []State{StatePassed, StateUnknown, StateSkipped}},
		{name: "unjustified skip", states: []State{StateSkipped, StatePassed, StatePassed}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observations := make([]Observation, len(edges))
			for index := range edges {
				observations[index] = Observation{EdgeID: edges[index].ID, State: test.states[index]}
			}
			got, firstFailure, err := ApplyCausality(edges, observations, time.Now().UTC())
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ApplyCausality: %v", err)
			}
			if firstFailure != test.wantFirstFailure {
				t.Errorf("first failure = %q, want %q", firstFailure, test.wantFirstFailure)
			}
			for index, state := range test.want {
				if got[index].State != state {
					t.Errorf("state %d = %q, want %q", index, got[index].State, state)
				}
			}
		})
	}
}

func TestApplyCausalityContinuesAfterInformationalUnknown(t *testing.T) {
	edges := []Edge{{ID: "application", BlocksFollowing: false}, {ID: "dns", BlocksFollowing: true}}
	observations := []Observation{{EdgeID: "application", State: StateUnknown}, {EdgeID: "dns", State: StatePassed}}
	got, firstFailure, err := ApplyCausality(edges, observations, time.Now().UTC())
	if err != nil {
		t.Fatalf("ApplyCausality: %v", err)
	}
	if got[0].State != StateUnknown || got[1].State != StatePassed || firstFailure != "" {
		t.Fatalf("observations = %+v, firstFailure = %q", got, firstFailure)
	}
}

func TestClassify(t *testing.T) {
	result := validResult()
	if got, err := Classify(result); err != nil || got != OutcomePassed {
		t.Fatalf("Classify(healthy) = %q, %v", got, err)
	}
	result.Observations[0].State = StateUnknown
	result.Observations[1].State = StateSkipped
	if got, err := Classify(result); err != nil || got != OutcomeInconclusive {
		t.Fatalf("Classify(unknown) = %q, %v", got, err)
	}
	result = validResult()
	result.Observations[1].State = StateFailed
	result.FirstFailure = "talaria-dns"
	if got, err := Classify(result); err != nil || got != OutcomeFailed {
		t.Fatalf("Classify(failed) = %q, %v", got, err)
	}
}
