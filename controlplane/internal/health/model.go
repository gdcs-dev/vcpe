// Package health defines the versioned health response shared by containers and
// the control-plane HTTP collector.
package health

import (
	"fmt"
	"strings"
	"time"
)

const (
	SchemaVersion    = "vcpe.dev/health/v1"
	MaxChecks        = 64
	MaxMessageLength = 256
)

// Status is the readiness state emitted by an in-container health endpoint.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusStarting  Status = "starting"
	StatusUnhealthy Status = "unhealthy"
)

// Check is one named, service-owned readiness observation.
type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

// Response is the complete response body returned from GET /health.
type Response struct {
	SchemaVersion string    `json:"schemaVersion"`
	Status        Status    `json:"status"`
	ObservedAt    time.Time `json:"observedAt"`
	Checks        []Check   `json:"checks"`
}

// Validate confirms that a response is safe to consume as the v1 health
// protocol. It intentionally does not infer aggregate state from checks; the
// in-container service owns that readiness decision.
func (r Response) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported health schema version %q", r.SchemaVersion)
	}
	if !r.Status.valid() {
		return fmt.Errorf("invalid health status %q", r.Status)
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("health observation time is required")
	}
	if len(r.Checks) > MaxChecks {
		return fmt.Errorf("health response has %d checks: maximum is %d", len(r.Checks), MaxChecks)
	}

	seen := make(map[string]struct{}, len(r.Checks))
	for _, check := range r.Checks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("health check name is required")
		}
		if _, duplicate := seen[check.Name]; duplicate {
			return fmt.Errorf("duplicate health check %q", check.Name)
		}
		seen[check.Name] = struct{}{}
		if !check.Status.valid() {
			return fmt.Errorf("health check %q has invalid status %q", check.Name, check.Status)
		}
		if len(check.Message) > MaxMessageLength {
			return fmt.Errorf("health check %q message exceeds %d bytes", check.Name, MaxMessageLength)
		}
	}
	return nil
}

func (s Status) valid() bool {
	return s == StatusHealthy || s == StatusStarting || s == StatusUnhealthy
}
