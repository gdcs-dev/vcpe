// Package diagnostic defines the versioned diagnostic graph shared by source
// endpoints and the control plane.
package diagnostic

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	SchemaVersion           = "vcpe.dev/diagnostic/v1"
	CapabilitiesSchema      = "vcpe.dev/diagnostics/v1"
	JourneyCPEWebPA         = "cpe-webpa"
	MaxNodes                = 16
	MaxEdges                = 16
	MaxEvidencePerEdge      = 8
	MaxCapabilities         = 16
	MaxIDLength             = 64
	MaxLabelLength          = 128
	MaxMessageLength        = 256
	MaxEvidenceValueLength  = 256
	MaxDiagnosticBodyBytes  = 64 * 1024
	MaxCapabilitiesBodySize = 4 * 1024
	MaxInvocationBodySize   = 1024
)

var stableIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// State is the observation state for one graph edge.
type State string

const (
	StatePassed  State = "passed"
	StateFailed  State = "failed"
	StateUnknown State = "unknown"
	StateSkipped State = "skipped"
)

// EndpointIdentity identifies one deployment service replica without exposing
// container-runtime details.
type EndpointIdentity struct {
	Deployment string `json:"deployment"`
	Service    string `json:"service"`
	Type       string `json:"type"`
	Replica    int    `json:"replica"`
}

// Node is one expected component in a diagnostic journey.
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// Edge is one ordered boundary between expected components.
type Edge struct {
	ID              string `json:"id"`
	From            string `json:"from"`
	To              string `json:"to"`
	Label           string `json:"label"`
	BlocksFollowing bool   `json:"blocksFollowing"`
}

// Evidence is one bounded, non-sensitive key/value observation.
type Evidence struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Observation records the source endpoint's result for one expected edge.
type Observation struct {
	EdgeID        string     `json:"edgeId"`
	State         State      `json:"state"`
	ReasonID      string     `json:"reasonId,omitempty"`
	RemediationID string     `json:"remediationId,omitempty"`
	Message       string     `json:"message,omitempty"`
	Evidence      []Evidence `json:"evidence,omitempty"`
	ObservedAt    time.Time  `json:"observedAt"`
}

// Result is the complete CPE-to-WebPA diagnostic graph.
type Result struct {
	SchemaVersion string           `json:"schemaVersion"`
	Journey       string           `json:"journey"`
	Source        EndpointIdentity `json:"source"`
	Target        EndpointIdentity `json:"target"`
	Metadata      []Evidence       `json:"metadata,omitempty"`
	Nodes         []Node           `json:"nodes"`
	Edges         []Edge           `json:"edges"`
	Observations  []Observation    `json:"observations"`
	FirstFailure  string           `json:"firstFailure,omitempty"`
	ObservedAt    time.Time        `json:"observedAt"`
}

// Capabilities is the passive response returned by GET /diagnostics.
type Capabilities struct {
	SchemaVersion string   `json:"schemaVersion"`
	Journeys      []string `json:"journeys"`
}

// EndpointResponse is the bounded source-local response returned by an active
// diagnostic route before the control plane merges expected graph metadata.
type EndpointResponse struct {
	SchemaVersion string        `json:"schemaVersion"`
	Journey       string        `json:"journey"`
	Observations  []Observation `json:"observations"`
	ObservedAt    time.Time     `json:"observedAt"`
}

// Invocation contains the only caller-selectable input for an active journey.
// The source still owns the WRP source, device identity, endpoint, credentials,
// and fixed Parodus service-status destination prefix.
type Invocation struct {
	ClientService string `json:"clientService"`
}

// Validate confirms that caller input is a single stable service identifier.
func (invocation Invocation) Validate() error {
	return validateID("client service", invocation.ClientService)
}

