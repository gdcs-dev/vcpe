## 1. Diagnostic Result Foundation

- [x] 1.1 Add the versioned diagnostic journey, endpoint identity, node, edge, observation, evidence, reason, remediation, and result models with explicit size and cardinality limits.
- [x] 1.2 Implement result validation for schema version, stable unique IDs, valid edge states, graph references, ordering, timestamps, and first-failure consistency, with focused table tests.
- [x] 1.3 Implement prerequisite evaluation that assigns `passed`, `failed`, `unknown`, and `skipped`, selects only the earliest confirmed failure, distinguishes blocking from informational unknown edges, and treats unknown results as inconclusive, with a complete state-matrix test.
- [x] 1.4 Add the central evidence allowlisting, truncation, and redaction pass and test credential, authorization-header, token, oversized-message, excessive-evidence, and excessive-graph cases.

## 2. Journey Provider And Topology Resolution

- [x] 2.1 Add an optional diagnostic journey-provider registry independent of `typeregistry.ServiceType`, including duplicate registration, supported source/target, and deterministic lookup tests.
- [x] 2.2 Implement persisted deployment resolver logic for deployment name, source service, source replica, and exactly one WebPA target, with tests for missing, unsupported, out-of-range, and ambiguous selections.
- [x] 2.3 Investigate and test whether Gateway and XB10 expose authoritative per-application Parodus client evidence; use Parodus's existing Scytale-accessible `service-status/<service>` response for receive-enabled clients and retain unknown for unavailable evidence.
- [x] 2.4 Implement shared CPE-to-WebPA expected-path metadata and register Gateway and XB10 journey providers with sanitized local Parodus endpoint, Talaria endpoint, and device-identity derivation.
- [x] 2.5 Add provider contract tests proving Gateway and XB10 produce the same ordered v1 stages while retaining type-specific endpoint metadata and identity inputs.

## 3. Workload-Local HTTP Diagnostic Support

- [x] 3.1 Extend the common per-instance HTTP server on port 9878 with bounded `GET /diagnostics` capability discovery and method-restricted active journey routing while preserving `GET /health` behavior.
- [x] 3.2 Implement Gateway `POST /diagnostics/cpe-webpa` handling with bounded application/Parodus evidence, Talaria DNS, transport, authentication, and registration observations using source-local configuration and credentials.
- [x] 3.3 Implement XB10 `POST /diagnostics/cpe-webpa` handling with the same response contract while preserving XB10-specific runtime behavior and identity derivation.
- [x] 3.4 Add HTTP handler and probe tests for capability discovery, method rejection, healthy, DNS failure, connection failure, HTTP 401, missing registration, malformed response, timeout, body limits, and secret non-disclosure cases.
- [x] 3.5 Verify common health checks and diagnostic handlers use the same Talaria endpoint, credentials source, and device identity so readiness and diagnosis cannot silently disagree.

## 4. HTTP Collection And Diagnostic Orchestration

- [x] 4.1 Add a bounded diagnostic HTTP client with persisted-loopback target validation, capability discovery, active POST invocation, context deadlines, response-size limits, strict schema decoding, and HTTP test-server coverage.
- [x] 4.2 Implement source diagnostic endpoint resolution by reusing persisted per-instance health endpoint records without Podman, Docker, Compose, container-name discovery, or container exec.
- [x] 4.3 Implement the CPE-to-WebPA orchestrator to resolve the expected graph, verify advertised capability, merge returned observations, recompute causal ordering and first failure, and timestamp collection.
- [x] 4.4 Implement completed-result exit classification for passed, failed, and inconclusive journeys while keeping invocation, topology, and pre-result HTTP transport/protocol errors distinct.
- [x] 4.5 Add orchestrator tests for fully healthy, local Parodus failure, unavailable application evidence, DNS failure, transport failure, authentication failure, missing registration, probe timeout, endpoint unavailable, and invalid HTTP response paths.

## 5. CLI And Output Contracts

