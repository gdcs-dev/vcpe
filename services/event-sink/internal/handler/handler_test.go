package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/event-sink/internal/diagnosticstate"
	"github.com/xmidt-org/wrp-go/v3"
)

func makeSignature(secret string, body []byte) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	return "sha1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandler_ValidEvent(t *testing.T) {
	h := New("test-secret", slog.Default())

	msg := wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      "mac:aabbccddeeff/apparmor-simulator",
		Destination: "event:apparmor/denied/mac:aabbccddeeff",
		ContentType: "application/json",
		Payload:     []byte(`{"apparmor":"DENIED","simulated":true}`),
	}
	var buf bytes.Buffer
	if err := wrp.NewEncoder(&buf, wrp.Msgpack).Encode(&msg); err != nil {
		t.Fatalf("encode WRP: %v", err)
	}
	body := buf.Bytes()
	sig := makeSignature("test-secret", body)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", sig)
	req.Header.Set("Content-Type", "application/msgpack")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandler_MissingSignature(t *testing.T) {
	h := New("test-secret", slog.Default())

	body := []byte(`{"source":"mac:aabbccddeeff/apparmor-simulator","dest":"event:apparmor/denied/mac:aabbccddeeff"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	// No X-Webpa-Signature header
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandler_WrongSecret(t *testing.T) {
	h := New("correct-secret", slog.Default())

	body := []byte(`{"source":"mac:aabbccddeeff/apparmor-simulator","dest":"event:test"}`)
	sig := makeSignature("wrong-secret", body)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", sig)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandler_IsolatesSignedDiagnosticCallbacks(t *testing.T) {
	state := diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{})
	h := NewWithState("test-secret", slog.Default(), state)
	correlationID := strings.Repeat("a", 64)
	body := []byte(`{"vcpe_diagnostic":"webhook-registration-callback-diagnostics","correlation_id":"` + correlationID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", makeSignature("test-secret", body))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	receipt, ok := state.Receipt(correlationID)
	if !ok || receipt.Source != diagnosticstate.SourceDirect || receipt.HTTPStatus != http.StatusNoContent {
		t.Fatalf("receipt = %+v, found = %t", receipt, ok)
	}
}

func TestHandler_DiagnosticCaduceusReceipt(t *testing.T) {
	state := diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{})
	h := NewWithState("test-secret", slog.Default(), state)
	correlationID := strings.Repeat("b", 64)
	body := []byte(`{"vcpe_diagnostic":"cpe-webpa-callback","correlation_id":"` + correlationID + `"}`)
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-Webpa-Signature", makeSignature("test-secret", body))
		req.Header.Set("X-Xmidt-Message-Type", "4")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
		}
	}

	receipt, ok := state.Receipt(correlationID)
	if !ok || receipt.Source != diagnosticstate.SourceCaduceus {
		t.Fatalf("receipt = %+v, found = %t", receipt, ok)
	}
}

func TestHandler_DiagnosticCallbacksAreNotNormalEvents(t *testing.T) {
	state := diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{})
	var logs bytes.Buffer
	h := NewWithState("test-secret", slog.New(slog.NewJSONHandler(&logs, nil)), state)
	correlationID := strings.Repeat("c", 64)
	diagnosticBody := []byte(`{"vcpe_diagnostic":"webhook-registration-callback-diagnostics","correlation_id":"` + correlationID + `"}`)
	diagnosticRequest := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(diagnosticBody))
	diagnosticRequest.Header.Set("X-Webpa-Signature", makeSignature("test-secret", diagnosticBody))
	diagnosticRequest.Header.Set("X-Xmidt-Message-Type", "4")
	diagnosticResponse := httptest.NewRecorder()
	h.ServeHTTP(diagnosticResponse, diagnosticRequest)
	if diagnosticResponse.Code != http.StatusNoContent {
		t.Fatalf("diagnostic status = %d, want %d", diagnosticResponse.Code, http.StatusNoContent)
	}

	normalBody := []byte(`{"apparmor":"DENIED"}`)
	normalRequest := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(normalBody))
	normalRequest.Header.Set("X-Webpa-Signature", makeSignature("test-secret", normalBody))
	normalRequest.Header.Set("X-Xmidt-Message-Type", "4")
	normalRequest.Header.Set("X-Xmidt-Source", "mac:aabbccddeeff/apparmor-simulator")
	normalRequest.Header.Set("X-Webpa-Device-Name", "event:apparmor/denied/mac:aabbccddeeff")
	normalResponse := httptest.NewRecorder()
	h.ServeHTTP(normalResponse, normalRequest)
	if normalResponse.Code != http.StatusOK {
		t.Fatalf("normal event status = %d, want %d", normalResponse.Code, http.StatusOK)
	}

	if _, ok := state.Receipt(correlationID); !ok {
		t.Fatal("diagnostic callback did not record its receipt")
	}
	output := logs.String()
	if strings.Contains(output, "isolated-diagnostic") || strings.Count(output, `"msg":"event received"`) != 1 {
		t.Fatalf("diagnostic callback was logged as a normal event: %s", output)
	}
	if !strings.Contains(output, `"dest":"event:apparmor/denied/mac:aabbccddeeff"`) || !strings.Contains(output, `"source":"mac:aabbccddeeff/apparmor-simulator"`) {
		t.Fatalf("ordinary signed event log changed: %s", output)
	}
}

