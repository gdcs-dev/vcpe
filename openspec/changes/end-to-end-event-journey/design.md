## Context

The CPE-to-WebPA journey establishes application-to-Parodus evidence and Talaria device registration. The webhook journey establishes subscriber intent, Argus registration, Caduceus delivery, and a bounded callback receipt. Both use versioned diagnostic graphs and persisted loopback HTTP endpoints, but their results are independent and do not prove one event traversed both paths.

This change composes those foundations for the supported Gateway CPE source and the event-sink subscriber. It must preserve diagnostics' no-container-runtime contract, keep generated traffic explicit, and never leak WRP credentials, Argus credentials, webhook secrets, HMAC signatures, or arbitrary event bodies.

## Goals / Non-Goals

**Goals:**

- Answer whether one bounded, marked representative event was accepted by the selected CPE application's Parodus client, processed by WebPA routing, and acknowledged by the selected subscriber callback.
- Attribute the first confirmed causal boundary while preserving passed observations from independently verifiable downstream probes.
- Reuse the existing CPE and webhook providers, persisted endpoints, safety pass, graph renderer, and exit classification.
- Make active generation opt-in, deterministic, bounded, and isolated from normal subscriber event processing.

**Non-Goals:**

- Trace arbitrary production events or persist telemetry for later correlation.
- Prove semantics of application-defined event payloads beyond callback receipt.
- Support arbitrary CPE types, subscribers, callback URLs, destinations, credentials, or event payloads.
- Change normal Parodus, Argus, Caduceus, event-sink, health, manifest, or callback behavior.
- Measure performance, retries, throughput, or delivery latency.

## Decisions

### 1. Introduce a composed `cpe-webpa-callback` journey

The CLI resolves one CPE source replica, one receive-enabled client service, one event-sink subscriber replica, and one WebPA service from persisted deployment state. It invokes a new provider that creates a single ordered graph rather than joining two rendered graphs after the fact. The journey identifier is `cpe-webpa-callback`, exposed as `--to callback`.

The graph has these causal stages:

1. CPE application-to-Parodus client evidence
2. CPE Talaria DNS, transport, authentication, and device registration
3. Subscriber intent and authoritative Argus registration validation
4. Active marked event accepted by the selected CPE/Parodus path
5. WebPA/Caduceus routing acceptance and registration selection
6. Correctly signed subscriber callback receipt for the same correlation ID

The existing CPE and webhook providers remain independently callable. Joining rendered output was rejected because output text lacks sufficient stable internal identity and cannot supply a shared event correlation ID.

### 2. Require explicit active consent and fixed selections

The command is:

```text
vcpe diagnose --name <deployment> --from <cpe-service> --to callback \
  --client-service <name> --subscriber <service> --event <destination> \
  --device-id <id> --allow-active-event [--replica <index>] [--subscriber-replica <index>] [--json]
```

All selection fields are required. `--allow-active-event` is mandatory because the journey sends traffic through the configured CPE event path. Values use the existing bounded identifier and WRP validation rules, and are checked against the subscriber's stored registration filters before sending. The control plane rejects incompatible journey flags before state or HTTP access.

Passive partial results are intentionally not offered: this journey's purpose is delivery correlation, which cannot be claimed without a marked event. Operators can use the existing passive webhook journey for registration inspection.

### 3. Preserve one controlled event and one correlation identity

After prerequisites pass, the CPE diagnostic endpoint creates one minimal WRP simple-event using the selected destination and device identity. It includes a reserved diagnostic extension containing an opaque random correlation ID. No caller can supply raw WRP, headers, source, target URL, credentials, callback URL, or body.

The CPE's normal Parodus transport carries the event to WebPA. WebPA extracts the allowed marker only in its diagnostic route and makes it available to Caduceus routing. Event-sink recognizes the marker only after normal HMAC validation, records an expiring bounded receipt, responds `204`, and skips normal log/application handling. Correlation IDs are unguessable, size-limited, single-use, and never displayed in full.

Injecting directly into Caduceus was rejected because it proves only the second half of the path. Allowing arbitrary event payloads was rejected because it converts diagnosis into an application event injection interface.

