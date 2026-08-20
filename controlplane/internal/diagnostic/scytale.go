package diagnostic

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xmidt-org/wrp-go/v3"
)

const wrpMsgpackContentType = "application/msgpack"

func (probe CPEWebPAProbe) observeParodusClient(ctx context.Context, observedAt time.Time) Observation {
	unknown := func(reason, remediation, message string) Observation {
		return Observation{
			EdgeID:        "application-parodus",
			State:         StateUnknown,
			ReasonID:      reason,
			RemediationID: remediation,
			Message:       message,
			Evidence:      []Evidence{{Key: "client-service", Value: probe.ParodusClient}},
			ObservedAt:    observedAt,
		}
	}
	if err := validateClientService(probe.ParodusClient); err != nil {
		return unknown("parodus-client-invalid", "configure-parodus-client", "configured Parodus client service name is invalid")
	}
	if probe.DeviceID == "" || probe.ScytaleURL == "" {
		return unknown("parodus-client-query-unconfigured", "configure-parodus-client-query", "Parodus client status query is not configured")
	}

	requestMessage := wrp.Message{
		Type:            wrp.RetrieveMessageType,
		Source:          "dns:webpa",
		Destination:     probe.DeviceID + "/parodus/service-status/" + probe.ParodusClient,
		TransactionUUID: newTransactionUUID(),
	}
	body, err := json.Marshal(requestMessage)
	if err != nil {
		return unknown("parodus-client-request-invalid", "check-diagnostic-protocol", "failed to encode Parodus client status request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, probe.ScytaleURL, bytes.NewReader(body))
	if err != nil {
		return unknown("scytale-url-invalid", "check-scytale-configuration", "configured Scytale URL is invalid")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(probe.Username, probe.Password)
	response, err := probe.HTTPClient.Do(request)
	if err != nil {
		return unknown("scytale-request-failed", "check-scytale-reachability", "Scytale client status request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return unknown("scytale-authentication-failed", "check-scytale-credentials", "Scytale rejected the client status request")
	}
	if response.StatusCode != http.StatusOK {
		return unknown("scytale-http-status", "check-scytale-service", fmt.Sprintf("Scytale returned HTTP %d", response.StatusCode))
	}
	if response.ContentLength > MaxDiagnosticBodyBytes {
		return unknown("scytale-response-oversized", "check-scytale-service", "Scytale client status response exceeded the size limit")
	}
	limited := &io.LimitedReader{R: response.Body, N: MaxDiagnosticBodyBytes + 1}
	encoded, err := io.ReadAll(limited)
	if err != nil || limited.N <= 0 {
		return unknown("scytale-response-invalid", "check-scytale-service", "Scytale client status response could not be read")
	}
	var responseMessage wrp.Message
	if err := wrp.NewDecoderBytes(encoded, wrp.Msgpack).Decode(&responseMessage); err != nil {
		return unknown("scytale-response-invalid", "check-scytale-service", "Scytale returned an invalid WRP response")
	}
	if responseMessage.TransactionUUID != requestMessage.TransactionUUID || responseMessage.Destination != requestMessage.Source {
		return unknown("scytale-response-mismatch", "check-scytale-service", "Scytale returned a mismatched WRP response")
	}
	var payload struct {
		Status string `json:"service-status"`
	}
	if err := json.Unmarshal(responseMessage.Payload, &payload); err != nil {
		return unknown("parodus-client-status-invalid", "check-parodus-service-status", "Parodus returned an invalid client status payload")
	}
	evidence := []Evidence{
		{Key: "client-service", Value: probe.ParodusClient},
		{Key: "client-evidence", Value: payload.Status},
	}
	switch strings.ToLower(payload.Status) {
	case "online":
		return Observation{EdgeID: "application-parodus", State: StatePassed, Message: "Parodus reports the application client online", Evidence: evidence, ObservedAt: observedAt}
	case "offline":
		return Observation{EdgeID: "application-parodus", State: StateFailed, ReasonID: "parodus-client-offline", RemediationID: "check-libparodus-registration", Message: "Parodus reports the application client offline", Evidence: evidence, ObservedAt: observedAt}
	default:
		return unknown("parodus-client-status-invalid", "check-parodus-service-status", "Parodus returned an unknown client status")
	}
}

func (probe CPEWebPAProbe) observeParodusClients(ctx context.Context, observedAt time.Time) ([]string, bool, Observation) {
	unknown := func(reason, remediation, message string) ([]string, bool, Observation) {
		return nil, false, Observation{
			EdgeID:        "parodus-client-list",
			State:         StateUnknown,
			ReasonID:      reason,
			RemediationID: remediation,
			Message:       message,
			ObservedAt:    observedAt,
		}
	}
	if probe.DeviceID == "" || probe.ScytaleURL == "" {
		return unknown("parodus-client-list-unconfigured", "configure-parodus-client-query", "Parodus client-list query is not configured")
	}

	requestMessage := wrp.Message{
		Type:            wrp.RetrieveMessageType,
		Source:          "dns:webpa",
		Destination:     probe.DeviceID + "/parodus/client-list",
		TransactionUUID: newTransactionUUID(),
	}
	body, err := json.Marshal(requestMessage)
	if err != nil {
		return unknown("parodus-client-list-request-invalid", "check-diagnostic-protocol", "failed to encode Parodus client-list request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, probe.ScytaleURL, bytes.NewReader(body))
	if err != nil {
		return unknown("scytale-url-invalid", "check-scytale-configuration", "configured Scytale URL is invalid")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(probe.Username, probe.Password)
	response, err := probe.HTTPClient.Do(request)
	if err != nil {
		return unknown("scytale-request-failed", "check-scytale-reachability", "Scytale client-list request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return unknown("scytale-authentication-failed", "check-scytale-credentials", "Scytale rejected the client-list request")
	}
	if response.StatusCode != http.StatusOK {
		return unknown("scytale-http-status", "check-scytale-service", fmt.Sprintf("Scytale returned HTTP %d", response.StatusCode))
	}
	if response.ContentLength > MaxDiagnosticBodyBytes {
		return unknown("scytale-response-oversized", "check-scytale-service", "Scytale client-list response exceeded the size limit")
	}
	limited := &io.LimitedReader{R: response.Body, N: MaxDiagnosticBodyBytes + 1}
	encoded, err := io.ReadAll(limited)
	if err != nil || limited.N <= 0 {
		return unknown("scytale-response-invalid", "check-scytale-service", "Scytale client-list response could not be read")
	}
	var responseMessage wrp.Message
	if err := wrp.NewDecoderBytes(encoded, wrp.Msgpack).Decode(&responseMessage); err != nil {
		return unknown("scytale-response-invalid", "check-scytale-service", "Scytale returned an invalid WRP response")
	}
	if responseMessage.TransactionUUID != requestMessage.TransactionUUID || responseMessage.Destination != requestMessage.Source {
		return unknown("scytale-response-mismatch", "check-scytale-service", "Scytale returned a mismatched WRP response")
	}
	var payload struct {
		Clients   []string `json:"client-list"`
		Truncated *bool    `json:"truncated"`
	}
	if err := json.Unmarshal(responseMessage.Payload, &payload); err != nil || payload.Clients == nil || payload.Truncated == nil {
		return unknown("parodus-client-list-invalid", "check-parodus-client-list", "Parodus returned an invalid client-list payload")
	}
	if err := validateParodusClientList(JourneyParodusClients, &payload.Clients, payload.Truncated); err != nil {
		return unknown("parodus-client-list-invalid", "check-parodus-client-list", "Parodus returned an invalid client-list payload")
	}
	return payload.Clients, *payload.Truncated, Observation{EdgeID: "parodus-client-list", State: StatePassed, Message: "Parodus returned registered application clients", ObservedAt: observedAt}
}

func newTransactionUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("diagnostic-%d", time.Now().UnixNano())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
