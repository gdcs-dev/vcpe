package diagnostic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

var (
	ErrTalariaInventoryInvalid = errors.New("Talaria device inventory is invalid")
	ErrTalariaInventoryLimit   = errors.New("Talaria device inventory exceeds the diagnostic limit")
)

// CPEWebPAProbe owns source-local dependencies for the active journey. Tests
// replace these functions without requiring a container runtime.
type CPEWebPAProbe struct {
	TalariaURL    string
	ScytaleURL    string
	Username      string
	Password      string
	DeviceID      string
	ParodusClient string
	Timeout       time.Duration
	Now           func() time.Time
	ServiceState  func(context.Context) string
	LookupHost    func(context.Context, string) ([]string, error)
	DialContext   func(context.Context, string, string) (net.Conn, error)
	HTTPClient    *http.Client
}

// NewCPEWebPAProbeFromEnvironment uses the same endpoint, credential, and
// identity sources as the existing Gateway/XB10 health checks.
func NewCPEWebPAProbeFromEnvironment(timeout time.Duration) CPEWebPAProbe {
	talariaURL := getenv("VCPE_TALARIA_DEVICES_URL", "http://talaria:6200/api/v2/devices")
	username, password, _ := strings.Cut(getenv("VCPE_TALARIA_BASIC_AUTH", "user:pass"), ":")
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	return CPEWebPAProbe{
		TalariaURL: talariaURL,
		ScytaleURL: getenv("VCPE_SCYTALE_URL", "http://scytale:6300/api/v3/device"),
		Username:   username,
		Password:   password,
		DeviceID:   sourceDeviceID(),
		Timeout:    timeout,
		Now:        func() time.Time { return time.Now().UTC() },
		ServiceState: func(ctx context.Context) string {
			output, err := exec.CommandContext(ctx, "systemctl", "is-active", "parodus.service").Output()
			if err != nil {
				return "inactive"
			}
			return strings.TrimSpace(string(output))
		},
		LookupHost:  net.DefaultResolver.LookupHost,
		DialContext: dialer.DialContext,
		HTTPClient:  &http.Client{Timeout: timeout},
	}
}