### 4. Coordinate through persisted loopback endpoints only

The control plane performs capability discovery and invokes the CPE, WebPA, and subscriber diagnostic endpoints through persisted loopback health records. It never discovers containers, calls a runtime CLI, execs into a workload, or opens an arbitrary network target. WebPA and event-sink retain source-local credentials and secrets; the control plane forwards only bounded sanitized intent, correlation metadata, and selection inputs.

The orchestrator runs probes in order, passes correlation metadata only after validation, and performs bounded receipt polling. Missing endpoints, unsupported capabilities, malformed HTTP, or restart-lost receipt state are invocation/protocol errors or unknown observations as appropriate; they never become fabricated delivery failures.

### 5. Reuse central graph safety and causality

Participant responses use strict schema decoding and capped request/response, evidence, candidate, polling, and total-duration limits. The merged result is centrally validated and redacted before ASCII or JSON rendering. Edges use `passed`, `failed`, `unknown`, and `skipped`; a confirmed earliest failure becomes `firstFailure`, while unknown evidence produces an inconclusive non-zero result.

The callback receipt is the sole proof of full completion. A CPE acceptance or Caduceus acknowledgement without a matching receipt is not treated as delivery success.

### 6. CPE provider capability spike outcome

The Gateway AppArmor simulator owns a receive-enabled `libparodus` client and already exposes an RBUS method that emits exactly one normal fixture event. Gateway callback support adds a root-only local Unix socket at that application boundary. It accepts only the validated fixed client name, selected destination, matching local device identity, and bounded correlation token; the simulator itself constructs and sends the marked WRP event. The control plane SHALL NOT inject directly into Scytale, Talaria, or Parodus.

### 7. Passive Caduceus routing observation

The WebPA image builds Caduceus from the pinned upstream `v0.11.13` source with a local patch. The patch is disabled by default and does not alter event intake, subscriber matching, queueing, delivery, retries, or callback payloads. When explicitly enabled in the vCPE Caduceus configuration, it recognizes only the reserved bounded marker, records a capped, expiring `{correlationId, state, observedAt}` observation after existing sender selection, and exposes it through an authenticated Caduceus diagnostic lookup route. It never records payloads, registration details, destinations, callback URLs, or credentials.

Production deployments without this patch create and route the same marked event normally; they simply cannot provide the additional routing observation. A receipt remains the proof of callback delivery.

## Risks / Trade-offs

- [A diagnostic event affects a deployed system] -> Require an explicit consent flag, allow only one bounded reserved event, and suppress normal subscriber processing after HMAC validation.
- [The CPE source cannot safely emit a diagnostic event through its real client] -> Report the feature unsupported for that provider; do not emulate the CPE from the control plane.
- [Routing can acknowledge before callback delivery] -> Distinguish ingress/routing acknowledgement from the bounded subscriber receipt poll.
- [Correlations can be lost during restarts] -> Store expiring receipt state in memory and report unknown with a stable restart/receipt reason.
- [Existing foundation changes remain unarchived or unsynced] -> Treat their providers and endpoint contracts as prerequisites; implementation begins only once both are available in the working tree.
- [Secrets appear in multi-party evidence] -> Keep secrets local to owning workloads and run the existing central redaction pass on the merged graph.

## Migration Plan

1. Ensure the CPE connectivity and webhook callback journey implementations are present and their capabilities are registered.
2. Add the composed invocation, capability discovery, and provider contracts behind the existing diagnostic endpoints.
3. Add CPE-originated marked event emission, WebPA correlation forwarding, and event-sink receipt isolation without changing ordinary event processing.
4. Wire CLI parsing, persisted-endpoint orchestration, graph rendering, tests, docs, and opt-in deployed smoke coverage.
5. Roll back by unregistering the composed journey and removing diagnostic-only marker handling; normal production event flow and existing standalone journeys remain intact.

## Open Questions

None. The implementation must first confirm that the selected Gateway diagnostic endpoint can inject one marker-bearing event through its existing Parodus connection without changing normal application behavior; if it cannot, the provider is explicitly unsupported rather than approximated.