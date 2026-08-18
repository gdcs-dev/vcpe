package diagnostic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const defaultReceiptPollInterval = 100 * time.Millisecond

var ErrReceiptMissing = errors.New("diagnostic callback receipt is missing")

// Target is one persisted loopback diagnostic endpoint.
type Target struct {
	Host string
	Port int
}

// Client retrieves capability and active diagnostic responses exclusively over
// bounded HTTP.
type Client struct {
	HTTPClient          *http.Client
	ReceiptPollInterval time.Duration
}

// Receipt is the bounded state retained by a subscriber after it accepts a
// correctly signed diagnostic callback.
type Receipt struct {
	SchemaVersion string    `json:"schemaVersion"`
	CorrelationID string    `json:"correlationId"`
	Source        string    `json:"source"`
	AcceptedAt    time.Time `json:"acceptedAt"`
	HTTPStatus    int       `json:"httpStatus"`
}

// NewClient creates a diagnostic client with a bounded request duration.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{HTTPClient: &http.Client{Timeout: timeout}, ReceiptPollInterval: defaultReceiptPollInterval}
}

// Discover passively retrieves the journeys supported by a source instance.
func (client *Client) Discover(ctx context.Context, target Target) (Capabilities, error) {
	var capabilities Capabilities
	if err := client.getJSON(ctx, target, "/diagnostics", MaxCapabilitiesBodySize, &capabilities); err != nil {
		return Capabilities{}, err
	}
	if err := capabilities.Validate(); err != nil {
		return Capabilities{}, fmt.Errorf("invalid diagnostic capabilities: %w", err)
	}
	return capabilities, nil
}

