package diagnostic

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderJSON serializes the validated output-safe graph.
func RenderJSON(result Result) (string, error) {
	if err := result.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// RenderASCII produces deterministic, color-independent path output.
func RenderASCII(result Result) (string, error) {
	if err := result.Validate(); err != nil {
		return "", err
	}
	labels := make(map[string]string, len(result.Nodes))
	for _, node := range result.Nodes {
		labels[node.ID] = node.Label
	}
	var output strings.Builder
	fmt.Fprintf(&output, "%s: %s/%s[%d] -> %s\n\n", asciiHeading(result.Journey), result.Source.Deployment, result.Source.Service, result.Source.Replica, result.Target.Service)
	for index, edge := range result.Edges {
		observation := result.Observations[index]
		fmt.Fprintf(&output, "[%s] --%s--> [%s]  %s\n", labels[edge.From], strings.ToUpper(string(observation.State)), labels[edge.To], edge.Label)
	}
	if result.FirstFailure != "" {
		observation := observationByEdge(result, result.FirstFailure)
		fmt.Fprintf(&output, "\nFirst failure: %s", result.FirstFailure)
		if observation.Message != "" {
			fmt.Fprintf(&output, " - %s", observation.Message)
		}
		output.WriteByte('\n')
		writeObservationDetails(&output, observation)
	} else if unknown := firstUnknown(result); unknown != nil {
		fmt.Fprintf(&output, "\nInconclusive: %s", unknown.EdgeID)
		if unknown.Message != "" {
			fmt.Fprintf(&output, " - %s", unknown.Message)
		}
		output.WriteByte('\n')
		writeObservationDetails(&output, *unknown)
	} else {
		output.WriteString("\nResult: passed\n")
	}
	return strings.TrimRight(output.String(), "\n"), nil
}

func asciiHeading(journey string) string {
	if journey == JourneyCPEWebPACallback {
		return "CPE to callback diagnostic"
	}
	if journey == JourneyWebhook {
		return "Webhook diagnostic"
	}
	return "CPE to WebPA diagnostic"
}

func observationByEdge(result Result, edgeID string) Observation {
	for _, observation := range result.Observations {
		if observation.EdgeID == edgeID {
			return observation
		}
	}
	return Observation{}
}

func firstUnknown(result Result) *Observation {
	for index := range result.Observations {
		if result.Observations[index].State == StateUnknown {
			return &result.Observations[index]
		}
	}
	return nil
}

func writeObservationDetails(output *strings.Builder, observation Observation) {
	if observation.ReasonID != "" {
		fmt.Fprintf(output, "Reason: %s\n", observation.ReasonID)
	}
	if observation.RemediationID != "" {
		fmt.Fprintf(output, "Remediation: %s\n", observation.RemediationID)
	}
	for _, evidence := range observation.Evidence {
		fmt.Fprintf(output, "Evidence: %s=%s\n", evidence.Key, evidence.Value)
	}
}
