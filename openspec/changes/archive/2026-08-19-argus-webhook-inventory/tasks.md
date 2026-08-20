## 1. Diagnostic Model And Safe Argus Inventory

- [x] 1.1 Add the `argus-webhooks` journey, its 64-registration limit, safe structured `webhookRegistrations` result and endpoint-response fields, journey-specific invocation validation, and strict nested result validation in `controlplane/internal/diagnostic/model.go`.
- [x] 1.2 Extend central diagnostic safety copying and ASCII/JSON rendering in `controlplane/internal/diagnostic/safety.go` and `render.go` so registrations appear only for the inventory journey, remain fingerprint-sorted, and render an explicit empty state.
- [x] 1.3 Add an inventory-specific `WebhookProbe` operation in `controlplane/internal/diagnostic/webhook.go` that uses compatible ancla/chrysom decoding, includes registrations from every Argus creator, normalizes safe callback identities, sorts fields and entries deterministically, and rejects unsafe or over-limit buckets without serializing raw item data.
- [x] 1.4 Add model, safety, renderer, and webhook-probe tests covering valid multi-creator inventory, empty inventory, deterministic ordering, normalization/redaction, malformed records, and the 64-registration limit.

## 2. WebPA-Local Journey Wiring

- [x] 2.1 Add a WebPA-only `argus-webhooks` provider and expected two-stage WebPA-to-Argus graph in `controlplane/internal/diagnostic/provider.go`, then update resolver selection so no subscriber or target health endpoint is required.
- [x] 2.2 Add client and orchestrator support for invoking the WebPA-local inventory endpoint and copying validated structured registrations to the final result while preserving existing webhook and Parodus journeys.
- [x] 2.3 Register and dispatch the inventory capability in `controlplane/cmd/vcpe-healthd` and the WebPA service diagnostic configuration so the persisted WebPA loopback endpoint advertises and serves `argus-webhooks` without callback, Caduceus, or Argus mutation traffic.
- [x] 2.4 Add provider, resolver, client, orchestrator, and health-daemon tests for capability discovery, one-WebPA selection, persisted-loopback-only collection, successful empty and multi-registration results, and bounded Argus transport/decode/limit failures.

## 3. CLI Contract And Help

- [x] 3.1 Extend `controlplane/internal/app/cli.go` to accept `--to webhooks`, require only the common deployment/source selectors, and reject client-service, subscriber, subscriber-replica, active-callback, active-event, event, and device-id options before state access.
- [x] 3.2 Update `controlplane/internal/app/help.go` and the diagnose help golden file with the `webhooks` target description and WebPA-source example, preserving the existing focused `webhook` documentation.
- [x] 3.3 Add CLI parse and `executeLocal` tests covering a valid WebPA inventory request, every cross-journey rejection, persisted loopback invocation, and both human and JSON structured output.

## 4. Verification

- [x] 4.1 Run focused tests for `./internal/diagnostic`, `./cmd/vcpe-healthd`, and `./internal/app` from `controlplane/`.
- [x] 4.2 Run `go test ./...` and `openspec validate argus-webhook-inventory --strict`; record any known environment-dependent failures separately from this change.