// Package diagnosticapi exposes bounded, non-sensitive event-sink diagnostic
// evidence over the service's existing loopback HTTP listener.
package diagnosticapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/event-sink/internal/diagnosticstate"
)

const (
	capabilitiesPath = "/diagnostics"
	intentPath       = "/diagnostics/webhook-subscriber/intent"
	receiptsPath     = "/diagnostics/webhook-subscriber/receipts/"
)

// API serves the subscriber-owned portion of webhook diagnostics.
type API struct{ state *diagnosticstate.Store }

// New creates a bounded diagnostic API over the supplied in-memory state.
func New(state *diagnosticstate.Store) *API { return &API{state: state} }

// ServeHTTP implements the diagnostic endpoint routes.
func (api *API) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.URL.Path == capabilitiesPath:
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		writeJSON(writer, capabilitiesResponse{SchemaVersion: "vcpe.dev/diagnostics/v1", Journeys: []string{"webhook-subscriber"}})
	case request.URL.Path == intentPath:
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		api.writeIntent(writer)
	case strings.HasPrefix(request.URL.Path, receiptsPath):
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		api.writeReceipt(writer, strings.TrimPrefix(request.URL.Path, receiptsPath))
	default:
		http.NotFound(writer, request)
	}
}

type capabilitiesResponse struct {
	SchemaVersion string   `json:"schemaVersion"`
	Journeys      []string `json:"journeys"`
}

type intentResponse struct {
	SchemaVersion     string `json:"schemaVersion"`
	Journey           string `json:"journey"`
	ObservedAt        string `json:"observedAt"`
	CallbackURL       string `json:"callbackUrl"`
	EventFilter       string `json:"eventFilter"`
	DeviceMatcher     string `json:"deviceMatcher"`
	ContentType       string `json:"contentType"`
	SecretConfigured  bool   `json:"secretConfigured"`
	InitialSuccessAt  string `json:"initialSuccessAt,omitempty"`
	RefreshSuccessAt  string `json:"refreshSuccessAt,omitempty"`
	RefreshFailureAt  string `json:"refreshFailureAt,omitempty"`
	LastFailureAt     string `json:"lastFailureAt,omitempty"`
	LastErrorCategory string `json:"lastErrorCategory,omitempty"`
}

type receiptResponse struct {
	SchemaVersion string `json:"schemaVersion"`
	CorrelationID string `json:"correlationId"`
	Source        string `json:"source"`
	AcceptedAt    string `json:"acceptedAt"`
	HTTPStatus    int    `json:"httpStatus"`
}

func (api *API) writeIntent(writer http.ResponseWriter) {
	snapshot := api.state.Snapshot()
	writeJSON(writer, intentResponse{
		SchemaVersion:     "vcpe.dev/diagnostic/v1",
		Journey:           "webhook-subscriber",
		ObservedAt:        formatTime(snapshot.ObservedAt),
		CallbackURL:       snapshot.Intent.CallbackURL,
		EventFilter:       snapshot.Intent.EventFilter,
		DeviceMatcher:     snapshot.Intent.DeviceMatcher,
		ContentType:       snapshot.Intent.ContentType,
		SecretConfigured:  snapshot.Intent.SecretConfigured,
		InitialSuccessAt:  formatTime(snapshot.InitialSuccessAt),
		RefreshSuccessAt:  formatTime(snapshot.RefreshSuccessAt),
		RefreshFailureAt:  formatTime(snapshot.RefreshFailureAt),
		LastFailureAt:     formatTime(snapshot.LastFailureAt),
		LastErrorCategory: snapshot.LastErrorCategory,
	})
}

func (api *API) writeReceipt(writer http.ResponseWriter, correlationID string) {
	if !diagnosticstate.ValidCorrelationID(correlationID) {
		http.NotFound(writer, nil)
		return
	}
	receipt, ok := api.state.Receipt(correlationID)
	if !ok {
		http.NotFound(writer, nil)
		return
	}
	writeJSON(writer, receiptResponse{
		SchemaVersion: "vcpe.dev/diagnostic/v1",
		CorrelationID: receipt.CorrelationID,
		Source:        receipt.Source,
		AcceptedAt:    formatTime(receipt.AcceptedAt),
		HTTPStatus:    receipt.HTTPStatus,
	})
}

func methodNotAllowed(writer http.ResponseWriter, method string) {
	writer.Header().Set("Allow", method)
	http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
