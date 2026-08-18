## 1. External Contract Spikes

- [x] 1.1 Exercise the deployed ancla/chrysom client against local Argus with event-sink's empty owner and document the bounded API that enumerates `webhooks` items, generated IDs, TTL fields, and deserialized secret/config fields.
- [x] 1.2 Exercise the curated Caduceus ingestion endpoint with a marked synthetic WRP event and document request encoding, authentication, acknowledgement timing, registration selection, callback headers/signature, and failure responses.
- [x] 1.3 Update `design.md` and delta specs if either spike falsifies owner-empty lookup, synchronous ingestion, callback correlation, or safe direct-callback assumptions before implementing active mode.

## 2. Webhook Journey Model And Provider

- [x] 2.1 Extend journey-specific invocation models with passive/active webhook fields, strict size and format validation, and rejection of cross-journey fields.
- [x] 2.2 Add subscriber intent, Argus, callback, and Caduceus graph nodes/edges with blocking semantics, stable reason IDs, and remediation IDs.
- [x] 2.3 Add a webhook provider registry entry for event-sink-to-WebPA and deterministic expected-graph contract tests.
- [x] 2.4 Extend topology resolution to select one subscriber replica, exactly one WebPA participant, and both persisted loopback endpoints with missing/ambiguous/unsupported tests.
- [x] 2.5 Extend central limits, evidence allowlisting, validation, redaction, and rendering tests for registration fingerprints, HTTP status, correlation state, and participant timestamps.

## 3. Event-Sink Diagnostic State

- [x] 3.1 Add a concurrency-safe bounded event-sink diagnostic state store for intended registration, initial success, refresh success/failure, bounded error category, and expiring callback receipts.
- [x] 3.2 Instrument initial Argus registration and refresh paths to update diagnostic state without changing retry, TTL, health, or logging behavior.
- [x] 3.3 Extend the event-sink webhook handler to recognize only correctly signed reserved diagnostic markers, record bounded direct/Caduceus receipts, return HTTP 204, and suppress normal event logging/processing.
- [x] 3.4 Add event-sink diagnostic HTTP capability, intent, and correlation-receipt handlers using strict bounded schemas and no secret/environment/raw-payload output.
- [x] 3.5 Add race-enabled tests for registration transitions, bounded receipt eviction/expiry, valid diagnostic isolation, invalid HMAC rejection, normal event preservation, and restart-empty state.

## 4. WebPA Argus Inspection

- [x] 4.1 Add a WebPA webhook diagnostic handler using the spike-proven ancla/chrysom lookup path with bounded candidate enumeration and existing source-local Argus credentials.
- [x] 4.2 Implement normalized callback identity matching and explicit zero-match, duplicate-match, and excessive-candidate results.
- [x] 4.3 Implement registration freshness evaluation against authoritative duration/until and the event-sink 12-hour TTL/six-hour refresh policy.
- [x] 4.4 Implement conformance comparison for URL, event filter, device matcher, content type, and secret configured/equality state without serializing secret material.
- [x] 4.5 Add HTTP and model tests for reachable/auth-failed, missing, ambiguous, expired, near-stale, conformant, field-mismatch, secret-mismatch, malformed, oversized, and redaction cases.

## 5. Direct Callback Isolation

- [x] 5.1 Implement active-consent gating so no callback code runs in passive mode or after registration presence/freshness/conformance failure.
- [x] 5.2 Implement bounded callback URL parsing and DNS/transport checks from the WebPA namespace without redirects or arbitrary caller-supplied targets.
- [x] 5.3 Implement one signed diagnostic callback using the stored secret, reserved marker, random correlation ID, bounded body, strict deadlines, and no secret/signature/body evidence.
- [x] 5.4 Poll subscriber correlation state over its persisted loopback endpoint and distinguish DNS, transport, HMAC rejection, HTTP rejection, accepted-but-unrecorded, and recorded receipt outcomes.
- [x] 5.5 Add deterministic tests for direct callback success, DNS failure, connection failure, timeout, redirect refusal, 401, non-success status, missing receipt, and secret non-disclosure.

## 6. Caduceus Routing And Delivery