// RunWithInvocation applies a validated caller-selected client service while
// retaining all source-owned connection and credential configuration.
func (probe CPEWebPAProbe) RunWithInvocation(parent context.Context, invocation Invocation) EndpointResponse {
	if err := invocation.Validate(); err != nil {
		now := time.Now().UTC()
		return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, ObservedAt: now, Observations: []Observation{{EdgeID: "application-parodus", State: StateUnknown, ReasonID: "parodus-client-invalid", ObservedAt: now}}}
	}
	probe.ParodusClient = invocation.ClientService
	probe.defaults()
	now := probe.Now()
	edges := []Edge{
		{ID: "application-parodus", BlocksFollowing: false},
		{ID: "talaria-dns", BlocksFollowing: true},
		{ID: "talaria-transport", BlocksFollowing: true},
		{ID: "talaria-authentication", BlocksFollowing: true},
		{ID: "device-registration", BlocksFollowing: true},
	}
	observations := make([]Observation, len(edges))
	for index := range edges {
		observations[index] = Observation{EdgeID: edges[index].ID, State: StatePassed, ObservedAt: now}
	}

	state := probe.ServiceState(parent)
	if state == "active" {
		observations[0] = ApplicationEvidenceUnavailable(now, state)
	} else {
		observations[0] = Observation{EdgeID: edges[0].ID, State: StateFailed, ReasonID: "parodus-inactive", RemediationID: "start-parodus", Message: "Parodus is not active", Evidence: []Evidence{{Key: "service-state", Value: state}}, ObservedAt: now}
	}

	parsed, err := url.Parse(probe.TalariaURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		observations[1] = failedObservation(edges[1].ID, "talaria-url-invalid", "check-talaria-configuration", "configured Talaria URL is invalid", now)
		return probe.finish(edges, observations, now)
	}
	addresses, err := probe.LookupHost(parent, parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		observations[1] = failedObservation(edges[1].ID, "talaria-dns-failed", "check-talaria-dns", "Talaria hostname did not resolve", now)
		return probe.finish(edges, observations, now)
	}
	observations[1].Evidence = []Evidence{{Key: "resolved-address", Value: addresses[0]}}

	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	connection, err := probe.DialContext(parent, "tcp", net.JoinHostPort(parsed.Hostname(), port))
	if err != nil {
		observations[2] = failedObservation(edges[2].ID, "talaria-transport-failed", "check-talaria-route", "Talaria TCP connection failed", now)
		return probe.finish(edges, observations, now)
	}
	_ = connection.Close()
	observations[2].Evidence = []Evidence{{Key: "endpoint", Value: net.JoinHostPort(parsed.Hostname(), port)}}

	request, _ := http.NewRequestWithContext(parent, http.MethodGet, probe.TalariaURL, nil)
	request.SetBasicAuth(probe.Username, probe.Password)
	response, err := probe.HTTPClient.Do(request)
	if err != nil {
		observations[3] = Observation{EdgeID: edges[3].ID, State: StateUnknown, ReasonID: "talaria-http-failed", RemediationID: "check-talaria-http", Message: "Talaria HTTP request failed after TCP connection", ObservedAt: now}
		return probe.finish(edges, observations, now)
	}
	defer response.Body.Close()
	observations[3].Evidence = []Evidence{{Key: "http-status", Value: fmt.Sprint(response.StatusCode)}}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		observations[3].State = StateFailed
		observations[3].ReasonID = "talaria-authentication-failed"
		observations[3].RemediationID = "check-talaria-credentials"
		observations[3].Message = "Talaria rejected authentication"
		return probe.finish(edges, observations, now)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		observations[3].State = StateUnknown
		observations[3].ReasonID = "talaria-http-status"
		observations[3].RemediationID = "check-talaria-service"
		observations[3].Message = "Talaria returned an unexpected HTTP status"
		return probe.finish(edges, observations, now)
	}

	var registry struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, MaxDiagnosticBodyBytes+1))
	if err := decoder.Decode(&registry); err != nil {
		observations[4] = failedObservation(edges[4].ID, "talaria-registry-invalid", "check-talaria-registry", "Talaria device registry response is invalid", now)
		return probe.finish(edges, observations, now)
	}
	registered := false
	for _, device := range registry.Devices {
		if device.ID == probe.DeviceID {
			registered = true
			break
		}
	}
	observations[4].Evidence = []Evidence{{Key: "device-id", Value: probe.DeviceID}}
	if !registered {
		observations[4].State = StateFailed
		observations[4].ReasonID = "device-registration-missing"
		observations[4].RemediationID = "check-parodus-registration"
		observations[4].Message = "expected device is not present in Talaria registry"
		return probe.finish(edges, observations, now)
	}
	if state == "active" {
		observations[0] = probe.observeParodusClient(parent, now)
	}
	return probe.finish(edges, observations, now)
}

// RunParodusClients retrieves the bounded list of receive-enabled clients from
// the source-owned Parodus instance without requiring a WebPA target.
func (probe CPEWebPAProbe) RunParodusClients(parent context.Context, invocation Invocation) EndpointResponse {
	probe.defaults()
	now := probe.Now()
	response := EndpointResponse{
		SchemaVersion: SchemaVersion,
		Journey:       JourneyParodusClients,
		ObservedAt:    now,
	}
	if err := invocation.ValidateFor(JourneyParodusClients); err != nil {
		response.Observations = []Observation{{EdgeID: "parodus-client-list", State: StateUnknown, ReasonID: "parodus-client-list-invalid", RemediationID: "check-diagnostic-request", Message: "Parodus client-list request is invalid", ObservedAt: now}}
		return response
	}
	if state := probe.ServiceState(parent); state != "active" {
		response.Observations = []Observation{{EdgeID: "parodus-client-list", State: StateUnknown, ReasonID: "parodus-inactive", RemediationID: "start-parodus", Message: "Parodus is not active", Evidence: []Evidence{{Key: "service-state", Value: state}}, ObservedAt: now}}
		return response
	}
	clients, truncated, observation := probe.observeParodusClients(parent, now)
	response.Observations = []Observation{observation}
	if observation.State == StatePassed {
		response.ParodusClients = &clients
		response.ParodusClientsTruncated = &truncated
	}
	return response
}

