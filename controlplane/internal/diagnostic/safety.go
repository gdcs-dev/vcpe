package diagnostic

import (
	"fmt"
	"regexp"
	"strings"
)

var allowedEvidenceKeys = map[string]struct{}{
	"client-evidence":  {},
	"client-service":   {},
	"device-id":        {},
	"endpoint":         {},
	"http-status":      {},
	"listener":         {},
	"parodus-endpoint": {},
	"resolved-address": {},
	"service-state":    {},
	"talaria-endpoint": {},
}

var sensitiveValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:basic|bearer)\s+[a-z0-9+/=_\-.]+`),
	regexp.MustCompile(`(?i)(?:password|passwd|secret|token|authorization|credential)\s*[:=]\s*[^\s,;]+`),
}

// Sanitize returns an output-safe deep copy. Oversized graph structure is
// rejected; provider-controlled text and evidence are bounded and redacted.
func Sanitize(result Result) (Result, error) {
	if len(result.Nodes) > MaxNodes || len(result.Edges) > MaxEdges || len(result.Observations) > MaxEdges {
		return Result{}, fmt.Errorf("diagnostic graph exceeds structural limits")
	}
	clean := result
	clean.Nodes = append([]Node(nil), result.Nodes...)
	clean.Edges = append([]Edge(nil), result.Edges...)
	clean.Metadata = sanitizeEvidence(result.Metadata)
	clean.Observations = make([]Observation, len(result.Observations))
	for index, observation := range result.Observations {
		observation.Message = redact(truncate(observation.Message, MaxMessageLength))
		observation.Evidence = sanitizeEvidence(observation.Evidence)
		clean.Observations[index] = observation
	}
	if err := clean.Validate(); err != nil {
		return Result{}, err
	}
	return clean, nil
}

func sanitizeEvidence(evidence []Evidence) []Evidence {
	if len(evidence) > MaxEvidencePerEdge {
		evidence = evidence[:MaxEvidencePerEdge]
	}
	clean := make([]Evidence, 0, len(evidence))
	seen := map[string]struct{}{}
	for _, entry := range evidence {
		if _, allowed := allowedEvidenceKeys[entry.Key]; !allowed {
			continue
		}
		if _, duplicate := seen[entry.Key]; duplicate {
			continue
		}
		seen[entry.Key] = struct{}{}
		clean = append(clean, Evidence{Key: entry.Key, Value: redact(truncate(entry.Value, MaxEvidenceValueLength))})
	}
	return clean
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	if maximum <= 3 {
		return value[:maximum]
	}
	return value[:maximum-3] + "..."
}

func redact(value string) string {
	clean := value
	for _, pattern := range sensitiveValuePatterns {
		clean = pattern.ReplaceAllString(clean, "[REDACTED]")
	}
	for _, marker := range []string{"password", "passwd", "secret", "token", "authorization", "credential"} {
		if strings.Contains(strings.ToLower(clean), marker) {
			return "[REDACTED]"
		}
	}
	return clean
}
