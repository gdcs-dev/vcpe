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
	"testing"

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
