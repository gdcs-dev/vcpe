// event-sink receives WRP events from Caduceus via a registered Argus-backed
// webhook and logs them as structured JSON.
//
// Environment variables:
//
//	ARGUS_URL              Argus base URL (default: http://webpa:6600)
//	ARGUS_BASIC_AUTH       Base64-encoded "user:pass" for Argus (required)
//	WEBHOOK_URL            This service's webhook URL (default: http://event-sink:8080/webhook)
//	WEBHOOK_EVENTS_REGEX   Caduceus events filter regex (default: apparmor/.*)
//	WEBHOOK_DEVICE_MATCHER Caduceus device_id filter regex (default: .*)
//	WEBHOOK_SECRET         HMAC secret shared with Caduceus (required)
//	LISTEN_ADDR            HTTP listen address (default: :8080)
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/gdcs-dev/vcpe/event-sink/internal/handler"
	"github.com/gdcs-dev/vcpe/event-sink/internal/registration"
)

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustCompileRegex(key, value string) {
	if _, err := regexp.Compile(value); err != nil {
		slog.Error("invalid regex env var", "key", key, "value", value, "error", err)
		os.Exit(1)
	}
}

func main() {
	// Structured JSON logging to stdout
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	argusURL := getEnvDefault("ARGUS_URL", "http://webpa:6600")
	argusAuth := os.Getenv("ARGUS_BASIC_AUTH")
	webhookURL := getEnvDefault("WEBHOOK_URL", "http://event-sink:8080/webhook")
	eventsRegex := getEnvDefault("WEBHOOK_EVENTS_REGEX", "apparmor/.*")
	deviceMatcher := getEnvDefault("WEBHOOK_DEVICE_MATCHER", ".*")
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	listenAddr := getEnvDefault("LISTEN_ADDR", ":8080")

	// Fail fast on invalid regexes
	mustCompileRegex("WEBHOOK_EVENTS_REGEX", eventsRegex)
	mustCompileRegex("WEBHOOK_DEVICE_MATCHER", deviceMatcher)

	slog.Info("event-sink starting",
		"argus_url", argusURL,
		"webhook_url", webhookURL,
		"events_regex", eventsRegex,
		"device_matcher", deviceMatcher,
		"listen_addr", listenAddr,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Set up HTTP server (start before registration so /health responds immediately)
	wh := handler.New(webhookSecret, slog.Default())
	mux := http.NewServeMux()
	mux.Handle("/webhook", wh)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			stop()
		}
	}()

	slog.Info("HTTP server listening", "addr", listenAddr)

	// Register webhook with Argus (blocks until first successful PUT)
	regCfg := registration.Config{
		ArgusURL:       argusURL,
		ArgusBasicAuth: argusAuth,
		WebhookURL:     webhookURL,
		EventsRegex:    eventsRegex,
		DeviceMatcher:  deviceMatcher,
		WebhookSecret:  webhookSecret,
	}

	if err := registration.Register(ctx, regCfg); err != nil {
		slog.Error("webhook registration failed", "error", err)
		os.Exit(1)
	}

	// Start TTL refresh goroutine
	go registration.RefreshLoop(ctx, regCfg)

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
}
