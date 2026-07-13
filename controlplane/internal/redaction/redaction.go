package redaction

import "strings"

func Scrub(message string, secrets map[string]string) string {
	out := message
	for _, value := range secrets {
		if value == "" {
			continue
		}
		out = strings.ReplaceAll(out, value, "***REDACTED***")
	}
	return out
}
