package diagnostic

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

const (
	argusWebhookBucket    = "webhooks"
	defaultArgusURL       = "http://127.0.0.1:6600"
	defaultArgusBasicAuth = "Basic dXNlcjpwYXNz"
	defaultCaduceusURL    = "http://127.0.0.1:6000/api/v4/notify"
	defaultCaduceusAuth   = "Basic dXNlcjpwYXNz"
	webhookDuration       = 12 * time.Hour
	webhookRefreshPeriod  = 6 * time.Hour
)

var (
	ErrWebhookCandidateLimit        = errors.New("Argus webhook candidate limit exceeded")
	ErrWebhookRegistrationMissing   = errors.New("Argus webhook registration is missing")
	ErrWebhookRegistrationAmbiguous = errors.New("Argus webhook registration is ambiguous")
	ErrWebhookRegistrationExpired   = errors.New("Argus webhook registration is expired")
	ErrWebhookRegistrationStale     = errors.New("Argus webhook registration is stale")
	ErrActiveCallbackNotPermitted   = errors.New("active callback is not permitted")
	ErrCallbackURLInvalid           = errors.New("stored callback URL is invalid")
	ErrCallbackDNS                  = errors.New("stored callback hostname did not resolve")
	ErrCallbackTransport            = errors.New("stored callback TCP connection failed")
	ErrCallbackSecretUnavailable    = errors.New("stored callback secret is unavailable")
	ErrCallbackRejected             = errors.New("diagnostic callback was rejected")
	ErrWebhookEventFilterInvalid    = errors.New("stored webhook event filter is invalid")
	ErrWebhookEventMismatch         = errors.New("representative event does not match webhook filter")
	ErrWebhookDeviceMatcherInvalid  = errors.New("stored webhook device matcher is invalid")
	ErrWebhookDeviceMismatch        = errors.New("representative device does not match webhook matcher")
	ErrCaduceusIngestionRejected    = errors.New("Caduceus rejected synthetic diagnostic event")
)

// WebhookCandidate is the bounded, non-secret representation of an
// authoritative Argus webhook registration. The stored callback secret stays
// private to the source-local probe for later active operations.
type WebhookCandidate struct {
	Fingerprint    string
	TTLSeconds     int64
	TTLKnown       bool
	CallbackURL    string
	EventFilters   []string
	DeviceMatchers []string
	ContentType    string
	Duration       time.Duration
	Until          time.Time
	SecretPresent  bool
	secret         string
}

// WebhookRegistrationIntent is the bounded subscriber-owned registration
// contract used for comparison with an Argus candidate. The optional secret is
// memory-only and deliberately excluded from every serialized representation.
type WebhookRegistrationIntent struct {
	CallbackURL      string
	EventFilter      string
	DeviceMatcher    string
	ContentType      string
	SecretConfigured bool
	secret           string
}

// Validate confirms that subscriber intent can safely be forwarded to WebPA
// without carrying callback credentials, query values, or secret material.
func (intent WebhookSubscriberIntent) Validate() error {
	if intent.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported subscriber intent schema version %q", intent.SchemaVersion)
	}
	if intent.Journey != "webhook-subscriber" {
		return fmt.Errorf("unsupported subscriber intent journey %q", intent.Journey)
	}
	if intent.ObservedAt.IsZero() {
		return fmt.Errorf("subscriber intent observation time is required")
	}
	if len(intent.CallbackURL) == 0 || len(intent.CallbackURL) > MaxCallbackURLLength {
		return fmt.Errorf("subscriber callback URL is invalid")
	}
	parsed, err := url.Parse(intent.CallbackURL)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("subscriber callback URL is invalid")
	}
	if _, err := NormalizeCallbackIdentity(intent.CallbackURL); err != nil {
		return fmt.Errorf("subscriber callback URL is invalid")
	}
	if err := validateText("subscriber event filter", intent.EventFilter, MaxInvocationTextLength, true); err != nil {
		return err
	}
	if err := validateText("subscriber device matcher", intent.DeviceMatcher, MaxInvocationTextLength, true); err != nil {
		return err
	}
	if _, _, err := mime.ParseMediaType(intent.ContentType); err != nil || strings.TrimSpace(intent.ContentType) == "" {
		return fmt.Errorf("subscriber content type is invalid")
	}
	return validateOptionalID("subscriber last error category", intent.LastErrorCategory)
}

