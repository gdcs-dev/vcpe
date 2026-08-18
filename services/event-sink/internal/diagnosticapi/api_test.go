package diagnosticapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gdcs-dev/vcpe/event-sink/internal/diagnosticstate"
)

func TestAPIExposesBoundedSafeIntentAndReceipts(t *testing.T) {
	now := time.Date(2026, time.August, 14, 19, 0, 0, 0, time.UTC)
	correlationID := strings.Repeat("a", 64)
	state := diagnosticstate.New(diagnosticstate.Intent{CallbackURL: "http://user:secret@example/webhook?token=hidden", EventFilter: "apparmor/.*", DeviceMatcher: ".*", ContentType: "application/json", SecretConfigured: true}, diagnosticstate.Config{Now: func() time.Time { return now }})
	state.RecordInitialSuccess(now)
	state.RecordReceipt(diagnosticstate.Receipt{CorrelationID: correlationID, Source: diagnosticstate.SourceDirect, HTTPStatus: http.StatusNoContent})
	api := New(state)

	intent := httptest.NewRecorder()
	api.ServeHTTP(intent, httptest.NewRequest(http.MethodGet, intentPath, nil))
	if intent.Code != http.StatusOK || strings.Contains(intent.Body.String(), "user:secret") || strings.Contains(intent.Body.String(), "token=hidden") {
		t.Fatalf("intent response = %d %s", intent.Code, intent.Body.String())
	}
	var decoded intentResponse
	if err := json.Unmarshal(intent.Body.Bytes(), &decoded); err != nil || decoded.CallbackURL != "http://example/webhook" || !decoded.SecretConfigured || decoded.ObservedAt == "" || decoded.InitialSuccessAt == "" {
		t.Fatalf("intent = %+v, error = %v", decoded, err)
	}

	receipt := httptest.NewRecorder()
	api.ServeHTTP(receipt, httptest.NewRequest(http.MethodGet, receiptsPath+correlationID, nil))
	if receipt.Code != http.StatusOK || strings.Contains(receipt.Body.String(), "secret") {
		t.Fatalf("receipt response = %d %s", receipt.Code, receipt.Body.String())
	}
}

func TestAPIMethodAndReceiptValidation(t *testing.T) {
	api := New(diagnosticstate.New(diagnosticstate.Intent{}, diagnosticstate.Config{}))
	for _, path := range []string{capabilitiesPath, intentPath, receiptsPath + "missing"} {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status = %d, want %d", path, response.Code, http.StatusMethodNotAllowed)
		}
	}
	missing := httptest.NewRecorder()
	api.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, receiptsPath+"missing", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing receipt status = %d, want %d", missing.Code, http.StatusNotFound)
	}
	malformed := httptest.NewRecorder()
	api.ServeHTTP(malformed, httptest.NewRequest(http.MethodGet, receiptsPath+strings.Repeat("a", 63), nil))
	if malformed.Code != http.StatusNotFound {
		t.Fatalf("malformed receipt status = %d, want %d", malformed.Code, http.StatusNotFound)
	}
	nested := httptest.NewRecorder()
	api.ServeHTTP(nested, httptest.NewRequest(http.MethodGet, receiptsPath+strings.Repeat("a", 64)+"/extra", nil))
	if nested.Code != http.StatusNotFound {
		t.Fatalf("nested receipt status = %d, want %d", nested.Code, http.StatusNotFound)
	}
}
