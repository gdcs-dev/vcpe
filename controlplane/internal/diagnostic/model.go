// Package diagnostic defines the versioned diagnostic graph shared by source
// endpoints and the control plane.
package diagnostic

import (
	"fmt"
	"mime"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	SchemaVersion                 = "vcpe.dev/diagnostic/v1"
	CapabilitiesSchema            = "vcpe.dev/diagnostics/v1"
	JourneyCPEWebPA               = "cpe-webpa"
	JourneyCPEWebPACallback       = "cpe-webpa-callback"
	JourneyParodusClients         = "parodus-clients"
	JourneyArgusWebhooks          = "argus-webhooks"
	JourneyWebhook                = "webhook"
	JourneyWebhookSubscriber      = "webhook-subscriber"
	WebhookActiveDirect           = "direct"
	WebhookActiveCaduceus         = "caduceus"
	MaxNodes                      = 16
	MaxEdges                      = 16
	MaxEvidencePerEdge            = 8
	MaxCapabilities               = 16
	MaxParodusClients             = 64
	MaxWebhookRegistrations       = 64
	MaxWebhookRegistrationFilters = 8
	MaxIDLength                   = 64
	MaxLabelLength                = 128
	MaxMessageLength              = 256
	MaxEvidenceValueLength        = 256
	MaxDiagnosticBodyBytes        = 64 * 1024
	MaxCapabilitiesBodySize       = 4 * 1024
	MaxInvocationBodySize         = 1024
	MaxInvocationTextLength       = 256
	MaxCallbackURLLength          = 2048
	MaxWebhookIntentBodySize      = 4 * 1024
	MaxWebhookCandidates          = 8
	MaxReceiptPollAttempts        = 5
	MaxCorrelationIDLength        = 64
)