// WebhookProbe owns source-local Argus access from the WebPA namespace.
type WebhookProbe struct {
	ArgusURL         string
	BasicAuth        string
	CaduceusURL      string
	CaduceusAuth     string
	HTTPClient       *http.Client
	MaxItems         int
	Now              func() time.Time
	LookupHost       func(context.Context, string) ([]string, error)
	DialContext      func(context.Context, string, string) (net.Conn, error)
	getItems         func(context.Context) (chrysom.Items, error)
	newCorrelationID func() (string, error)
}

type callbackTarget struct {
	url     *url.URL
	host    string
	address string
}

type directCallbackResult struct {
	correlationID string
	httpStatus    int
}

type caduceusInjectionResult struct {
	correlationID string
	httpStatus    int
}

type caduceusDeliveryResult struct {
	ingestion caduceusInjectionResult
	receipt   Receipt
}

// NewWebhookProbeFromEnvironment creates a probe using WebPA-local Argus
// configuration. Credentials remain in process memory and are never returned.
func NewWebhookProbeFromEnvironment(timeout time.Duration) WebhookProbe {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return WebhookProbe{
		ArgusURL:     getenv("VCPE_ARGUS_URL", defaultArgusURL),
		BasicAuth:    getenv("VCPE_ARGUS_BASIC_AUTH", defaultArgusBasicAuth),
		CaduceusURL:  getenv("VCPE_CADUCEUS_URL", defaultCaduceusURL),
		CaduceusAuth: getenv("VCPE_CADUCEUS_BASIC_AUTH", defaultCaduceusAuth),
		HTTPClient:   &http.Client{Timeout: timeout},
		MaxItems:     MaxWebhookCandidates,
		Now:          func() time.Time { return time.Now().UTC() },
		LookupHost:   net.DefaultResolver.LookupHost,
		DialContext:  (&net.Dialer{Timeout: timeout}).DialContext,
	}
}

