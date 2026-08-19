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
	if result.Journey == JourneyParodusClients && result.ParodusClients != nil && result.ParodusClientsTruncated != nil {
		output.WriteString("\nRegistered clients:\n")
		if len(*result.ParodusClients) == 0 {
			output.WriteString("  (none)\n")
		} else {
			for _, client := range *result.ParodusClients {
				fmt.Fprintf(&output, "  %s\n", client)
			}
		}
		fmt.Fprintf(&output, "Truncated: %t\n", *result.ParodusClientsTruncated)
	}
	if result.Journey == JourneyArgusWebhooks && result.WebhookRegistrations != nil {
		output.WriteString("\nRegistered webhooks:\n")
		if len(*result.WebhookRegistrations) == 0 {
			output.WriteString("  (none)\n")
		} else {
			for _, registration := range *result.WebhookRegistrations {
				fmt.Fprintf(&output, "  %s\n", registration.Fingerprint)
				fmt.Fprintf(&output, "    Callback URL: %s\n", registration.CallbackURL)
				fmt.Fprintf(&output, "    Event filters: %s\n", formatRegistrationPatterns(registration.EventFilters))
				fmt.Fprintf(&output, "    Device matchers: %s\n", formatRegistrationPatterns(registration.DeviceMatchers))
				fmt.Fprintf(&output, "    Content type: %s\n", registration.ContentType)
				fmt.Fprintf(&output, "    Expires: %s\n", registration.Until.UTC().Format("2006-01-02T15:04:05Z07:00"))
				if registration.TTLSeconds != nil {
					fmt.Fprintf(&output, "    TTL seconds: %d\n", *registration.TTLSeconds)
				}
				fmt.Fprintf(&output, "    Secret configured: %t\n", registration.SecretPresent)
			}
		}
	}
	return strings.TrimRight(output.String(), "\n"), nil
}

func formatRegistrationPatterns(patterns []string) string {
	if len(patterns) == 0 {
		return "(none)"
	}
	return strings.Join(patterns, ", ")
}

func asciiHeading(journey string) string {
	if journey == JourneyCPEWebPACallback {
		return "CPE to callback diagnostic"
	}
	if journey == JourneyWebhook {
		return "Webhook diagnostic"
	}
	if journey == JourneyParodusClients {
		return "Parodus client enumeration"
	}
	if journey == JourneyArgusWebhooks {
		return "Argus webhook inventory"
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
