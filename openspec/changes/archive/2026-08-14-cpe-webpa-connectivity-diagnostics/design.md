## Context

vCPE already publishes curated per-instance health over loopback HTTP. Gateway and XB10 health include `parodus`, `webpa-reachable`, and `webpa-registration`, and registration is checked against Talaria's authenticated live device registry. This answers whether an instance is ready, but it collapses DNS, transport, authentication, and registration into a few checks and does not show developers the expected dependency path or the first broken boundary.

`vcpe status` is intentionally passive and HTTP-only. Diagnosis is explicitly requested and may trigger bounded trusted probes inside a workload, but it can use the same runtime-agnostic communication pattern: a loopback-published HTTP endpoint served from the workload network namespace. The control plane therefore does not need Podman, Docker, Compose, container-name discovery, or container exec to diagnose a source instance.

The initial journey targets a deployed Gateway or XB10 instance and its configured WebPA service. A CPE application is expected to use the source instance's local Parodus endpoint. Parodus already maintains a live client list for receive-enabled libparodus clients and exposes it through the WRP destination `<device>/parodus/service-status/<service>`. Scytale can carry this generic WRP Retrieve synchronously and return Parodus's msgpack response without changes to Parodus or libparodus.

## Goals / Non-Goals

**Goals:**

- Show the expected CPE application to Parodus to Talaria/WebPA path as a compact ASCII graph overlaid with observed state.
- Separate local Parodus readiness, DNS, TCP/HTTP reachability, authentication, and authoritative device registration.
- Identify exactly one first confirmed causal failure and skip dependent downstream stages.
- Emit a stable, versioned JSON graph suitable for automation and later visual clients.
- Keep probes bounded, deterministic, redacted, and testable with ordinary HTTP test servers.
- Keep control-plane diagnostic communication independent of the container runtime.
- Make journey-specific logic extensible without placing protocol knowledge in CLI formatting code.

**Non-Goals:**

- Webhook/Argus subscription registration, Caduceus callback delivery, or end-to-end event correlation.
- A general network scanner, arbitrary user-supplied command runner, or log parser.
- Changing `vcpe status`, the common health schema, or apply readiness behavior.
- Diagnosing applications outside a managed vCPE source instance.
- Adding diagnostic overlays to the VS Code visual manifest editor in this change.

## Decisions

### 1. Add a separate diagnostic command and protocol

The command is `vcpe diagnose --name <deployment> --from <service> --to webpa --client-service <name> [--replica <index>] [--json]`. `--name`, `--from`, `--to`, and `--client-service` are required. Replica zero is selected when the source has one replica; `--replica` is required when it has multiple replicas. Requiring the libparodus service name keeps application identity explicit and avoids image-specific defaults.

Diagnosis receives its own `vcpe.dev/diagnostic/v1` JSON schema rather than extending health responses. Health is continuously safe and service-owned; diagnosis is an on-demand multi-stage operation with expected topology, active observations, causal analysis, and remediation.

The source instance's existing persisted loopback endpoint and container port `9878` serve both protocols. `GET /health` remains unchanged. `GET /diagnostics` advertises supported journey IDs, and `POST /diagnostics/cpe-webpa` triggers the bounded active journey. A separate schema and path prevent active work from occurring during health collection.

Alternatives considered:

- Extend `vcpe status`: rejected because it would violate the status HTTP-only contract and make a routine read command invasive.
- Add more health checks only: rejected because health checks do not model expected graph edges, skipped dependencies, or first-failure causality.

### 2. Model the result as an ordered graph with four observation states

The result contains journey metadata, source and target identities, ordered nodes and edges, observations, and an optional `firstFailure` edge ID. Every observed edge has one of `passed`, `failed`, `unknown`, or `skipped`:

- `passed`: the boundary was positively verified.
- `failed`: the boundary was positively tested and failed.
- `unknown`: the probe could not establish the boundary's state, including missing authoritative application-client evidence.
- `skipped`: a blocking prerequisite failed or was unknown, so evaluating this boundary would be misleading.