// RunTalariaDevices performs one passive, source-local Talaria device-list
// request. The caller cannot influence the Talaria endpoint or credentials.
func (probe CPEWebPAProbe) RunTalariaDevices(ctx context.Context, invocation Invocation) EndpointResponse {
	probe.defaults()
	now := probe.Now()
	response := EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyTalariaDevices, ObservedAt: now}
	if err := invocation.ValidateFor(JourneyTalariaDevices); err != nil {
		response.Observations = []Observation{
			{EdgeID: "talaria-reachability", State: StateUnknown, ReasonID: "talaria-inventory-invalid", RemediationID: "check-diagnostic-request", Message: "Talaria device inventory invocation is invalid", ObservedAt: now},
			{EdgeID: "talaria-device-inventory", State: StateSkipped, ReasonID: ReasonPrerequisiteFailed, ObservedAt: now},
		}
		return response
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.TalariaURL, nil)
	if err != nil {
		response.Observations = []Observation{
			failedObservation("talaria-reachability", "talaria-url-invalid", "check-talaria-configuration", "configured Talaria URL is invalid", now),
			{EdgeID: "talaria-device-inventory", State: StateSkipped, ReasonID: ReasonPrerequisiteFailed, ObservedAt: now},
		}
		return response
	}
	request.SetBasicAuth(probe.Username, probe.Password)
	client := *probe.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpResponse, err := client.Do(request)
	if err != nil {
		response.Observations = []Observation{
			failedObservation("talaria-reachability", "talaria-request-failed", "check-talaria-http", "Talaria device inventory request failed", now),
			{EdgeID: "talaria-device-inventory", State: StateSkipped, ReasonID: ReasonPrerequisiteFailed, ObservedAt: now},
		}
		return response
	}
	defer httpResponse.Body.Close()
	reachability := Observation{EdgeID: "talaria-reachability", State: StatePassed, Evidence: []Evidence{{Key: "http-status", Value: fmt.Sprint(httpResponse.StatusCode)}}, ObservedAt: now}
	if httpResponse.StatusCode == http.StatusUnauthorized || httpResponse.StatusCode == http.StatusForbidden {
		response.Observations = []Observation{
			reachability,
			{EdgeID: "talaria-device-inventory", State: StateFailed, ReasonID: "talaria-authentication-failed", RemediationID: "check-talaria-credentials", Message: "Talaria rejected authentication", ObservedAt: now},
		}
		return response
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		response.Observations = []Observation{
			reachability,
			{EdgeID: "talaria-device-inventory", State: StateUnknown, ReasonID: "talaria-inventory-unavailable", RemediationID: "check-talaria-service", Message: "Talaria returned an unexpected HTTP status", ObservedAt: now},
		}
		return response
	}
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, MaxDiagnosticBodyBytes+1))
	if err == nil && len(body) > MaxDiagnosticBodyBytes {
		err = fmt.Errorf("%w: response exceeds %d bytes", ErrTalariaInventoryInvalid, MaxDiagnosticBodyBytes)
	}
	devices, decodeErr := decodeTalariaDevices(body)
	if err == nil {
		err = decodeErr
	}
	if err != nil {
		reason := "talaria-inventory-invalid"
		if errors.Is(err, ErrTalariaInventoryLimit) {
			reason = "talaria-inventory-limit"
		}
		response.Observations = []Observation{
			reachability,
			{EdgeID: "talaria-device-inventory", State: StateUnknown, ReasonID: reason, RemediationID: "check-talaria-registry", Message: "Talaria device inventory was unavailable", ObservedAt: now},
		}
		return response
	}
	response.Observations = []Observation{
		reachability,
		{EdgeID: "talaria-device-inventory", State: StatePassed, ObservedAt: now},
	}
	response.TalariaDevices = &devices
	return response
}

