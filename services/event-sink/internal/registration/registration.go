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

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/argus/chrysom"
)

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
	return ancla.NewService(anclaConfig, nil)
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
	svc, err := newAnclaService(cfg)
	if err != nil {
		return fmt.Errorf("ancla service init: %w", err)
	}

	hook := buildHook(cfg)
	delay := time.Second
	attempt := 0

	for {
		err := svc.Add(ctx, "", hook)
		if err == nil {
			slog.Info("webhook registered",
				"events_regex", cfg.EventsRegex,
				"device_matcher", cfg.DeviceMatcher,
				"webhook_url", cfg.WebhookURL)
			return nil
		}

		attempt++
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
	svc, err := newAnclaService(cfg)
	if err != nil {
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
				slog.Error("webhook TTL refresh failed", "error", err)
			} else {
				slog.Info("webhook TTL refreshed")
			}
		}
	}
}