var (
	stableIDPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	clientServicePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	eventDestinationPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)
	deviceIdentityPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]*:[a-z0-9][a-z0-9._-]*$`)
	registrationIDPattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

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
	SchemaVersion           string                 `json:"schemaVersion"`
	Journey                 string                 `json:"journey"`
	Source                  EndpointIdentity       `json:"source"`
	Target                  EndpointIdentity       `json:"target"`
	Metadata                []Evidence             `json:"metadata,omitempty"`
	Nodes                   []Node                 `json:"nodes"`
	Edges                   []Edge                 `json:"edges"`
	Observations            []Observation          `json:"observations"`
	ParodusClients          *[]string              `json:"parodusClients,omitempty"`
	ParodusClientsTruncated *bool                  `json:"parodusClientsTruncated,omitempty"`
	WebhookRegistrations    *[]WebhookRegistration `json:"webhookRegistrations,omitempty"`
	FirstFailure            string                 `json:"firstFailure,omitempty"`
	ObservedAt              time.Time              `json:"observedAt"`
}

// Capabilities is the passive response returned by GET /diagnostics.
type Capabilities struct {
	SchemaVersion string   `json:"schemaVersion"`
	Journeys      []string `json:"journeys"`
}

// EndpointResponse is the bounded source-local response returned by an active
// diagnostic route before the control plane merges expected graph metadata.
type EndpointResponse struct {
	SchemaVersion           string                 `json:"schemaVersion"`
	Journey                 string                 `json:"journey"`
	Observations            []Observation          `json:"observations"`
	ParodusClients          *[]string              `json:"parodusClients,omitempty"`
	ParodusClientsTruncated *bool                  `json:"parodusClientsTruncated,omitempty"`
	WebhookRegistrations    *[]WebhookRegistration `json:"webhookRegistrations,omitempty"`
	Active                  *WebhookActiveResult   `json:"active,omitempty"`
	ActiveEvent             *CPEActiveEventResult  `json:"activeEvent,omitempty"`
	ObservedAt              time.Time              `json:"observedAt"`
}

// Invocation contains the bounded caller-selectable inputs for active journeys.
// The source still owns endpoints, credentials, and other runtime configuration.
type Invocation struct {
	ClientService       string                   `json:"clientService,omitempty"`
	Subscriber          string                   `json:"subscriber,omitempty"`
	AllowActiveCallback bool                     `json:"allowActiveCallback,omitempty"`
	AllowActiveEvent    bool                     `json:"allowActiveEvent,omitempty"`
	Event               string                   `json:"event,omitempty"`
	DeviceID            string                   `json:"deviceId,omitempty"`
	CorrelationID       string                   `json:"correlationId,omitempty"`
	SubscriberIntent    *WebhookSubscriberIntent `json:"subscriberIntent,omitempty"`
	ActivePhase         string                   `json:"activePhase,omitempty"`
}

// WebhookActiveResult contains only the safe acknowledgement needed for the
// control plane to poll the subscriber after one WebPA-local active action.
type WebhookActiveResult struct {
	Phase         string `json:"phase"`
	CorrelationID string `json:"correlationId"`
	HTTPStatus    int    `json:"httpStatus"`
}

// CPEActiveEventResult acknowledges one source-owned marked event without
// serializing event content or its correlation identity.
type CPEActiveEventResult struct {
	Accepted bool `json:"accepted"`
}

// RoutingObservation is the bounded WebPA-owned result of looking up a
// selected Caduceus route for one opaque diagnostic correlation ID.
type RoutingObservation struct {
	SchemaVersion string    `json:"schemaVersion"`
	CorrelationID string    `json:"correlationId"`
	State         string    `json:"state"`
	ObservedAt    time.Time `json:"observedAt"`
}

// WebhookSubscriberIntent is the bounded, safe registration state collected
// from a webhook subscriber before contacting the WebPA participant.
type WebhookSubscriberIntent struct {
	SchemaVersion     string    `json:"schemaVersion"`
	Journey           string    `json:"journey"`
	ObservedAt        time.Time `json:"observedAt"`
	CallbackURL       string    `json:"callbackUrl"`
	EventFilter       string    `json:"eventFilter"`
	DeviceMatcher     string    `json:"deviceMatcher"`
	ContentType       string    `json:"contentType"`
	SecretConfigured  bool      `json:"secretConfigured"`
	InitialSuccessAt  time.Time `json:"initialSuccessAt,omitempty"`
	RefreshSuccessAt  time.Time `json:"refreshSuccessAt,omitempty"`
	RefreshFailureAt  time.Time `json:"refreshFailureAt,omitempty"`
	LastFailureAt     time.Time `json:"lastFailureAt,omitempty"`
	LastErrorCategory string    `json:"lastErrorCategory,omitempty"`
}

// WebhookRegistration is the bounded, non-secret representation of one
// authoritative Argus webhook record.
type WebhookRegistration struct {
	Fingerprint    string    `json:"fingerprint"`
	CallbackURL    string    `json:"callbackUrl"`
	EventFilters   []string  `json:"eventFilters"`
	DeviceMatchers []string  `json:"deviceMatchers"`
	ContentType    string    `json:"contentType"`
	Until          time.Time `json:"until"`
	TTLSeconds     *int64    `json:"ttlSeconds,omitempty"`
	SecretPresent  bool      `json:"secretPresent"`
}

// Validate preserves the CPE-to-WebPA validation contract for source-local
// handlers that predate journey-aware invocation routing.
func (invocation Invocation) Validate() error {
	return invocation.ValidateFor(JourneyCPEWebPA)
}

// ValidateFor confirms that caller input belongs exclusively to the selected
// journey and that active webhook traffic has explicit representative inputs.
func (invocation Invocation) ValidateFor(journey string) error {
	switch journey {
	case JourneyCPEWebPA:
		if invocation.Subscriber != "" || invocation.AllowActiveCallback || invocation.AllowActiveEvent || invocation.Event != "" || invocation.DeviceID != "" || invocation.CorrelationID != "" || invocation.SubscriberIntent != nil || invocation.ActivePhase != "" {
			return fmt.Errorf("webhook invocation fields are valid only for journey %q", JourneyWebhook)
		}
		return validateClientService(invocation.ClientService)
	case JourneyWebhook:
		if invocation.ClientService != "" || invocation.Subscriber != "" || invocation.AllowActiveEvent || invocation.CorrelationID != "" {
			return fmt.Errorf("client service is valid only for journey %q", JourneyCPEWebPA)
		}
		if invocation.SubscriberIntent != nil {
			if err := invocation.SubscriberIntent.Validate(); err != nil {
				return fmt.Errorf("invalid webhook subscriber intent: %w", err)
			}
		}
		if !invocation.AllowActiveCallback {
			if invocation.Event != "" || invocation.DeviceID != "" || invocation.ActivePhase != "" {
				return fmt.Errorf("event and device identity require active callback consent")
			}
			return nil
		}
		if err := validateInvocationText("event", invocation.Event, eventDestinationPattern); err != nil {
			return err
		}
		if err := validateInvocationText("device identity", invocation.DeviceID, deviceIdentityPattern); err != nil {
			return err
		}
		if invocation.ActivePhase != "" && invocation.ActivePhase != WebhookActiveDirect && invocation.ActivePhase != WebhookActiveCaduceus {
			return fmt.Errorf("active webhook phase %q is invalid", invocation.ActivePhase)
		}
		if invocation.SubscriberIntent != nil && invocation.ActivePhase == "" {
			return fmt.Errorf("active webhook phase is required when subscriber intent is forwarded")
		}
		return nil
	case JourneyCPEWebPACallback:
		if invocation.AllowActiveCallback || invocation.SubscriberIntent != nil || invocation.ActivePhase != "" {
			return fmt.Errorf("webhook invocation fields are valid only for journey %q", JourneyWebhook)
		}
		if !invocation.AllowActiveEvent {
			return fmt.Errorf("active event consent is required")
		}
		if err := validateClientService(invocation.ClientService); err != nil {
			return err
		}
		if err := validateID("subscriber", invocation.Subscriber); err != nil {
			return err
		}
		if err := validateInvocationText("event", invocation.Event, eventDestinationPattern); err != nil {
			return err
		}
		if err := validateInvocationText("device identity", invocation.DeviceID, deviceIdentityPattern); err != nil {
			return err
		}
		if err := validateCorrelationID(invocation.CorrelationID); err != nil {
			return err
		}
		return nil
	case JourneyParodusClients:
		if invocation.ClientService != "" || invocation.Subscriber != "" || invocation.AllowActiveCallback || invocation.AllowActiveEvent || invocation.Event != "" || invocation.DeviceID != "" || invocation.CorrelationID != "" || invocation.SubscriberIntent != nil || invocation.ActivePhase != "" {
			return fmt.Errorf("Parodus enumeration does not accept invocation fields")
		}
		return nil
	case JourneyArgusWebhooks:
		if invocation.ClientService != "" || invocation.Subscriber != "" || invocation.AllowActiveCallback || invocation.AllowActiveEvent || invocation.Event != "" || invocation.DeviceID != "" || invocation.CorrelationID != "" || invocation.SubscriberIntent != nil || invocation.ActivePhase != "" {
			return fmt.Errorf("Argus webhook inventory does not accept invocation fields")
		}
		return nil
	default:
		return fmt.Errorf("unsupported diagnostic journey %q", journey)
	}
}

// Validate confirms that a result is bounded, internally consistent, and safe
// for deterministic rendering. It does not redact evidence; Sanitize owns that
// final output step.
func (r Result) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported diagnostic schema version %q", r.SchemaVersion)
	}
	if !resultJourney(r.Journey) {
		return fmt.Errorf("unsupported diagnostic journey %q", r.Journey)
	}
	if err := r.Source.validate("source"); err != nil {
		return err
	}
	if err := r.Target.validate("target"); err != nil {
		return err
	}
	if err := validateParodusClientList(r.Journey, r.ParodusClients, r.ParodusClientsTruncated); err != nil {
		return err
	}
	if err := validateWebhookRegistrationList(r.Journey, r.WebhookRegistrations); err != nil {
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
	if !resultJourney(r.Journey) {
		return fmt.Errorf("unsupported diagnostic journey %q", r.Journey)
	}
	if err := validateParodusClientList(r.Journey, r.ParodusClients, r.ParodusClientsTruncated); err != nil {
		return err
	}
	if err := validateWebhookRegistrationList(r.Journey, r.WebhookRegistrations); err != nil {
		return err
	}
	if r.Active != nil {
		if r.Journey != JourneyWebhook {
			return fmt.Errorf("active result is valid only for journey %q", JourneyWebhook)
		}
		if err := r.Active.Validate(); err != nil {
			return fmt.Errorf("invalid active result: %w", err)
		}
	}
	if r.ActiveEvent != nil {
		if r.Journey != JourneyCPEWebPACallback {
			return fmt.Errorf("active event result is valid only for journey %q", JourneyCPEWebPACallback)
		}
		if !r.ActiveEvent.Accepted {
			return fmt.Errorf("active event result must acknowledge acceptance")
		}
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

// Validate confirms the active acknowledgement is bounded and correlatable.
func (result WebhookActiveResult) Validate() error {
	if result.Phase != WebhookActiveDirect && result.Phase != WebhookActiveCaduceus {
		return fmt.Errorf("unsupported active webhook phase %q", result.Phase)
	}
	if err := validateCorrelationID(result.CorrelationID); err != nil {
		return fmt.Errorf("active webhook %w", err)
	}
	if result.HTTPStatus < 100 || result.HTTPStatus > 599 {
		return fmt.Errorf("active webhook HTTP status %d is invalid", result.HTTPStatus)
	}
	return nil
}

// Validate confirms the routing observation contains only the allowed
// selected-routing outcome for the requested diagnostic correlation.
func (observation RoutingObservation) Validate(correlationID string) error {
	if observation.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported diagnostic schema version %q", observation.SchemaVersion)
	}
	if err := validateCorrelationID(correlationID); err != nil {
		return err
	}
	if observation.CorrelationID != correlationID {
		return fmt.Errorf("correlation ID does not match request")
	}
	if observation.State != "selected" {
		return fmt.Errorf("routing observation state %q is invalid", observation.State)
	}
	if observation.ObservedAt.IsZero() {
		return fmt.Errorf("routing observation time is required")
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

func validateClientService(value string) error {
	if len(value) == 0 || len(value) > MaxIDLength || !clientServicePattern.MatchString(value) {
		return fmt.Errorf("client service %q is not a stable identifier", value)
	}
	return nil
}

func validateParodusClientList(journey string, clients *[]string, truncated *bool) error {
	if journey != JourneyParodusClients {
		if clients != nil || truncated != nil {
			return fmt.Errorf("Parodus client list is valid only for journey %q", JourneyParodusClients)
		}
		return nil
	}
	if clients == nil && truncated == nil {
		return nil
	}
	if clients == nil || truncated == nil {
		return fmt.Errorf("Parodus client list and truncation state must be present together")
	}
	if len(*clients) > MaxParodusClients {
		return fmt.Errorf("Parodus client list has %d entries: maximum is %d", len(*clients), MaxParodusClients)
	}
	for index, client := range *clients {
		if err := validateClientService(client); err != nil {
			return fmt.Errorf("Parodus client %d: %w", index, err)
		}
		if index > 0 && (*clients)[index-1] > client {
			return fmt.Errorf("Parodus client list must be sorted")
		}
	}
	return nil
}

func validateWebhookRegistrationList(journey string, registrations *[]WebhookRegistration) error {
	if journey != JourneyArgusWebhooks {
		if registrations != nil {
			return fmt.Errorf("webhook registrations are valid only for journey %q", JourneyArgusWebhooks)
		}
		return nil
	}
	if registrations == nil {
		return nil
	}
	if len(*registrations) > MaxWebhookRegistrations {
		return fmt.Errorf("webhook registration list has %d entries: maximum is %d", len(*registrations), MaxWebhookRegistrations)
	}
	for index, registration := range *registrations {
		if err := registration.validate(); err != nil {
			return fmt.Errorf("webhook registration %d: %w", index, err)
		}
		if index > 0 && (*registrations)[index-1].Fingerprint >= registration.Fingerprint {
			return fmt.Errorf("webhook registration list must be sorted without duplicate fingerprints")
		}
	}
	return nil
}

func (registration WebhookRegistration) validate() error {
	if !registrationIDPattern.MatchString(registration.Fingerprint) {
		return fmt.Errorf("fingerprint is invalid")
	}
	normalizedURL, err := NormalizeCallbackIdentity(registration.CallbackURL)
	if err != nil || normalizedURL != registration.CallbackURL {
		return fmt.Errorf("callback URL is not a normalized safe identity")
	}
	if err := validateWebhookPatterns("event filter", registration.EventFilters); err != nil {
		return err
	}
	if err := validateWebhookPatterns("device matcher", registration.DeviceMatchers); err != nil {
		return err
	}
	if mediaType, _, err := mime.ParseMediaType(registration.ContentType); err != nil || mediaType == "" {
		return fmt.Errorf("content type is invalid")
	}
	if registration.Until.IsZero() {
		return fmt.Errorf("expiry time is required")
	}
	if registration.TTLSeconds != nil && *registration.TTLSeconds < 0 {
		return fmt.Errorf("TTL seconds must not be negative")
	}
	return nil
}

func validateWebhookPatterns(name string, values []string) error {
	if len(values) > MaxWebhookRegistrationFilters {
		return fmt.Errorf("%s list has %d entries: maximum is %d", name, len(values), MaxWebhookRegistrationFilters)
	}
	for index, value := range values {
		if err := validateText(name, value, MaxInvocationTextLength, true); err != nil {
			return err
		}
		if index > 0 && values[index-1] > value {
			return fmt.Errorf("%s list must be sorted", name)
		}
	}
	return nil
}

func validateCorrelationID(value string) error {
	if len(value) != MaxCorrelationIDLength || !registrationIDPattern.MatchString(value) {
		return fmt.Errorf("correlation ID is invalid")
	}
	return nil
}

func resultJourney(journey string) bool {
	return journey == JourneyCPEWebPA || journey == JourneyCPEWebPACallback || journey == JourneyParodusClients || journey == JourneyArgusWebhooks || journey == JourneyWebhook
}

func validateOptionalID(name, value string) error {
	if value == "" {
		return nil
	}
	return validateID(name, value)
}

func validateInvocationText(name, value string, pattern *regexp.Regexp) error {
	if len(value) == 0 || len(value) > MaxInvocationTextLength || !pattern.MatchString(value) {
		return fmt.Errorf("%s %q is invalid", name, value)
	}
	return nil
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
		if _, allowed := allowedEvidenceKeys[entry.Key]; !allowed {
			return fmt.Errorf("evidence key %q is not allowed for %s", entry.Key, owner)
		}
		if _, duplicate := seen[entry.Key]; duplicate {
			return fmt.Errorf("duplicate evidence key %q for %s", entry.Key, owner)
		}
		seen[entry.Key] = struct{}{}
		if err := validateText("evidence value", entry.Value, MaxEvidenceValueLength, false); err != nil {
			return err
		}
		if err := validateEvidenceValue(entry); err != nil {
			return err
		}
	}
	return nil
}

func validateEvidenceValue(entry Evidence) error {
	switch entry.Key {
	case "http-status":
		status, err := strconv.Atoi(entry.Value)
		if err != nil || status < 100 || status > 599 {
			return fmt.Errorf("HTTP status evidence %q is invalid", entry.Value)
		}
	case "registration-fingerprint":
		if !registrationIDPattern.MatchString(entry.Value) {
			return fmt.Errorf("registration fingerprint evidence is invalid")
		}
	case "correlation-state":
		if _, ok := allowedCorrelationStates[entry.Value]; !ok {
			return fmt.Errorf("correlation state evidence %q is invalid", entry.Value)
		}
	case "participant-observed-at":
		observedAt, err := time.Parse(time.RFC3339Nano, entry.Value)
		if err != nil || observedAt.IsZero() {
			return fmt.Errorf("participant observation time evidence %q is invalid", entry.Value)
		}
	}
	return nil
}