// Candidates lists and decodes the ownerless Argus webhooks bucket through
// the deployed chrysom/ancla model. It refuses oversized responses before a
// caller can inspect registration details.
func (probe WebhookProbe) Candidates(ctx context.Context) ([]WebhookCandidate, error) {
	probe.defaults()
	items, err := probe.items(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) > probe.MaxItems {
		return nil, fmt.Errorf("%w: %d exceeds %d", ErrWebhookCandidateLimit, len(items), probe.MaxItems)
	}
	candidates := make([]WebhookCandidate, 0, len(items))
	for _, item := range items {
		webhook, err := ancla.ItemToInternalWebhook(item)
		if err != nil {
			return nil, fmt.Errorf("decode Argus webhook %q: %w", item.ID, err)
		}
		candidate := WebhookCandidate{
			Fingerprint:    item.ID,
			CallbackURL:    webhook.Webhook.Config.URL,
			EventFilters:   append([]string(nil), webhook.Webhook.Events...),
			DeviceMatchers: append([]string(nil), webhook.Webhook.Matcher.DeviceID...),
			ContentType:    webhook.Webhook.Config.ContentType,
			Duration:       webhook.Webhook.Duration,
			Until:          webhook.Webhook.Until.UTC(),
			SecretPresent:  webhook.Webhook.Config.Secret != "",
			secret:         webhook.Webhook.Config.Secret,
		}
		if item.TTL != nil {
			candidate.TTLSeconds = *item.TTL
			candidate.TTLKnown = true
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

// EvaluateWebhookFreshness checks the stored duration and expiry against the
// event-sink registration policy. A registration with no full refresh window
// left is stale even when it has not technically expired.
func EvaluateWebhookFreshness(now time.Time, candidate WebhookCandidate) error {
	now = now.UTC()
	if candidate.Until.IsZero() || !candidate.Until.After(now) {
		return ErrWebhookRegistrationExpired
	}
	if candidate.TTLKnown && candidate.TTLSeconds <= 0 {
		return ErrWebhookRegistrationExpired
	}
	remaining := candidate.Until.Sub(now)
	if candidate.TTLKnown {
		ttl := time.Duration(candidate.TTLSeconds) * time.Second
		if ttl < remaining {
			remaining = ttl
		}
	}
	if candidate.Duration != webhookDuration || remaining <= webhookRefreshPeriod {
		return ErrWebhookRegistrationStale
	}
	return nil
}

// CompareWebhookConformance returns stable field names that differ between the
// subscriber's intended registration and the authoritative Argus candidate.
// It intentionally returns neither secret values nor fingerprints.
func CompareWebhookConformance(intent WebhookRegistrationIntent, candidate WebhookCandidate) []string {
	mismatches := make([]string, 0, 6)
	intentURL, intentURLErr := NormalizeCallbackIdentity(intent.CallbackURL)
	candidateURL, candidateURLErr := NormalizeCallbackIdentity(candidate.CallbackURL)
	if intentURLErr != nil || candidateURLErr != nil || intentURL != candidateURL {
		mismatches = append(mismatches, "callback-url")
	}
	if len(candidate.EventFilters) != 1 || candidate.EventFilters[0] != intent.EventFilter {
		mismatches = append(mismatches, "event-filter")
	}
	if len(candidate.DeviceMatchers) != 1 || candidate.DeviceMatchers[0] != intent.DeviceMatcher {
		mismatches = append(mismatches, "device-matcher")
	}
	if !sameContentType(intent.ContentType, candidate.ContentType) {
		mismatches = append(mismatches, "content-type")
	}
	if intent.SecretConfigured != candidate.SecretPresent {
		mismatches = append(mismatches, "secret-configured")
	} else if intent.secret != "" && candidate.secret != intent.secret {
		mismatches = append(mismatches, "secret")
	}
	return mismatches
}

// ValidateRepresentativeSelection confirms that the active diagnostic values
// will select the stored webhook before Caduceus is asked to inject traffic.
func ValidateRepresentativeSelection(candidate WebhookCandidate, event, deviceID string) error {
	eventMatches, err := matchesWebhookRegex(candidate.EventFilters, event)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookEventFilterInvalid, err)
	}
	if !eventMatches {
		return ErrWebhookEventMismatch
	}
	deviceMatches, err := matchesWebhookRegex(candidate.DeviceMatchers, deviceID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookDeviceMatcherInvalid, err)
	}
	if !deviceMatches {
		return ErrWebhookDeviceMismatch
	}
	return nil
}

func matchesWebhookRegex(patterns []string, value string) (bool, error) {
	if len(patterns) == 0 {
		return false, nil
	}
	for _, pattern := range patterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		if compiled.MatchString(value) {
			return true, nil
		}
	}
	return false, nil
}

func sameContentType(left, right string) bool {
	leftMediaType, leftParams, leftErr := mime.ParseMediaType(left)
	rightMediaType, rightParams, rightErr := mime.ParseMediaType(right)
	if leftErr != nil || rightErr != nil || !strings.EqualFold(leftMediaType, rightMediaType) || len(leftParams) != len(rightParams) {
		return false
	}
	for key, leftValue := range leftParams {
		rightValue, ok := rightParams[key]
		if !ok || leftValue != rightValue {
			return false
		}
	}
	return true
}

