// Package registration handles Argus webhook registration and TTL refresh.
//
// Uses ancla v0.3.12 (same version as the deployed Caduceus/tr1d1um) via
// ancla.NewService() + svc.Add() — the same pattern used by the working
// caduceus-webhook-register reference implementation.
package registration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/event-sink/internal/diagnosticstate"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/argus/chrysom"
	"go.uber.org/zap"
)

// argusLogger is handed to ancla/chrysom as the required getLogger callback.
// chrysom.BasicClient stores this with no nil check and calls it
// unconditionally whenever Argus responds with a non-2xx status, so passing
// nil (as ancla.NewService(cfg, nil) would) panics on the first non-success
// response instead of just logging it.
var argusLogger = newArgusLogger()

func newArgusLogger() *zap.Logger {
	if logger, err := zap.NewProduction(); err == nil {
		return logger
	}
	return zap.NewNop()
}

const (
	hookDuration       = 12 * time.Hour
	hookUntilOffset    = 30 * 24 * time.Hour // 30 days
	refreshInterval    = 6 * time.Hour
	maxRetryDelay      = 30 * time.Second
	argusWebhookBucket = "webhooks"
)

// Config holds all parameters needed to register the webhook with Argus.
type Config struct {
	ArgusURL       string // e.g. "http://webpa:6600"
	ArgusBasicAuth string // Full Authorization header value or just the base64 part
	WebhookURL     string // e.g. "http://event-sink:8080/webhook"
	EventsRegex    string // Caduceus events filter regex
	DeviceMatcher  string // Caduceus device_id filter regex
	WebhookSecret  string // HMAC secret shared with Caduceus
}

func newAnclaService(cfg Config) (ancla.Service, error) {
	auth := cfg.ArgusBasicAuth
	if !strings.HasPrefix(auth, "Basic ") {
		auth = "Basic " + auth
	}
	anclaConfig := ancla.Config{
		JWTParserType:     "simple",
		DisablePartnerIDs: true,
		BasicClientConfig: chrysom.BasicClientConfig{
			Address: cfg.ArgusURL,
			Bucket:  argusWebhookBucket,
			Auth: chrysom.Auth{
				Basic: auth,
			},
		},
	}
	return ancla.NewService(anclaConfig, func(context.Context) *zap.Logger { return argusLogger })
}

func buildHook(cfg Config) ancla.InternalWebhook {
	return ancla.InternalWebhook{
		Webhook: ancla.Webhook{
			Address: "",
			Config: ancla.DeliveryConfig{
				URL:             cfg.WebhookURL,
				ContentType:     "application/json",
				Secret:          cfg.WebhookSecret,
				AlternativeURLs: []string{},
			},
			Events: []string{cfg.EventsRegex},
			Matcher: ancla.MetadataMatcherConfig{
				DeviceID: []string{cfg.DeviceMatcher},
			},
			Duration: hookDuration,
			Until:    time.Now().Add(hookUntilOffset),
		},
	}
}

// Register registers the webhook via ancla. Blocks until the first successful
// registration, retrying with exponential backoff (1s → ... → 30s cap).
func Register(ctx context.Context, cfg Config) error {
	return RegisterWithState(ctx, cfg, nil)
}

// RegisterWithState registers the webhook and records bounded diagnostic state
// without changing the retry, health, or logging behavior of Register.
func RegisterWithState(ctx context.Context, cfg Config, state *diagnosticstate.Store) error {
	svc, err := newAnclaService(cfg)
	if err != nil {
		if state != nil {
			state.RecordInitialFailure(time.Now(), registrationErrorCategory(err))
		}
		return fmt.Errorf("ancla service init: %w", err)
	}

	hook := buildHook(cfg)
	delay := time.Second
	attempt := 0

	for {
		err := svc.Add(ctx, "", hook)
		if err == nil {
			if state != nil {
				state.RecordInitialSuccess(time.Now())
			}
			slog.Info("webhook registered",
				"events_regex", cfg.EventsRegex,
				"device_matcher", cfg.DeviceMatcher,
				"webhook_url", cfg.WebhookURL)
			return nil
		}

		attempt++
		if state != nil {
			state.RecordInitialFailure(time.Now(), registrationErrorCategory(err))
		}
		slog.Error("argus registration failed",
			"error", err,
			"attempt", attempt,
			"retry_in", delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		if delay < maxRetryDelay {
			delay *= 2
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
		}
	}
}

// RefreshLoop re-registers the webhook every 6h to keep the Argus item alive.
func RefreshLoop(ctx context.Context, cfg Config) {
	RefreshLoopWithState(ctx, cfg, nil)
}

// RefreshLoopWithState refreshes the webhook at the existing interval while
// recording only bounded success and failure diagnostic state.
func RefreshLoopWithState(ctx context.Context, cfg Config, state *diagnosticstate.Store) {
	svc, err := newAnclaService(cfg)
	if err != nil {
		if state != nil {
			state.RecordRefreshFailure(time.Now(), registrationErrorCategory(err))
		}
		slog.Error("ancla service init for refresh", "error", err)
		return
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hook := buildHook(cfg)
			if err := svc.Add(ctx, "", hook); err != nil {
				if state != nil {
					state.RecordRefreshFailure(time.Now(), registrationErrorCategory(err))
				}
				slog.Error("webhook TTL refresh failed", "error", err)
			} else {
				if state != nil {
					state.RecordRefreshSuccess(time.Now())
				}
				slog.Info("webhook TTL refreshed")
			}
		}
	}
}

func registrationErrorCategory(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "authentication"), strings.Contains(message, "status 401"), strings.Contains(message, "status 403"):
		return "argus-authentication-failed"
	case strings.Contains(message, "context canceled"), strings.Contains(message, "deadline exceeded"):
		return "argus-request-cancelled"
	default:
		return "argus-registration-failed"
	}
}
