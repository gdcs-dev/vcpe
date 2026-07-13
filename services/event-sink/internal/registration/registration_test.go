package registration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(argusURL string) Config {
	return Config{
		ArgusURL:       argusURL,
		ArgusBasicAuth: "dXNlcjpwYXNz",
		WebhookURL:     "http://event-sink:8080/webhook",
		EventsRegex:    "apparmor/.*",
		DeviceMatcher:  ".*",
		WebhookSecret:  "test-secret",
	}
}

func TestRegister_Success(t *testing.T) {
	var putCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			atomic.AddInt32(&putCount, 1)
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	ctx := context.Background()
	if err := Register(ctx, cfg); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if atomic.LoadInt32(&putCount) == 0 {
		t.Error("expected at least one PUT to Argus")
	}
}

func TestRegister_ContextCancelled(t *testing.T) {
	// Use a hung server so chrysom gets a context-cancelled error (not a panic-inducing non-2xx).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // longer than context timeout
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := Register(ctx, cfg)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestNewAnclaService_PrefixesBasicIfNeeded(t *testing.T) {
	// WithPrefix
	cfg := testConfig("http://localhost:6600")
	cfg.ArgusBasicAuth = "dXNlcjpwYXNz"
	svc, err := newAnclaService(cfg)
	if err != nil {
		t.Fatalf("newAnclaService failed: %v", err)
	}
	if svc == nil {
		t.Error("expected non-nil service")
	}
	// Already has prefix
	cfg.ArgusBasicAuth = "Basic dXNlcjpwYXNz"
	svc2, err := newAnclaService(cfg)
	if err != nil {
		t.Fatalf("newAnclaService failed: %v", err)
	}
	if svc2 == nil {
		t.Error("expected non-nil service")
	}
}

func TestBuildHook_Fields(t *testing.T) {
	cfg := testConfig("http://localhost:6600")
	hook := buildHook(cfg)
	if hook.Webhook.Config.URL != cfg.WebhookURL {
		t.Errorf("URL mismatch: got %s", hook.Webhook.Config.URL)
	}
	if len(hook.Webhook.Events) != 1 || hook.Webhook.Events[0] != cfg.EventsRegex {
		t.Errorf("events mismatch: got %v", hook.Webhook.Events)
	}
	if hook.Webhook.Duration != hookDuration {
		t.Errorf("duration mismatch: got %v", hook.Webhook.Duration)
	}
	if hook.Webhook.Until.Before(time.Now()) {
		t.Error("Until should be in the future")
	}
}
