package diagnostic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Target is one persisted loopback diagnostic endpoint.
type Target struct {
	Host string
	Port int
}

// Client retrieves capability and active diagnostic responses exclusively over
// bounded HTTP.
type Client struct {
	HTTPClient *http.Client
}

// NewClient creates a diagnostic client with a bounded request duration.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{HTTPClient: &http.Client{Timeout: timeout}}
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
	if err := invocation.Validate(); err != nil {
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