// Validate confirms that a result is bounded, internally consistent, and safe
// for deterministic rendering. It does not redact evidence; Sanitize owns that
// final output step.
func (r Result) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported diagnostic schema version %q", r.SchemaVersion)
	}
	if err := validateID("journey", r.Journey); err != nil {
		return err
	}
	if err := r.Source.validate("source"); err != nil {
		return err
	}
	if err := r.Target.validate("target"); err != nil {
		return err
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("diagnostic observation time is required")
	}
	if len(r.Nodes) == 0 || len(r.Nodes) > MaxNodes {
		return fmt.Errorf("diagnostic graph has %d nodes: expected 1..%d", len(r.Nodes), MaxNodes)
	}
	if len(r.Edges) == 0 || len(r.Edges) > MaxEdges {
		return fmt.Errorf("diagnostic graph has %d edges: expected 1..%d", len(r.Edges), MaxEdges)
	}
	if len(r.Observations) != len(r.Edges) {
		return fmt.Errorf("diagnostic graph has %d observations for %d edges", len(r.Observations), len(r.Edges))
	}
	if len(r.Metadata) > MaxEvidencePerEdge {
		return fmt.Errorf("diagnostic metadata has %d entries: maximum is %d", len(r.Metadata), MaxEvidencePerEdge)
	}
	if err := validateEvidence("diagnostic metadata", r.Metadata); err != nil {
		return err
	}

	nodes := make(map[string]struct{}, len(r.Nodes))
	for _, node := range r.Nodes {
		if err := validateID("node", node.ID); err != nil {
			return err
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			return fmt.Errorf("duplicate diagnostic node %q", node.ID)
		}
		nodes[node.ID] = struct{}{}
		if err := validateText("node label", node.Label, MaxLabelLength, true); err != nil {
			return err
		}
		if err := validateID("node kind", node.Kind); err != nil {
			return err
		}
	}

	edges := make(map[string]int, len(r.Edges))
	for index, edge := range r.Edges {
		if err := validateID("edge", edge.ID); err != nil {
			return err
		}
		if _, duplicate := edges[edge.ID]; duplicate {
			return fmt.Errorf("duplicate diagnostic edge %q", edge.ID)
		}
		edges[edge.ID] = index
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("diagnostic edge %q references unknown source node %q", edge.ID, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("diagnostic edge %q references unknown target node %q", edge.ID, edge.To)
		}
		if err := validateText("edge label", edge.Label, MaxLabelLength, true); err != nil {
			return err
		}
	}

	firstFailed := ""
	blocked := false
	for index, observation := range r.Observations {
		edge := r.Edges[index]
		if observation.EdgeID != edge.ID {
			return fmt.Errorf("observation %d references edge %q: expected %q", index, observation.EdgeID, edge.ID)
		}
		if !observation.State.valid() {
			return fmt.Errorf("observation for edge %q has invalid state %q", edge.ID, observation.State)
		}
		if observation.ObservedAt.IsZero() {
			return fmt.Errorf("observation time is required for edge %q", edge.ID)
		}
		if err := validateOptionalID("reason", observation.ReasonID); err != nil {
			return err
		}
		if err := validateOptionalID("remediation", observation.RemediationID); err != nil {
			return err
		}
		if err := validateText("observation message", observation.Message, MaxMessageLength, false); err != nil {
			return err
		}
		if len(observation.Evidence) > MaxEvidencePerEdge {
			return fmt.Errorf("observation for edge %q has %d evidence entries: maximum is %d", edge.ID, len(observation.Evidence), MaxEvidencePerEdge)
		}
		if err := validateEvidence("edge "+edge.ID, observation.Evidence); err != nil {
			return err
		}
		if blocked && observation.State != StateSkipped {
			return fmt.Errorf("observation for edge %q must be skipped after an unresolved prerequisite", edge.ID)
		}
		if observation.State == StateFailed && firstFailed == "" {
			firstFailed = edge.ID
		}
		if edge.BlocksFollowing && (observation.State == StateFailed || observation.State == StateUnknown) {
			blocked = true
		}
	}
	if r.FirstFailure != firstFailed {
		return fmt.Errorf("firstFailure %q does not match earliest failed edge %q", r.FirstFailure, firstFailed)
	}
	return nil
}