- [x] 5.1 Add `diagnose` to the top-level Go command registry and parse required `--name`, `--from`, and `--to` plus optional `--replica` and `--json`, with command-validation tests.
- [x] 5.2 Add structured `vcpe diagnose` help, examples, global command listing, help-coverage assertions, and updated golden files.
- [x] 5.3 Wire local command dispatch to deployment resolution, provider lookup, the bounded diagnostic HTTP client, and diagnostic exit classification without changing `status` collection behavior.
- [x] 5.4 Implement deterministic color-independent ASCII graph rendering with state labels, skipped stages, first-failure or inconclusive details, bounded evidence, and remediation text, with golden tests.
- [x] 5.5 Implement `vcpe.dev/diagnostic/v1` JSON serialization from the same validated result model and add stable-schema golden tests for healthy, failed, and inconclusive results.

## 6. Integration And Documentation

- [x] 6.1 Add deployed integration coverage for a healthy Gateway-to-WebPA path and assert `vcpe` communicates only with the source's persisted loopback HTTP endpoint while every expected stage and final registration observation pass.
- [x] 6.2 Add representative deployed failure coverage that proves the ASCII and JSON outputs identify the first causal DNS or authentication boundary and skip dependent stages.
- [x] 6.3 Add regression coverage proving `vcpe status` remains passive, requests only `GET /health`, and never invokes active diagnostic routes or a container CLI.
- [x] 6.4 Document the diagnostic command, graph states, exit behavior, supported source types, evidence limitations, and the boundary between health and diagnosis.
- [x] 6.5 Document webhook registration, callback delivery, visual-editor overlays, and end-to-end event correlation as follow-up capabilities rather than partial behavior in this command.

## 7. Verification

- [x] 7.1 Run focused diagnostic, app CLI/help, Gateway probe, and XB10 probe tests and resolve all change-related failures.
- [x] 7.2 Run `go test ./...`, `go vet ./...`, strict OpenSpec validation, and the new HTTP diagnostic smokes; record any environment-dependent deployed-test exclusions with evidence.

## 8. Parodus Client Evidence Follow-up

- [x] 8.1 Query the configured receive-enabled client through Scytale using a direct WRP Retrieve and decode the bounded msgpack response.
- [x] 8.2 Report correlated `online` as passed, `offline` as failed, and transport/protocol/unconfigured outcomes as non-blocking unknown without exposing credentials.
- [x] 8.3 Update focused tests, deployed smoke expectations, documentation, dependency metadata, and strict validation for authoritative client evidence.

## 9. Client Service Selection

- [x] 9.1 Add `--client-service <name>` CLI and daemon protocol fields with stable-identifier validation.
- [x] 9.2 Send the selection in a bounded strict loopback HTTP invocation and reject unknown fields, oversized bodies, trailing JSON, and path-like values before running probes.
- [x] 9.3 Override only the final Parodus client service segment, retain source-owned WRP configuration, and cover CLI, HTTP, probe, help, docs, and smoke behavior.

## 10. Explicit Client Selection

- [x] 10.1 Make `--client-service <name>` required and reject omission before state or HTTP access.
- [x] 10.2 Require non-empty `clientService` in the strict active diagnostic invocation and remove environment/image/service-unit defaults.
- [x] 10.3 Update tests, help, specs, docs, and smoke examples to use explicit client selection.

## Verification Notes

- `go test ./...` passed every changed package; the only failure was the pre-existing `TestRunRelease_CoherenceFailure`, which requires the `main` branch and rejects the current `development` branch. `go test ./internal/app -skip TestRunRelease_CoherenceFailure` passed.
- `go vet ./...`, `go build -o bin/vcpe ./cmd/vcpe`, and `openspec validate cpe-webpa-connectivity-diagnostics --strict` passed.
- `tests/smoke/cpe-webpa-diagnostic.sh` passed `bash -n` and its safe default guard. The full Gateway/WebPA provisioning path was skipped because `VCPE_RUN_DEPLOYED_DIAGNOSTIC=1` was not explicitly authorized; the script records this as `SKIP` and contains both healthy and stopped-WebPA failure assertions.
- The staged Linux/arm64 `vcpe-healthd` was run temporarily on port 9988 inside the live `example-gateway-1` namespace. `apparmor-simulator` returned `client-evidence=online` with all five stages passed; `not-a-real-client` returned `parodus-client-offline` while all four downstream WebPA stages remained passed. The temporary binary and process were removed after each check.
- The same temporary endpoint was first invoked with `{"clientService":"apparmor-simulator"}` and then with `{"clientService":"config"}`. The latter returned `client-service=config`, `client-evidence=online`, and all five stages passed, confirming each request explicitly selects only the client service.