Only `failed` can become `firstFailure`. Edges declare whether an unresolved observation blocks following stages. After a blocking edge fails or is unknown, dependent stages are skipped. A result with unknown but no failed stage has no first causal failure and is reported as inconclusive.

The v1 ordered stages are:

1. CPE application to local Parodus
2. Parodus source to Talaria DNS resolution
3. Parodus source to Talaria transport
4. Talaria authentication
5. Device registration in Talaria

The application-to-Parodus stage passes only when a direct Scytale WRP Retrieve returns `{"service-status":"online"}` for the configured Parodus service. `offline` fails the edge. A failed or invalid Scytale query yields `unknown`, not a false application failure, and the edge remains non-blocking because DNS, transport, authentication, and registration can still be independently tested from the Parodus host. Send-only clients configured with `receive=false` never register in Parodus's client list and cannot use this evidence.

Alternative considered: mark every unexecuted downstream stage failed. Rejected because it obscures the earliest causal boundary and confuses consequences with causes.

### 3. Keep protocol knowledge in optional journey providers

Introduce a diagnostic registry separate from the mandatory `typeregistry.ServiceType` interface. A provider declares a journey name, supported source and target types, expected graph metadata, trusted probe definitions, dependency order, and interpretation/remediation mappings. Gateway and XB10 register CPE-to-WebPA providers; WebPA-specific target metadata is shared.

The control-plane provider owns expected-path metadata and interpretation/remediation mappings. The source HTTP service owns workload-local probe definitions and prerequisite execution because it has the correct network namespace, application context, and credentials. The diagnostic orchestrator owns source/target resolution, replica validation, HTTP deadlines, graph assembly, causal validation, redaction, and output limits. Providers and endpoints do not format CLI output and cannot return arbitrary unbounded logs.

Alternative considered: add diagnostics to `ServiceType`. Rejected because diagnostics are optional and journey-specific; forcing every service type, including generic-container, to implement them would widen an otherwise focused registry contract.

### 4. Trigger workload-local probes through the existing HTTP publication

Extend the common per-instance HTTP server on container port `9878` with two diagnostic routes:

- `GET /diagnostics` returns a bounded versioned list of supported journey IDs and performs no active probes.
- `POST /diagnostics/cpe-webpa` runs the source-owned journey and returns `vcpe.dev/diagnostic/v1` stage observations.

`vcpe` obtains the source instance's already-persisted loopback host port and calls these routes with a bounded HTTP client. The active request may contain only `clientService`, validated as a stable identifier with no slash or whitespace. It cannot supply commands, credentials, device IDs, WRP sources or destination prefixes, arbitrary target URLs, or probe definitions. The source constructs the fixed `<device>/parodus/service-status/<clientService>` destination and uses its own configured Talaria/Scytale endpoints, credentials, and device identity. Its response includes only allowlisted evidence and never returns credentials or raw logs.

Gateway and XB10 host the routes in their existing health service so probes run in the workload network namespace. A generic container can support the contract itself or use a vCPE-owned companion that shares its network namespace. A companion can observe DNS and transport but cannot infer application process state, credentials, or a specific Parodus client unless those signals are explicitly exposed; unavailable stages remain `unknown`.

For curated sources, the handler requires the validated invocation's `clientService` and posts a JSON WRP Retrieve to `VCPE_SCYTALE_URL` (default `http://scytale:6300/api/v3/device`). It uses the same source-local WebPA credentials and device identity as readiness checks, validates transaction correlation, decodes the bounded msgpack response with `wrp-go`, and exposes only the client service and `online`/`offline` status as evidence. There is no environment or image default for the client service. Diagnostic HTTP callers cannot choose the WRP destination prefix or credentials.

The control plane validates capability discovery and the diagnostic response, recomputes causal ordering and `firstFailure`, and applies final redaction before rendering. It never invokes a container backend or container CLI for diagnosis.

Alternatives considered:

- Infer all stages from existing health responses: rejected because current checks cannot distinguish DNS, transport, and authentication and cannot expose bounded expected-versus-observed metadata.
- Execute probes with `podman exec`: rejected because it couples diagnosis to a runtime, requires container-name discovery, and duplicates the established loopback HTTP transport.

### 5. Resolve expected topology before active execution

The orchestrator loads the latest persisted desired and planned deployment state, verifies the source and WebPA target, selects the source replica, resolves the source's persisted loopback endpoint, and asks the provider to construct the expected path. Unsupported source types, missing targets, ambiguous WebPA targets, missing endpoints, and stale/missing deployment state fail before any diagnostic HTTP request.

Expected display metadata may include service names, replica, sanitized endpoint host and port, local Parodus endpoint, and derived device ID. Secret values, complete environment dumps, and authentication headers are prohibited.

`--to webpa` selects exactly one deployed service whose registered type is `webpa`; zero or multiple candidates produce an actionable error. This keeps v1 syntax simple while preserving target service identity in the result schema.

### 6. Render the same graph model for humans and machines

The orchestrator returns only the structured result. The human renderer produces deterministic ASCII with node labels, state-marked edges, first-failure details, bounded evidence, and remediation text. It does not depend on terminal color or Unicode. The JSON renderer serializes the same model with schema version and stable machine-readable reason and remediation IDs.

The graph contract is intentionally presentation-neutral so a later visual-editor change can render it without scraping terminal output.

### 7. Bound and redact every diagnostic observation

The diagnostic request and source-local probes have bounded deadlines. HTTP bodies, evidence entries, graph cardinality, and message lengths have explicit limits. The control plane applies a final redaction and validation pass even when an endpoint claims its output is safe. Raw command lines, environment blocks, credentials, tokens, request payloads, and unbounded logs are never included.

The command exits non-zero for invalid invocation, unsupported topology, HTTP transport or protocol failure that prevents constructing a valid result, and a completed diagnostic containing a confirmed failed stage. An inconclusive result is valid output but exits non-zero. A fully passed path exits zero.

## Risks / Trade-offs

- [Parodus client status covers only receive-enabled libparodus clients] -> Document the limitation and report `unknown` when no valid configured client status can be obtained.
- [Active probes can alter load or trigger authentication auditing] -> Use read-only endpoints, strict deadlines, no retries by default, and run only on explicit `diagnose` invocation.
- [An active endpoint increases the responsibility of the health HTTP process] -> Separate active diagnostic routes from `GET /health`, enforce strict request methods and limits, and keep health collection behavior unchanged.
- [A network-sharing companion cannot observe arbitrary application internals] -> Require explicit diagnostic signals for application-level claims and report unsupported evidence as `unknown`.
- [Health and diagnostic scripts can drift] -> Reuse image-owned helpers where possible and add parity tests around shared checks and identity derivation.
- [Provider output could leak credentials] -> Do not pass secrets in arguments, use allowlisted evidence fields, cap output, and apply central redaction before rendering or serialization.
- [Multiple source replicas or WebPA targets can make causality ambiguous] -> Require explicit source replica selection and reject ambiguous target resolution in v1.
- [A downstream service can change between sequential probes] -> Timestamp each observation and keep the overall journey deadline short; do not claim an atomic distributed snapshot.

## Migration Plan

1. Add the graph model, validation, redaction, and causal evaluator with no CLI exposure.
2. Extend the common HTTP server with diagnostic capability discovery and active journey routing.
3. Add Gateway/XB10 and WebPA journey-provider metadata and workload-local HTTP probe support.
4. Add the bounded HTTP collector and integration tests against representative healthy and failed paths.
5. Expose `vcpe diagnose`, structured help, ASCII rendering, and JSON output.
6. Document usage and the distinction between status and diagnosis.

The change is additive and requires no state or manifest migration. Rollback consists of removing the command and provider registration; existing deployments and persisted state remain compatible.

## Open Questions

- No open questions remain for v1 client selection; the operator must provide the exact receive-enabled libparodus service name.
