package diagnostic

import (
	"fmt"
	"time"
)

const (
	ReasonPrerequisiteFailed  = "prerequisite-failed"
	ReasonPrerequisiteUnknown = "prerequisite-unknown"
)

// ApplyCausality normalizes ordered endpoint observations. The first failed or
// unknown edge blocks all dependent edges; only an actual failure becomes the
// first causal failure.
func ApplyCausality(edges []Edge, observations []Observation, observedAt time.Time) ([]Observation, string, error) {
	if len(edges) != len(observations) {
		return nil, "", fmt.Errorf("cannot evaluate %d observations for %d edges", len(observations), len(edges))
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	normalized := make([]Observation, len(observations))
	firstFailure := ""
	blockedReason := ""
	for index, observation := range observations {
		if observation.EdgeID != edges[index].ID {
			return nil, "", fmt.Errorf("observation %d references edge %q: expected %q", index, observation.EdgeID, edges[index].ID)
		}
		if !observation.State.valid() {
			return nil, "", fmt.Errorf("observation for edge %q has invalid state %q", observation.EdgeID, observation.State)
		}
		observation.Evidence = append([]Evidence(nil), observation.Evidence...)
		if observation.ObservedAt.IsZero() {
			observation.ObservedAt = observedAt
		}
		if blockedReason != "" {
			observation.State = StateSkipped
			observation.ReasonID = blockedReason
			observation.RemediationID = ""
			observation.Message = "not evaluated because a prerequisite was unresolved"
			observation.Evidence = nil
			normalized[index] = observation
			continue
		}
		switch observation.State {
		case StateFailed:
			if firstFailure == "" {
				firstFailure = observation.EdgeID
			}
			if edges[index].BlocksFollowing {
				blockedReason = ReasonPrerequisiteFailed
			}
		case StateUnknown:
			if edges[index].BlocksFollowing {
				blockedReason = ReasonPrerequisiteUnknown
			}
		case StateSkipped:
			return nil, "", fmt.Errorf("edge %q is skipped without an unresolved prerequisite", observation.EdgeID)
		}
		normalized[index] = observation
	}
	return normalized, firstFailure, nil
}

// Outcome classifies a completed and validated diagnostic result.
type Outcome string

const (
	OutcomePassed       Outcome = "passed"
	OutcomeFailed       Outcome = "failed"
	OutcomeInconclusive Outcome = "inconclusive"
)

// Classify returns the completed journey's exit classification.
func Classify(result Result) (Outcome, error) {
	if err := result.Validate(); err != nil {
		return "", err
	}
	if result.FirstFailure != "" {
		return OutcomeFailed, nil
	}
	for _, observation := range result.Observations {
		if observation.State == StateUnknown || observation.State == StateSkipped {
			return OutcomeInconclusive, nil
		}
	}
	return OutcomePassed, nil
}