// NormalizeCallbackIdentity produces the non-sensitive identity used to match
// subscriber intent to stored registrations. Query values and userinfo are not
// identity inputs because the subscriber never exposes them for diagnostics.
func NormalizeCallbackIdentity(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid callback URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported callback URL scheme %q", parsed.Scheme)
	}
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port == "" || (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		parsed.Host = hostname
	} else {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

// MatchWebhookCandidate selects exactly one candidate by safe callback
// identity. It never chooses an arbitrary registration when duplicates exist.
func MatchWebhookCandidate(callbackURL string, candidates []WebhookCandidate) (WebhookCandidate, error) {
	if len(candidates) > MaxWebhookCandidates {
		return WebhookCandidate{}, fmt.Errorf("%w: %d exceeds %d", ErrWebhookCandidateLimit, len(candidates), MaxWebhookCandidates)
	}
	identity, err := NormalizeCallbackIdentity(callbackURL)
	if err != nil {
		return WebhookCandidate{}, err
	}
	matches := make([]WebhookCandidate, 0, 1)
	for _, candidate := range candidates {
		candidateIdentity, err := NormalizeCallbackIdentity(candidate.CallbackURL)
		if err != nil {
			return WebhookCandidate{}, fmt.Errorf("invalid stored callback identity: %w", err)
		}
		if candidateIdentity == identity {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return WebhookCandidate{}, ErrWebhookRegistrationMissing
	case 1:
		return matches[0], nil
	default:
		return WebhookCandidate{}, fmt.Errorf("%w: %d candidates", ErrWebhookRegistrationAmbiguous, len(matches))
	}
}

// RunWithInvocation performs the passive, source-local Argus lookup. Matching
// subscriber intent is supplied by the control plane in a later protocol step.
func (probe WebhookProbe) RunWithInvocation(ctx context.Context, invocation Invocation) EndpointResponse {
	now := probe.observedAt()
	if err := invocation.ValidateFor(JourneyWebhook); err != nil {
		return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{{EdgeID: "argus-reachability", State: StateUnknown, ReasonID: ReasonArgusUnreachable, RemediationID: RemediationCheckArgusReachability, Message: "webhook diagnostic invocation is invalid", ObservedAt: now}}}
	}
	candidates, err := probe.Candidates(ctx)
	if err != nil {
		if errors.Is(err, chrysom.ErrFailedAuthentication) {
			return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{
				{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
				{EdgeID: "argus-authentication", State: StateFailed, ReasonID: ReasonArgusAuthenticationFailed, RemediationID: RemediationCheckArgusCredentials, Message: "Argus rejected authentication", ObservedAt: now},
			}}
		}
		if errors.Is(err, ErrWebhookCandidateLimit) {
			return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{
				{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
				{EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now},
				{EdgeID: "registration-present", State: StateFailed, ReasonID: ReasonRegistrationAmbiguous, RemediationID: RemediationRemoveDuplicateHooks, Message: "Argus returned too many webhook registrations", ObservedAt: now},
			}}
		}
		return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{{EdgeID: "argus-reachability", State: StateFailed, ReasonID: ReasonArgusUnreachable, RemediationID: RemediationCheckArgusReachability, Message: "Argus webhook lookup failed", ObservedAt: now}}}
	}
	if invocation.SubscriberIntent != nil {
		inspection := probe.inspectWebhookRegistration(now, *invocation.SubscriberIntent, candidates)
		if !invocation.AllowActiveCallback || len(inspection.Observations) != 5 || inspection.Observations[4].State != StatePassed {
			return inspection
		}
		candidate, err := MatchWebhookCandidate(invocation.SubscriberIntent.CallbackURL, candidates)
		if err != nil {
			return inspection
		}
		switch invocation.ActivePhase {
		case WebhookActiveDirect:
			result, err := probe.sendDiagnosticCallback(ctx, invocation, candidate, nil)
			if err != nil {
				inspection.Observations = append(inspection.Observations, directCallbackFailureObservations(now, result, err)...)
				return inspection
			}
			inspection.Active = &WebhookActiveResult{Phase: WebhookActiveDirect, CorrelationID: result.correlationID, HTTPStatus: result.httpStatus}
		case WebhookActiveCaduceus:
			result, err := probe.injectCaduceusEvent(ctx, invocation, candidate, nil)
			if err != nil {
				inspection.Observations = append(inspection.Observations, caduceusInjectionFailureObservation(now, result, err))
				return inspection
			}
			inspection.Active = &WebhookActiveResult{Phase: WebhookActiveCaduceus, CorrelationID: result.correlationID, HTTPStatus: result.httpStatus}
		}
		return inspection
	}
	return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: []Observation{
		{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
		{EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now},
	}}
}