func decodeTalariaDevices(body []byte) ([]TalariaDevice, error) {
	var registry struct {
		Devices *json.RawMessage `json:"devices"`
	}
	if err := json.Unmarshal(body, &registry); err != nil || registry.Devices == nil {
		return nil, ErrTalariaInventoryInvalid
	}
	type talariaStatistics struct {
		BytesSent        int64     `json:"bytesSent"`
		MessagesSent     int64     `json:"messagesSent"`
		BytesReceived    int64     `json:"bytesReceived"`
		MessagesReceived int64     `json:"messagesReceived"`
		Duplications     int64     `json:"duplications"`
		ConnectedAt      time.Time `json:"connectedAt"`
		Uptime           string    `json:"upTime"`
	}
	type talariaDevice struct {
		ID         string             `json:"id"`
		Pending    int64              `json:"pending"`
		Statistics *talariaStatistics `json:"statistics"`
	}
	var responseDevices []talariaDevice
	if err := json.Unmarshal(*registry.Devices, &responseDevices); err != nil {
		return nil, ErrTalariaInventoryInvalid
	}
	if responseDevices == nil {
		return nil, ErrTalariaInventoryInvalid
	}
	if len(responseDevices) > MaxTalariaDevices {
		return nil, ErrTalariaInventoryLimit
	}
	devices := make([]TalariaDevice, 0, len(responseDevices))
	for _, responseDevice := range responseDevices {
		if responseDevice.Statistics == nil {
			return nil, ErrTalariaInventoryInvalid
		}
		devices = append(devices, TalariaDevice{
			ID:               responseDevice.ID,
			Pending:          responseDevice.Pending,
			BytesSent:        responseDevice.Statistics.BytesSent,
			MessagesSent:     responseDevice.Statistics.MessagesSent,
			BytesReceived:    responseDevice.Statistics.BytesReceived,
			MessagesReceived: responseDevice.Statistics.MessagesReceived,
			Duplications:     responseDevice.Statistics.Duplications,
			ConnectedAt:      responseDevice.Statistics.ConnectedAt,
			Uptime:           responseDevice.Statistics.Uptime,
		})
	}
	sort.Slice(devices, func(left, right int) bool {
		return devices[left].ID < devices[right].ID
	})
	if err := validateTalariaDeviceList(JourneyTalariaDevices, &devices); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTalariaInventoryInvalid, err)
	}
	return devices, nil
}

func (probe *CPEWebPAProbe) defaults() {
	if probe.DeviceID == "" {
		probe.DeviceID = sourceDeviceID()
	}
	if probe.Timeout <= 0 {
		probe.Timeout = 2 * time.Second
	}
	if probe.Now == nil {
		probe.Now = func() time.Time { return time.Now().UTC() }
	}
	if probe.ServiceState == nil {
		probe.ServiceState = func(context.Context) string { return "unknown" }
	}
	if probe.LookupHost == nil {
		probe.LookupHost = net.DefaultResolver.LookupHost
	}
	if probe.DialContext == nil {
		dialer := &net.Dialer{Timeout: probe.Timeout}
		probe.DialContext = dialer.DialContext
	}
	if probe.HTTPClient == nil {
		probe.HTTPClient = &http.Client{Timeout: probe.Timeout}
	}
}

func sourceDeviceID() string {
	deviceID := os.Getenv("VCPE_HEALTH_SERIAL")
	if deviceID == "" {
		if address, err := os.ReadFile("/sys/class/net/erouter0/address"); err == nil {
			deviceID = strings.ReplaceAll(strings.TrimSpace(string(address)), ":", "")
		}
	}
	if deviceID != "" && !strings.HasPrefix(deviceID, "mac:") {
		return "mac:" + deviceID
	}
	return deviceID
}

func (probe CPEWebPAProbe) finish(edges []Edge, observations []Observation, observedAt time.Time) EndpointResponse {
	normalized, _, err := ApplyCausality(edges, observations, observedAt)
	if err != nil {
		normalized = []Observation{{EdgeID: "application-parodus", State: StateUnknown, ReasonID: "diagnostic-internal-error", Message: "diagnostic evaluation failed", ObservedAt: observedAt}}
	}
	return EndpointResponse{SchemaVersion: SchemaVersion, Journey: JourneyCPEWebPA, Observations: normalized, ObservedAt: observedAt}
}

func failedObservation(edge, reason, remediation, message string, observedAt time.Time) Observation {
	return Observation{EdgeID: edge, State: StateFailed, ReasonID: reason, RemediationID: remediation, Message: message, ObservedAt: observedAt}
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