func TestHandler_InvalidDiagnosticSignatureDoesNotRecordReceipt(t *testing.T) {
	state := diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{})
	h := NewWithState("test-secret", slog.Default(), state)
	correlationID := strings.Repeat("d", 64)
	body := []byte(`{"vcpe_diagnostic":"cpe-webpa-callback","correlation_id":"` + correlationID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", makeSignature("wrong-secret", body))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if _, ok := state.Receipt(correlationID); ok {
		t.Fatal("invalid callback recorded a receipt")
	}
}

func TestHandler_RejectsDirectCPECallbackMarker(t *testing.T) {
	state := diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{})
	h := NewWithState("test-secret", slog.Default(), state)
	correlationID := strings.Repeat("e", 64)
	body := []byte(`{"vcpe_diagnostic":"cpe-webpa-callback","correlation_id":"` + correlationID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", makeSignature("test-secret", body))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if _, ok := state.Receipt(correlationID); ok {
		t.Fatal("direct callback marker recorded a receipt")
	}
}

func TestHandler_RejectsMalformedCPECallbackMarker(t *testing.T) {
	state := diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{})
	h := NewWithState("test-secret", slog.Default(), state)
	correlationID := strings.Repeat("f", 63)
	body := []byte(`{"vcpe_diagnostic":"cpe-webpa-callback","correlation_id":"` + correlationID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", makeSignature("test-secret", body))
	req.Header.Set("X-Xmidt-Message-Type", "4")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if _, ok := state.Receipt(correlationID); ok {
		t.Fatal("malformed callback marker recorded a receipt")
	}
}

func TestHandler_RejectsOversizedCallbackWithoutLoggingPayload(t *testing.T) {
	var logs bytes.Buffer
	h := New("test-secret", slog.New(slog.NewJSONHandler(&logs, nil)))
	body := []byte(strings.Repeat("x", maxWebhookBodyBytes+1))
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", makeSignature("test-secret", body))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
	if logs.Len() != 0 {
		t.Fatalf("oversized payload was logged: %s", logs.String())
	}
}

func TestHandler_HeaderBasedEvent(t *testing.T) {
	h := New("test-secret", slog.Default())

	// Caduceus delivers payload as body + WRP metadata as headers.
	body := []byte(`{"apparmor":"DENIED","simulated":true}`)
	sig := makeSignature("test-secret", body)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webpa-Signature", sig)
	req.Header.Set("X-Xmidt-Message-Type", "4") // SimpleEvent
	req.Header.Set("X-Xmidt-Source", "mac:aabbccddeeff/apparmor-simulator")
	req.Header.Set("X-Webpa-Device-Name", "event:apparmor/denied/mac:aabbccddeeff")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	h := New("secret", slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestValidateHMAC_ValidPrefix(t *testing.T) {
	h := New("secret", slog.Default())
	body := []byte("hello")
	sig := makeSignature("secret", body)
	if !h.validateHMAC(body, sig) {
		t.Error("expected valid HMAC to pass")
	}
}

func TestValidateHMAC_InvalidPrefix(t *testing.T) {
	h := New("secret", slog.Default())
	body := []byte("hello")
	// Wrong prefix
	sig := fmt.Sprintf("md5=%s", hex.EncodeToString(body))
	if h.validateHMAC(body, sig) {
		t.Error("expected invalid prefix to fail")
	}
}