func (probe WebhookProbe) inspectWebhookRegistration(now time.Time, intent WebhookSubscriberIntent, candidates []WebhookCandidate) EndpointResponse {
	observations := []Observation{
		{EdgeID: "argus-reachability", State: StatePassed, ObservedAt: now},
		{EdgeID: "argus-authentication", State: StatePassed, ObservedAt: now},
	}
	candidate, err := MatchWebhookCandidate(intent.CallbackURL, candidates)
	if err != nil {
		reason, remediation, message := ReasonRegistrationMissing, RemediationRegisterWebhook, "no authoritative registration matched subscriber intent"
		if errors.Is(err, ErrWebhookRegistrationAmbiguous) || errors.Is(err, ErrWebhookCandidateLimit) {
			reason, remediation, message = ReasonRegistrationAmbiguous, RemediationRemoveDuplicateHooks, "multiple authoritative registrations matched subscriber intent"
		}
		return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: append(observations, Observation{EdgeID: "registration-present", State: StateFailed, ReasonID: reason, RemediationID: remediation, Message: message, ObservedAt: now})}
	}
	observations = append(observations, Observation{EdgeID: "registration-present", State: StatePassed, Evidence: []Evidence{{Key: "registration-fingerprint", Value: candidate.Fingerprint}}, ObservedAt: now})
	if err := EvaluateWebhookFreshness(now, candidate); err != nil {
		reason, remediation, message := ReasonRegistrationExpired, RemediationRefreshWebhook, "authoritative registration is expired"
		if errors.Is(err, ErrWebhookRegistrationStale) {
			reason, message = ReasonRegistrationStale, "authoritative registration is stale"
		}
		return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: append(observations, Observation{EdgeID: "registration-fresh", State: StateFailed, ReasonID: reason, RemediationID: remediation, Message: message, ObservedAt: now})}
	}
	observations = append(observations, Observation{EdgeID: "registration-fresh", State: StatePassed, ObservedAt: now})
	mismatches := CompareWebhookConformance(WebhookRegistrationIntent{CallbackURL: intent.CallbackURL, EventFilter: intent.EventFilter, DeviceMatcher: intent.DeviceMatcher, ContentType: intent.ContentType, SecretConfigured: intent.SecretConfigured}, candidate)
	if len(mismatches) > 0 {
		return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: append(observations, Observation{EdgeID: "registration-conformant", State: StateFailed, ReasonID: ReasonRegistrationMismatch, RemediationID: RemediationAlignWebhookConfig, Message: "authoritative registration differs in " + strings.Join(mismatches, ", "), ObservedAt: now})}
	}
	return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyWebhook, ObservedAt: now, Observations: append(observations, Observation{EdgeID: "registration-conformant", State: StatePassed, ObservedAt: now})}
}

