// Package handler provides the HTTP webhook handler for the event-sink service.
//
// It validates the X-Webpa-Signature HMAC-SHA1 header on every incoming POST
// before processing the payload, and logs each valid event as structured JSON.
// Caduceus signs webhook bodies with SHA1 HMAC (X-Webpa-Signature: sha1=<hex>).
package handler

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

// Handler handles webhook POSTs from Caduceus.
type Handler struct {
	secret []byte
	logger *slog.Logger
}

// New creates a Handler. secret is the WEBHOOK_SECRET value used to validate
// the X-Webpa-Signature header.
func New(secret string, logger *slog.Logger) *Handler {
	return &Handler{
		secret: []byte(secret),
		logger: logger,
	}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		h.logger.Error("failed to read request body", "error", err)
		http.Error(w, "read error", http.StatusInternalServerError)
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

	// Caduceus delivers the WRP payload as the HTTP body and the WRP metadata
	// (source, destination, content type, etc.) as X-Xmidt-*/X-Webpa-* headers.
	// Reconstruct the message from headers first; if that yields no source/dest
	// (some Caduceus configs send the whole WRP as a msgpack body), fall back to
	// decoding the body as a WRP message.
	var msg wrp.Message
	_ = wrphttp.SetMessageFromHeaders(r.Header, &msg)
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
