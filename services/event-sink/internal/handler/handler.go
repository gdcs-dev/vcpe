// Package handler provides the HTTP webhook handler for the event-sink service.
//
// It validates the X-Webpa-Signature HMAC-SHA1 header on every incoming POST
// before processing the payload, and logs each valid event as structured JSON.
// Caduceus signs webhook bodies with SHA1 HMAC (X-Webpa-Signature: sha1=<hex>).
package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gdcs-dev/vcpe/event-sink/internal/diagnosticstate"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

const maxWebhookBodyBytes = 1 << 20

// Handler handles webhook POSTs from Caduceus.
type Handler struct {
	secret []byte
	logger *slog.Logger
	state  *diagnosticstate.Store
}

// New creates a Handler. secret is the WEBHOOK_SECRET value used to validate
// the X-Webpa-Signature header.
func New(secret string, logger *slog.Logger) *Handler {
	return NewWithState(secret, logger, nil)
}

// NewWithState creates a Handler that records correctly signed diagnostic
// callback receipts without retaining their body or signature.
func NewWithState(secret string, logger *slog.Logger, state *diagnosticstate.Store) *Handler {
	return &Handler{
		secret: []byte(secret),
		logger: logger,
		state:  state,
	}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes+1))
	if err != nil {
		h.logger.Error("failed to read request body", "error", err)
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	if len(body) > maxWebhookBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Validate HMAC signature
	sig := r.Header.Get("X-Webpa-Signature")
	if !h.validateHMAC(body, sig) {
		h.logger.Warn("invalid or missing HMAC signature",
			"remote_addr", r.RemoteAddr,
			"sig_present", sig != "")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if receipt, ok := diagnosticReceipt(body, r.Header); ok {
		if h.state != nil {
			h.state.RecordReceipt(receipt)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Caduceus delivers the WRP payload as the HTTP body and its metadata as
	// headers. Its X-Webpa-Event value is the WRP destination without `event:`;
	// X-Webpa-Device-Name is only the device identity.
	var msg wrp.Message
	_ = wrphttp.SetMessageFromHeaders(r.Header, &msg)
	if event := r.Header.Get("X-Webpa-Event"); event != "" {
		msg.Destination = "event:" + strings.TrimPrefix(event, "event:")
	}
	msg.Payload = body

	if msg.Source == "" && msg.Destination == "" {
		var bodyMsg wrp.Message
		format := wrp.Msgpack
		if strings.Contains(r.Header.Get("Content-Type"), "json") {
			format = wrp.JSON
		}
		if err := wrp.NewDecoderBytes(body, format).Decode(&bodyMsg); err == nil && (bodyMsg.Source != "" || bodyMsg.Destination != "") {
			msg = bodyMsg
		}
	}

	// Extract device_id from WRP source (format: "mac:<hex>/service-name")
	deviceID := msg.Source
	if idx := strings.Index(msg.Source, "/"); idx > 0 {
		deviceID = msg.Source[:idx]
	}

	h.logger.Info("event received",
		"dest", msg.Destination,
		"source", msg.Source,
		"device_id", deviceID,
		"content_type", msg.ContentType,
		"payload_size", len(msg.Payload),
		"payload", string(msg.Payload),
	)

	w.WriteHeader(http.StatusOK)
}

// validateHMAC checks that sig matches "sha1=<hmac-hex>" over body using h.secret.
// Caduceus uses SHA1 HMAC. Uses constant-time comparison to prevent timing attacks.
func (h *Handler) validateHMAC(body []byte, sig string) bool {
	if !strings.HasPrefix(sig, "sha1=") {
		return false
	}
	mac := hmac.New(sha1.New, h.secret)
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig[len("sha1="):]))
}

type diagnosticMarker struct {
	Diagnostic    string `json:"vcpe_diagnostic"`
	CorrelationID string `json:"correlation_id"`
}

const (
	webhookDiagnosticMarker  = "webhook-registration-callback-diagnostics"
	callbackDiagnosticMarker = "cpe-webpa-callback"
)

func diagnosticReceipt(body []byte, header http.Header) (diagnosticstate.Receipt, bool) {
	var marker diagnosticMarker
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil || !diagnosticstate.ValidCorrelationID(marker.CorrelationID) {
		return diagnosticstate.Receipt{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return diagnosticstate.Receipt{}, false
	}
	fromCaduceus := header.Get("X-Xmidt-Message-Type") != "" || header.Get("X-Webpa-Event") != ""
	source := diagnosticstate.SourceDirect
	if fromCaduceus {
		source = diagnosticstate.SourceCaduceus
	}
	switch marker.Diagnostic {
	case webhookDiagnosticMarker:
	case callbackDiagnosticMarker:
		if !fromCaduceus {
			return diagnosticstate.Receipt{}, false
		}
	default:
		return diagnosticstate.Receipt{}, false
	}
	return diagnosticstate.Receipt{CorrelationID: marker.CorrelationID, Source: source, HTTPStatus: http.StatusNoContent}, true
}
