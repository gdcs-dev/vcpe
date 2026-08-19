package diagnostic

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var allowedEvidenceKeys = map[string]struct{}{
	"client-evidence":          {},
	"client-service":           {},
	"correlation-state":        {},
	"device-id":                {},
	"endpoint":                 {},
	"http-status":              {},
	"listener":                 {},
	"parodus-endpoint":         {},
	"participant-observed-at":  {},
	"registration-fingerprint": {},
	"resolved-address":         {},
	"service-state":            {},
	"talaria-endpoint":         {},
}

var allowedCorrelationStates = map[string]struct{}{
	"accepted":  {},
	"duplicate": {},
	"missing":   {},
	"recorded":  {},
	"rejected":  {},
	"restarted": {},
}

var sensitiveValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:basic|bearer)\s+[a-z0-9+/=_\-.]+`),
	regexp.MustCompile(`(?i)(?:password|passwd|secret|token|authorization|credential)\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`\b[a-f0-9]{64}\b`),
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
	if result.ParodusClients != nil {
		clients := append([]string(nil), (*result.ParodusClients)...)
		clean.ParodusClients = &clients
	}
	if result.ParodusClientsTruncated != nil {
		truncated := *result.ParodusClientsTruncated
		clean.ParodusClientsTruncated = &truncated
	}
	if result.WebhookRegistrations != nil {
		registrations := sanitizeWebhookRegistrations(*result.WebhookRegistrations)
		clean.WebhookRegistrations = &registrations
	}
	if result.TalariaDevices != nil {
		devices := sanitizeTalariaDevices(*result.TalariaDevices)
		clean.TalariaDevices = &devices
	}
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

func sanitizeWebhookRegistrations(registrations []WebhookRegistration) []WebhookRegistration {
	clean := make([]WebhookRegistration, len(registrations))
	for index, registration := range registrations {
		registration.EventFilters = sanitizeWebhookPatterns(registration.EventFilters)
		registration.DeviceMatchers = sanitizeWebhookPatterns(registration.DeviceMatchers)
		clean[index] = registration
	}
	return clean
}

func sanitizeWebhookPatterns(patterns []string) []string {
	clean := make([]string, len(patterns))
	for index, pattern := range patterns {
		clean[index] = redact(truncate(pattern, MaxInvocationTextLength))
	}
	sort.Strings(clean)
	return clean
}

func sanitizeTalariaDevices(devices []TalariaDevice) []TalariaDevice {
	clean := append([]TalariaDevice(nil), devices...)
	sort.Slice(clean, func(left, right int) bool {
		return clean[left].ID < clean[right].ID
	})
	return clean
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
		value := truncate(entry.Value, MaxEvidenceValueLength)
		if entry.Key != "registration-fingerprint" {
			value = redact(value)
		}
		clean = append(clean, Evidence{Key: entry.Key, Value: value})
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