func directCallbackFailureObservations(observedAt time.Time, result directCallbackResult, err error) []Observation {
	dnsFailed := Observation{EdgeID: "callback-dns", State: StateFailed, ReasonID: ReasonCallbackDNSFailed, RemediationID: RemediationCheckCallbackDNS, Message: "stored callback endpoint could not be resolved", ObservedAt: observedAt}
	if errors.Is(err, ErrCallbackDNS) || errors.Is(err, ErrCallbackURLInvalid) {
		return []Observation{dnsFailed}
	}
	passedDNS := passedWebhookObservation("callback-dns", observedAt, nil)
	transportFailed := Observation{EdgeID: "callback-transport", State: StateFailed, ReasonID: ReasonCallbackTransportFailed, RemediationID: RemediationCheckCallbackTransport, Message: "stored callback endpoint could not be reached", ObservedAt: observedAt}
	if errors.Is(err, ErrCallbackTransport) {
		return []Observation{passedDNS, transportFailed}
	}
	evidence := []Evidence(nil)
	if result.httpStatus != 0 {
		evidence = []Evidence{{Key: "http-status", Value: fmt.Sprint(result.httpStatus)}}
	}
	acceptanceFailed := Observation{EdgeID: "callback-acceptance", State: StateFailed, ReasonID: ReasonCallbackRejected, RemediationID: RemediationCheckCallbackSignature, Message: "diagnostic callback was not accepted", Evidence: evidence, ObservedAt: observedAt}
	return []Observation{passedDNS, passedWebhookObservation("callback-transport", observedAt, nil), acceptanceFailed}
}

func caduceusInjectionFailureObservation(observedAt time.Time, result caduceusInjectionResult, _ error) Observation {
	evidence := []Evidence(nil)
	if result.httpStatus != 0 {
		evidence = []Evidence{{Key: "http-status", Value: fmt.Sprint(result.httpStatus)}}
	}
	return Observation{EdgeID: "caduceus-ingestion", State: StateFailed, ReasonID: ReasonCaduceusIngestionRejected, RemediationID: RemediationCheckCaduceusIngestion, Message: "Caduceus did not accept the synthetic diagnostic event", Evidence: evidence, ObservedAt: observedAt}
}

// preflightDirectCallback checks the stored callback destination only after a
// valid active invocation and successful registration evaluation. It never
// accepts a caller-selected URL.
func (probe WebhookProbe) preflightDirectCallback(ctx context.Context, invocation Invocation, candidate WebhookCandidate, registrationErr error) (callbackTarget, error) {
	if registrationErr != nil || !invocation.AllowActiveCallback || invocation.ValidateFor(JourneyWebhook) != nil {
		return callbackTarget{}, ErrActiveCallbackNotPermitted
	}
	probe.defaults()
	if len(candidate.CallbackURL) == 0 || len(candidate.CallbackURL) > MaxCallbackURLLength {
		return callbackTarget{}, ErrCallbackURLInvalid
	}
	parsed, err := url.Parse(candidate.CallbackURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return callbackTarget{}, ErrCallbackURLInvalid
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, probe.timeout())
	defer cancel()
	addresses, err := probe.LookupHost(timeoutCtx, parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return callbackTarget{}, fmt.Errorf("%w: %v", ErrCallbackDNS, err)
	}
	address := net.JoinHostPort(parsed.Hostname(), port)
	connection, err := probe.DialContext(timeoutCtx, "tcp", address)
	if err != nil {
		return callbackTarget{}, fmt.Errorf("%w: %v", ErrCallbackTransport, err)
	}
	_ = connection.Close()
	return callbackTarget{url: parsed, host: parsed.Hostname(), address: address}, nil
}