// Run triggers one allowlisted active journey with bounded validated input.
func (client *Client) Run(ctx context.Context, target Target, journey string, invocation Invocation) (EndpointResponse, error) {
	if err := validateID("journey", journey); err != nil {
		return EndpointResponse{}, err
	}
	if err := invocation.ValidateFor(journey); err != nil {
		return EndpointResponse{}, err
	}
	endpoint, err := targetURL(target, "/diagnostics/"+journey)
	if err != nil {
		return EndpointResponse{}, err
	}
	body, err := json.Marshal(invocation)
	if err != nil {
		return EndpointResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return EndpointResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return EndpointResponse{}, fmt.Errorf("diagnostic request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return EndpointResponse{}, fmt.Errorf("diagnostic endpoint returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > MaxDiagnosticBodyBytes {
		return EndpointResponse{}, fmt.Errorf("diagnostic response exceeds %d bytes", MaxDiagnosticBodyBytes)
	}
	var result EndpointResponse
	if err := decodeBounded(response.Body, MaxDiagnosticBodyBytes, &result); err != nil {
		return EndpointResponse{}, fmt.Errorf("decode diagnostic response: %w", err)
	}
	if err := result.Validate(); err != nil {
		return EndpointResponse{}, fmt.Errorf("invalid diagnostic response: %w", err)
	}
	return result, nil
}

// SubscriberIntent retrieves the subscriber-owned registration contract from
// its persisted loopback endpoint.
func (client *Client) SubscriberIntent(ctx context.Context, target Target) (WebhookSubscriberIntent, error) {
	var intent WebhookSubscriberIntent
	if err := client.getJSON(ctx, target, "/diagnostics/webhook-subscriber/intent", MaxWebhookIntentBodySize, &intent); err != nil {
		return WebhookSubscriberIntent{}, err
	}
	if err := intent.Validate(); err != nil {
		return WebhookSubscriberIntent{}, fmt.Errorf("invalid webhook subscriber intent: %w", err)
	}
	return intent, nil
}

// RunWebhook sends validated subscriber intent and optional active inputs only
// to the resolved WebPA loopback endpoint.
func (client *Client) RunWebhook(ctx context.Context, target Target, intent WebhookSubscriberIntent, invocation Invocation) (EndpointResponse, error) {
	if err := intent.Validate(); err != nil {
		return EndpointResponse{}, fmt.Errorf("invalid webhook subscriber intent: %w", err)
	}
	if invocation.SubscriberIntent != nil {
		return EndpointResponse{}, fmt.Errorf("webhook invocation must not provide subscriber intent directly")
	}
	invocation.SubscriberIntent = &intent
	response, err := client.Run(ctx, target, JourneyWebhook, invocation)
	if err != nil || !invocation.AllowActiveCallback {
		return response, err
	}
	if response.Active == nil || response.Active.Phase != invocation.ActivePhase {
		return response, fmt.Errorf("webpa diagnostic response is missing active %q acknowledgement", invocation.ActivePhase)
	}
	return response, nil
}

// RunCPECallback invokes the selected CPE's one-event diagnostic route and
// requires an acknowledgement only when the source reports acceptance. A
// source-owned failed or unknown observation remains usable causal evidence.
func (client *Client) RunCPECallback(ctx context.Context, target Target, invocation Invocation) (EndpointResponse, error) {
	response, err := client.Run(ctx, target, JourneyCPEWebPACallback, invocation)
	if err != nil {
		return response, err
	}
	for _, observation := range response.Observations {
		if observation.EdgeID != "active-event-acceptance" {
			continue
		}
		if observation.State != StatePassed {
			return response, nil
		}
		if response.ActiveEvent != nil && response.ActiveEvent.Accepted {
			return response, nil
		}
		return response, fmt.Errorf("CPE diagnostic response is missing active event acknowledgement")
	}
	if response.ActiveEvent == nil || !response.ActiveEvent.Accepted {
		return response, fmt.Errorf("CPE diagnostic response is missing active event acknowledgement")
	}
	return response, nil
}

// ObserveRouting retrieves the selected Caduceus routing observation through
// WebPA's persisted loopback diagnostic endpoint. A missing record is a valid
// distinct result because it may have expired or been lost during a restart.
func (client *Client) ObserveRouting(ctx context.Context, target Target, correlationID string) (RoutingObservation, bool, error) {
	if err := validateCorrelationID(correlationID); err != nil {
		return RoutingObservation{}, false, err
	}
	endpoint, err := targetURL(target, "/diagnostics/"+JourneyCPEWebPACallback+"/routing")
	if err != nil {
		return RoutingObservation{}, false, err
	}
	body, err := json.Marshal(struct {
		CorrelationID string `json:"correlationId"`
	}{CorrelationID: correlationID})
	if err != nil {
		return RoutingObservation{}, false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return RoutingObservation{}, false, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return RoutingObservation{}, false, fmt.Errorf("routing observation request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return RoutingObservation{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return RoutingObservation{}, false, fmt.Errorf("routing observation endpoint returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > MaxInvocationBodySize {
		return RoutingObservation{}, false, fmt.Errorf("routing observation response exceeds %d bytes", MaxInvocationBodySize)
	}
	var observation RoutingObservation
	if err := decodeBounded(response.Body, MaxInvocationBodySize, &observation); err != nil {
		return RoutingObservation{}, false, fmt.Errorf("decode routing observation: %w", err)
	}
	if err := observation.Validate(correlationID); err != nil {
		return RoutingObservation{}, false, fmt.Errorf("invalid routing observation: %w", err)
	}
	return observation, true, nil
}

// PollReceipt waits for the bounded receipt state recorded by the subscriber.
// It polls only the resolved loopback endpoint and reports a missing receipt
// separately from transport or protocol failures.
func (client *Client) PollReceipt(ctx context.Context, target Target, correlationID string) (Receipt, error) {
	if err := validateCorrelationID(correlationID); err != nil {
		return Receipt{}, err
	}
	interval := client.receiptPollInterval()
	for attempt := 0; attempt < MaxReceiptPollAttempts; attempt++ {
		receipt, found, err := client.getReceipt(ctx, target, correlationID)
		if err != nil {
			return Receipt{}, err
		}
		if found {
			return receipt, nil
		}
		if attempt == MaxReceiptPollAttempts-1 {
			break
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return Receipt{}, ctx.Err()
		case <-timer.C:
		}
	}
	return Receipt{}, ErrReceiptMissing
}

func (client *Client) getReceipt(ctx context.Context, target Target, correlationID string) (Receipt, bool, error) {
	endpoint, err := targetURL(target, "/diagnostics/webhook-subscriber/receipts/"+url.PathEscape(correlationID))
	if err != nil {
		return Receipt{}, false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Receipt{}, false, err
	}
	response, err := client.httpClient().Do(request)
	if err != nil {
		return Receipt{}, false, fmt.Errorf("diagnostic receipt request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Receipt{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return Receipt{}, false, fmt.Errorf("diagnostic receipt endpoint returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > MaxInvocationBodySize {
		return Receipt{}, false, fmt.Errorf("diagnostic receipt response exceeds %d bytes", MaxInvocationBodySize)
	}
	var receipt Receipt
	if err := decodeBounded(response.Body, MaxInvocationBodySize, &receipt); err != nil {
		return Receipt{}, false, fmt.Errorf("decode diagnostic receipt: %w", err)
	}
	if err := receipt.validate(correlationID); err != nil {
		return Receipt{}, false, fmt.Errorf("invalid diagnostic receipt: %w", err)
	}
	return receipt, true, nil
}

func (client *Client) receiptPollInterval() time.Duration {
	if client != nil && client.ReceiptPollInterval > 0 {
		return client.ReceiptPollInterval
	}
	return defaultReceiptPollInterval
}

func (receipt Receipt) validate(correlationID string) error {
	if receipt.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported diagnostic schema version %q", receipt.SchemaVersion)
	}
	if receipt.CorrelationID != correlationID {
		return fmt.Errorf("correlation ID does not match request")
	}
	if err := validateCorrelationID(receipt.CorrelationID); err != nil {
		return err
	}
	if err := validateID("receipt source", receipt.Source); err != nil {
		return err
	}
	if receipt.Source != "direct" && receipt.Source != "caduceus" {
		return fmt.Errorf("unsupported receipt source %q", receipt.Source)
	}
	if receipt.AcceptedAt.IsZero() {
		return fmt.Errorf("receipt accepted time is required")
	}
	if receipt.HTTPStatus < http.StatusContinue || receipt.HTTPStatus > 599 {
		return fmt.Errorf("receipt HTTP status %d is invalid", receipt.HTTPStatus)
	}
	return nil
}

func (client *Client) getJSON(ctx context.Context, target Target, path string, maximum int64, output any) error {
	endpoint, err := targetURL(target, path)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := client.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("diagnostic capability request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("diagnostic capability endpoint returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maximum {
		return fmt.Errorf("diagnostic capability response exceeds %d bytes", maximum)
	}
	if err := decodeBounded(response.Body, maximum, output); err != nil {
		return fmt.Errorf("decode diagnostic capabilities: %w", err)
	}
	return nil
}

func (client *Client) httpClient() *http.Client {
	if client != nil && client.HTTPClient != nil {
		return client.HTTPClient
	}
	return NewClient(0).HTTPClient
}

func targetURL(target Target, path string) (string, error) {
	if target.Host != "127.0.0.1" || target.Port < 1 || target.Port > 65535 {
		return "", fmt.Errorf("invalid persisted diagnostic loopback endpoint")
	}
	return "http://" + net.JoinHostPort(target.Host, fmt.Sprint(target.Port)) + path, nil
}

func decodeBounded(reader io.Reader, maximum int64, output any) error {
	limited := &io.LimitedReader{R: reader, N: maximum + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		if limited.N <= 0 {
			return fmt.Errorf("response exceeds %d bytes", maximum)
		}
		return err
	}
	if limited.N <= 0 {
		return fmt.Errorf("response exceeds %d bytes", maximum)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("response contains multiple JSON values")
		}
		return err
	}
	return nil
}
