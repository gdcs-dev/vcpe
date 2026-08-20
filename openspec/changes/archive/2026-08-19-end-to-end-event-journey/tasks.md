## 1. Foundation And Contract Spike

- [x] 1.1 Verify Gateway can emit exactly one bounded marker-bearing WRP simple event through its existing Parodus path without altering normal application behavior; document unsupported providers rather than approximating the path.
- [x] 1.2 Define the `cpe-webpa-callback` journey identity, bounded correlation token, selection models, graph nodes/edges, stable reason/remediation IDs, and provider registration with validation tests.
- [x] 1.3 Extend central diagnostic limits, strict JSON decoding, evidence allowlisting, graph validation, redaction, rendering, and exit classification for the composed journey.
- [x] 1.4 Add persisted deployment resolution for one supported CPE replica, one WebPA service, one supported subscriber replica, and their loopback diagnostic endpoints, including missing, ambiguous, and unsupported cases.

## 2. Workload-Local Correlation Protocol

- [x] 2.1 Add strict, bounded CPE diagnostic invocation and capability discovery for one active marker-bearing event, requiring validated client service, destination, device identity, and correlation metadata.
- [x] 2.2 Implement Gateway provider support for the marker-bearing event path, including source-owned application/Parodus evidence and no raw WRP, credentials, or event-body output.
- [x] 2.3 Add WebPA diagnostic capability and strict handler contracts that accept only sanitized correlation metadata, inspect the routing outcome, and retain WebPA-owned credentials locally.
- [x] 2.4 Extend event-sink diagnostic state and loopback endpoints with bounded expiring correlation receipts; recognize reserved markers only after HMAC validation, return the isolated response, and preserve ordinary callback processing.
- [x] 2.5 Add race-enabled workload tests for receipt expiry/eviction, duplicate or invalid markers, valid receipt isolation, normal-event preservation, malformed requests, limits, and secret non-disclosure.

## 3. Causal Orchestration

- [x] 3.1 Extend the diagnostic HTTP client with strict capability, active-event, routing-observation, and receipt-poll contracts using only persisted loopback endpoints and bounded deadlines.
- [x] 3.2 Implement the composed provider's prerequisite collection: CPE application/Parodus evidence, Talaria DNS/transport/authentication/device registration, subscriber intent, and authoritative Argus registration validation.
- [x] 3.3 Implement active execution in causal order: generate one CPE marker event only after prerequisites pass, collect WebPA/Caduceus acceptance, then poll the selected subscriber for the matching receipt.
- [x] 3.4 Merge participant observations into the validated graph, distinguish confirmed failures from unknown/restart-lost evidence, skip dependent work correctly, and select the earliest confirmed failure.
- [x] 3.5 Add state-matrix orchestrator tests for every CPE, Talaria, Argus, routing, callback, and receipt boundary, including malformed responses and proof that no runtime/container access is used.

## 4. CLI And Output

- [x] 4.1 Extend `vcpe diagnose` and daemon protocol parsing for `--to callback`, `--client-service`, `--subscriber`, `--event`, `--device-id`, `--allow-active-event`, and valid replica selectors; reject missing, malformed, cross-journey, and incompatible flags before state or HTTP access.
- [x] 4.2 Wire local dispatch to the composed provider while preserving standalone CPE-to-WebPA and webhook journeys and passive `vcpe status` behavior.
- [x] 4.3 Add help, ASCII, and JSON golden coverage for successful correlation, CPE-path failure, routing/callback failure, receipt timeout, and inconclusive restart handling without leaking sensitive correlation data.
- [x] 4.4 Document supported source/subscriber types, command grammar, explicit generated-traffic consent, stage meanings, exit behavior, evidence limits, and the distinction from real-event tracing.

## 5. Integration And Verification

- [x] 5.1 Add HTTP integration coverage with independent CPE, WebPA, and subscriber test servers proving only persisted loopback endpoints are used and exactly one marked event is generated after prerequisites pass.
- [x] 5.2 Add opt-in deployed smoke coverage for successful Gateway-to-event-sink correlation and representative failures at CPE connectivity, registration matcher, routing, invalid callback signature, and absent receipt boundaries.
- [x] 5.3 Run focused diagnostic, Gateway, WebPA, event-sink, CLI/help, race, and integration tests; resolve change-related failures.
- [x] 5.4 Run `go test ./...`, `go vet ./...`, binary builds, strict OpenSpec validation, and the opt-in diagnostic smokes; record branch, live-port, or runtime-dependent exclusions with evidence.

## Verification Notes

- 2026-08-17: `go vet ./...`, `go build -o bin/vcpe ./cmd/vcpe`, strict OpenSpec validation, focused diagnostic/CLI tests, event-sink `go test -race ./...`, and fresh Gateway/WebPA Containerfile builds passed.
- 2026-08-17: `go test ./...` passed every package except `internal/app`'s branch-gated `TestRunRelease_CoherenceFailure`: the checkout is on `development`, while release requires `main`.
- 2026-08-17: enabled callback smoke coverage was excluded before deployment because `gvproxy` owned `127.0.0.1:47001`. The other deployed diagnostic smokes use the same deterministic fresh-state health-port range and were not run against the active deployment. All deployed diagnostic smoke scripts passed `bash -n`.