// sendDiagnosticCallback sends exactly one signed, marked callback to the
// matched stored URL. The secret, signature, and body remain process-local.
func (probe WebhookProbe) sendDiagnosticCallback(ctx context.Context, invocation Invocation, candidate WebhookCandidate, registrationErr error) (directCallbackResult, error) {
	target, err := probe.preflightDirectCallback(ctx, invocation, candidate, registrationErr)
	if err != nil {
		return directCallbackResult{}, err
	}
	if candidate.secret == "" {
		return directCallbackResult{}, ErrCallbackSecretUnavailable
	}
	probe.defaults()
	correlationID, err := probe.newCorrelationID()
	if err != nil {
		return directCallbackResult{}, fmt.Errorf("generate callback correlation ID: %w", err)
	}
	body, err := json.Marshal(struct {
		Diagnostic    string `json:"vcpe_diagnostic"`
		CorrelationID string `json:"correlation_id"`
	}{Diagnostic: "webhook-registration-callback-diagnostics", CorrelationID: correlationID})
	if err != nil {
		return directCallbackResult{}, fmt.Errorf("encode diagnostic callback: %w", err)
	}
	if len(body) > MaxInvocationBodySize {
		return directCallbackResult{}, fmt.Errorf("diagnostic callback body exceeds limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.url.String(), strings.NewReader(string(body)))
	if err != nil {
		return directCallbackResult{}, fmt.Errorf("create diagnostic callback request: %w", err)
	}
	mac := hmac.New(sha1.New, []byte(candidate.secret))
	_, _ = mac.Write(body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Webpa-Signature", "sha1="+hex.EncodeToString(mac.Sum(nil)))
	client := *probe.HTTPClient
	client.Timeout = probe.timeout()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return directCallbackResult{}, fmt.Errorf("send diagnostic callback: %w", err)
	}
	defer response.Body.Close()
	result := directCallbackResult{correlationID: correlationID, httpStatus: response.StatusCode}
	if response.StatusCode != http.StatusNoContent {
		return result, fmt.Errorf("%w: HTTP %d", ErrCallbackRejected, response.StatusCode)
	}
	return result, nil
}

// injectCaduceusEvent sends one bounded WRP simple event through the local
// Caduceus ingress after active consent and registration selection succeed.
func (probe WebhookProbe) injectCaduceusEvent(ctx context.Context, invocation Invocation, candidate WebhookCandidate, registrationErr error) (caduceusInjectionResult, error) {
	if registrationErr != nil || !invocation.AllowActiveCallback || invocation.ValidateFor(JourneyWebhook) != nil {
		return caduceusInjectionResult{}, ErrActiveCallbackNotPermitted
	}
	if err := ValidateRepresentativeSelection(candidate, invocation.Event, invocation.DeviceID); err != nil {
		return caduceusInjectionResult{}, err
	}
	probe.defaults()
	correlationID, err := probe.newCorrelationID()
	if err != nil {
		return caduceusInjectionResult{}, fmt.Errorf("generate Caduceus correlation ID: %w", err)
	}
	payload, err := json.Marshal(struct {
		Diagnostic    string `json:"vcpe_diagnostic"`
		CorrelationID string `json:"correlation_id"`
	}{"webhook-registration-callback-diagnostics", correlationID})
	if err != nil {
		return caduceusInjectionResult{}, fmt.Errorf("encode Caduceus marker: %w", err)
	}
	message := wrp.Message{Type: wrp.SimpleEventMessageType, Source: invocation.DeviceID + "/vcpe-diagnostic", Destination: "event:" + invocation.Event + "/" + invocation.DeviceID, TransactionUUID: correlationID, ContentType: "application/json", Payload: payload}
	var encoded bytes.Buffer
	if err := wrp.NewEncoder(&encoded, wrp.Msgpack).Encode(&message); err != nil {
		return caduceusInjectionResult{}, fmt.Errorf("encode Caduceus WRP event: %w", err)
	}
	if encoded.Len() > MaxDiagnosticBodyBytes {
		return caduceusInjectionResult{}, fmt.Errorf("Caduceus WRP event exceeds limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, probe.CaduceusURL, &encoded)
	if err != nil {
		return caduceusInjectionResult{}, fmt.Errorf("create Caduceus request: %w", err)
	}
	request.Header.Set("Content-Type", wrpMsgpackContentType)
	request.Header.Set("Authorization", probe.CaduceusAuth)
	response, err := probe.HTTPClient.Do(request)
	if err != nil {
		return caduceusInjectionResult{}, fmt.Errorf("send Caduceus event: %w", err)
	}
	defer response.Body.Close()
	result := caduceusInjectionResult{correlationID: correlationID, httpStatus: response.StatusCode}
	if response.StatusCode != http.StatusAccepted {
		return result, fmt.Errorf("%w: HTTP %d", ErrCaduceusIngestionRejected, response.StatusCode)
	}
	return result, nil
}

// injectCaduceusAndPoll keeps Caduceus's queue acknowledgement distinct from
// the subscriber's later signed delivery receipt.
func (probe WebhookProbe) injectCaduceusAndPoll(ctx context.Context, client *Client, subscriber Target, invocation Invocation, candidate WebhookCandidate, registrationErr error) (caduceusDeliveryResult, error) {
	ingestion, err := probe.injectCaduceusEvent(ctx, invocation, candidate, registrationErr)
	if err != nil {
		return caduceusDeliveryResult{ingestion: ingestion}, err
	}
	receipt, err := client.PollReceipt(ctx, subscriber, ingestion.correlationID)
	if err != nil {
		return caduceusDeliveryResult{ingestion: ingestion}, err
	}
	if receipt.Source != "caduceus" {
		return caduceusDeliveryResult{ingestion: ingestion, receipt: receipt}, fmt.Errorf("unexpected Caduceus receipt source %q", receipt.Source)
	}
	return caduceusDeliveryResult{ingestion: ingestion, receipt: receipt}, nil
}

func (probe *WebhookProbe) defaults() {
	if probe.ArgusURL == "" {
		probe.ArgusURL = defaultArgusURL
	}
	if probe.BasicAuth == "" {
		probe.BasicAuth = defaultArgusBasicAuth
	}
	if probe.CaduceusURL == "" {
		probe.CaduceusURL = defaultCaduceusURL
	}
	if probe.CaduceusAuth == "" {
		probe.CaduceusAuth = defaultCaduceusAuth
	}
	if !strings.HasPrefix(probe.BasicAuth, "Basic ") {
		probe.BasicAuth = "Basic " + probe.BasicAuth
	}
	if probe.HTTPClient == nil {
		probe.HTTPClient = &http.Client{Timeout: 2 * time.Second}
	}
	if probe.MaxItems <= 0 {
		probe.MaxItems = MaxWebhookCandidates
	}
	if probe.Now == nil {
		probe.Now = func() time.Time { return time.Now().UTC() }
	}
	if probe.LookupHost == nil {
		probe.LookupHost = net.DefaultResolver.LookupHost
	}
	if probe.DialContext == nil {
		probe.DialContext = (&net.Dialer{Timeout: probe.timeout()}).DialContext
	}
	if probe.newCorrelationID == nil {
		probe.newCorrelationID = newDiagnosticCorrelationID
	}
}

func (probe WebhookProbe) timeout() time.Duration {
	if probe.HTTPClient != nil && probe.HTTPClient.Timeout > 0 {
		return probe.HTTPClient.Timeout
	}
	return 2 * time.Second
}

func newDiagnosticCorrelationID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (probe WebhookProbe) observedAt() time.Time {
	probe.defaults()
	return probe.Now().UTC()
}

func (probe WebhookProbe) items(ctx context.Context) (chrysom.Items, error) {
	if probe.getItems != nil {
		return probe.getItems(ctx)
	}
	client, err := chrysom.NewBasicClient(chrysom.BasicClientConfig{
		Address:    probe.ArgusURL,
		Bucket:     argusWebhookBucket,
		HTTPClient: probe.HTTPClient,
		Auth:       chrysom.Auth{Basic: probe.BasicAuth},
	}, func(context.Context) *zap.Logger { return zap.NewNop() })
	if err != nil {
		return nil, fmt.Errorf("create Argus client: %w", err)
	}
	return client.GetItems(ctx, "")
}