// Validate confirms that capability discovery is versioned, bounded, unique,
// and deterministic.
func (c Capabilities) Validate() error {
	if c.SchemaVersion != CapabilitiesSchema {
		return fmt.Errorf("unsupported diagnostics capability schema version %q", c.SchemaVersion)
	}
	if len(c.Journeys) > MaxCapabilities {
		return fmt.Errorf("diagnostic endpoint has %d capabilities: maximum is %d", len(c.Journeys), MaxCapabilities)
	}
	seen := map[string]struct{}{}
	for index, journey := range c.Journeys {
		if err := validateID("journey", journey); err != nil {
			return err
		}
		if _, duplicate := seen[journey]; duplicate {
			return fmt.Errorf("duplicate diagnostic journey %q", journey)
		}
		seen[journey] = struct{}{}
		if index > 0 && c.Journeys[index-1] > journey {
			return fmt.Errorf("diagnostic journeys must be sorted")
		}
	}
	return nil
}

// Validate confirms that an active endpoint response is versioned and bounded.
func (r EndpointResponse) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported diagnostic schema version %q", r.SchemaVersion)
	}
	if err := validateID("journey", r.Journey); err != nil {
		return err
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("diagnostic endpoint observation time is required")
	}
	if len(r.Observations) == 0 || len(r.Observations) > MaxEdges {
		return fmt.Errorf("diagnostic endpoint has %d observations: expected 1..%d", len(r.Observations), MaxEdges)
	}
	seen := map[string]struct{}{}
	for _, observation := range r.Observations {
		if err := validateID("observation edge", observation.EdgeID); err != nil {
			return err
		}
		if _, duplicate := seen[observation.EdgeID]; duplicate {
			return fmt.Errorf("duplicate endpoint observation for edge %q", observation.EdgeID)
		}
		seen[observation.EdgeID] = struct{}{}
		if !observation.State.valid() {
			return fmt.Errorf("observation for edge %q has invalid state %q", observation.EdgeID, observation.State)
		}
		if observation.ObservedAt.IsZero() {
			return fmt.Errorf("observation time is required for edge %q", observation.EdgeID)
		}
		if len(observation.Message) > MaxMessageLength || len(observation.Evidence) > MaxEvidencePerEdge {
			return fmt.Errorf("observation for edge %q exceeds diagnostic limits", observation.EdgeID)
		}
		if err := validateEvidence("edge "+observation.EdgeID, observation.Evidence); err != nil {
			return err
		}
	}
	return nil
}

func (e EndpointIdentity) validate(name string) error {
	if err := validateID(name+" deployment", e.Deployment); err != nil {
		return err
	}
	if err := validateID(name+" service", e.Service); err != nil {
		return err
	}
	if err := validateID(name+" type", e.Type); err != nil {
		return err
	}
	if e.Replica < 0 {
		return fmt.Errorf("%s replica must not be negative", name)
	}
	return nil
}

func (s State) valid() bool {
	return s == StatePassed || s == StateFailed || s == StateUnknown || s == StateSkipped
}

func validateID(name, value string) error {
	if len(value) == 0 || len(value) > MaxIDLength || !stableIDPattern.MatchString(value) {
		return fmt.Errorf("%s %q is not a stable identifier", name, value)
	}
	return nil
}

func validateOptionalID(name, value string) error {
	if value == "" {
		return nil
	}
	return validateID(name, value)
}

func validateText(name, value string, maximum int, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", name, maximum)
	}
	return nil
}

func validateEvidence(owner string, evidence []Evidence) error {
	seen := map[string]struct{}{}
	for _, entry := range evidence {
		if err := validateID("evidence key", entry.Key); err != nil {
			return err
		}
		if _, duplicate := seen[entry.Key]; duplicate {
			return fmt.Errorf("duplicate evidence key %q for %s", entry.Key, owner)
		}
		seen[entry.Key] = struct{}{}
		if err := validateText("evidence value", entry.Value, MaxEvidenceValueLength, false); err != nil {
			return err
		}
	}
	return nil
}