- [x] 6.1 Validate operator event and device identity against the stored event regex and device matcher before synthetic injection, with invalid-regex and mismatch tests.
- [x] 6.2 Implement the spike-proven bounded synthetic WRP event injection through Caduceus using a distinct correlation marker and source-local endpoint/auth configuration.
- [x] 6.3 Poll subscriber receipt state and separately report Caduceus ingestion acknowledgement and signed callback receipt.
- [x] 6.4 Add tests for successful selection/delivery, event mismatch, device mismatch, ingestion rejection, ingestion timeout, accepted-without-receipt, duplicate receipt, and subscriber restart.

## 7. Multi-Participant Orchestration

- [x] 7.1 Extend the diagnostic HTTP client with strict subscriber-intent, WebPA inspection/active, and receipt-poll request/response contracts and per-operation limits.
- [x] 7.2 Implement passive orchestration that gathers subscriber intent and WebPA Argus observations while marking active stages not exercised.
- [x] 7.3 Implement active orchestration in causal order: intent, Argus inspection, direct callback, subscriber receipt, Caduceus injection, and Caduceus receipt.
- [x] 7.4 Merge participant observations into one timestamped graph, recompute first failure centrally, and preserve transport/protocol errors as incomplete-result failures.
- [x] 7.5 Add orchestrator state-matrix tests for every registration, direct callback, and Caduceus boundary plus participant unavailability and malformed responses.

## 8. CLI And Output Contracts

- [x] 8.1 Extend `vcpe diagnose` parsing for `--to webhook`, `--allow-active-callback`, `--event`, and `--device-id` with journey-specific required/incompatible flag validation.
- [x] 8.2 Extend daemon protocol forwarding for webhook journey inputs and add round-trip protocol tests.
- [x] 8.3 Add structured diagnose help and golden updates for passive and active webhook examples, generated-traffic warning, and CPE/WebPA flag separation.
- [x] 8.4 Wire local dispatch to the webhook provider and multi-participant orchestrator without changing CPE-to-WebPA behavior or passive status collection.
- [x] 8.5 Add ASCII and JSON golden tests for passive-inconclusive, fully healthy active, registration failure, direct callback failure, and Caduceus delivery failure graphs.

## 9. Integration And Documentation

- [x] 9.1 Add HTTP integration coverage with independent subscriber and WebPA test servers proving the control plane uses only persisted loopback endpoints and sends no active traffic in passive mode.
- [x] 9.2 Add an opt-in deployed smoke for fresh Argus registration, successful direct callback, successful Caduceus delivery, and correlated subscriber receipts.
- [x] 9.3 Add deployed failure phases for missing/expired registration, unreachable callback, invalid signature, and event-filter mismatch with first-failure assertions.
- [x] 9.4 Add regressions proving diagnostic callbacks are not logged as normal events and ordinary signed Caduceus events remain unchanged.
- [x] 9.5 Document command usage, active consent, representative event/device selection, graph states, generated traffic, security limits, and supported subscriber types.
- [x] 9.6 Document CPE-event-to-webhook end-to-end correlation and visual-editor overlays as follow-up capabilities rather than partial behavior in this journey.

## 10. Verification

- [x] 10.1 Run focused diagnostic, event-sink, WebPA handler, CLI/help, renderer, and HTTP integration tests and resolve all change-related failures.
- [x] 10.2 Run race tests for event-sink diagnostic state and callback handling.
- [x] 10.3 Run `go test ./...`, `go vet ./...`, binary builds, strict OpenSpec validation, and webhook diagnostic smokes; record branch-gated, live-port, or runtime-dependent exclusions with evidence.

Verification notes:

- `controlplane go test ./...` exercised all packages but `internal/app` still has pre-existing environment failures: `TestAppliedDeploymentStatusCollectsGenericHealthOverHTTP`, `TestNamedStatusCollectsPersistedHealthOverHTTP`, and `TestNamedStatusHealthStateMatrix` could not bind already-occupied loopback ports `47003` or `47000`; `TestRunRelease_CoherenceFailure` requires the `main` branch while this checkout is on `development`.
- `controlplane go vet ./...`, `go build -o bin/vcpe ./cmd/vcpe`, and `go build -o bin/vcpe-healthd ./cmd/vcpe-healthd` passed. Event-sink `go test ./...`, `go vet ./...`, and `go build ./cmd/event-sink` passed.
- Deployed webhook smokes parsed and exited through their safe opt-in guards. Full Podman execution requires `VCPE_RUN_DEPLOYED_WEBHOOK_DIAGNOSTIC=1` or `VCPE_RUN_DEPLOYED_WEBHOOK_DIAGNOSTIC_FAILURES=1` and a live runtime.
