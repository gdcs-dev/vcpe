## Overview

This change adds a passive diagnostic inventory for authoritative Argus webhook registrations. The control plane resolves one WebPA instance and calls its already-published loopback diagnostic endpoint. WebPA, which already owns Argus connectivity and credentials for webhook diagnosis, reads the `webhooks` bucket and returns only a validated, non-secret inventory. The control plane renders the result without using container discovery, container exec, or direct Argus access.

## Components

- **CLI command grammar**: Selects the dedicated `webhooks` target and rejects inputs belonging to other diagnostic journeys.
- **Diagnostic registry and resolver**: Resolves one selected WebPA instance and its persisted loopback endpoint without requiring a subscriber participant.
- **WebPA-local inventory probe**: Reuses the deployed ancla/chrysom-compatible Argus bucket reader, validates registrations, normalizes callback identities, and applies the inventory bound.
- **Diagnostic model and renderers**: Carry and present structured registration entries and any inventory-limit state in JSON and deterministic human output.

## Key Architectural Decisions

### Dedicated `webhooks` diagnostic target
**Choice**: Add `--to webhooks`, separate from the existing subscriber-specific `--to webhook` journey.
**Rationale**: The inventory has a different source (WebPA), participant set, output shape, and success meaning. A registration inventory must not imply that any specific subscriber is conformant or that callback delivery works.
**Alternatives considered**: A `--list` option on `--to webhook` was rejected because it mixes mutually exclusive journeys and makes subscriber validation ambiguous.

### Argus is the authoritative source
**Choice**: List registrations from the complete Argus `webhooks` bucket accessible from the selected WebPA instance, regardless of their creator.
**Rationale**: The requested inventory must include event-sink, other vCPE services, stale registrations, and registrations created by external tools. Event-sink intent state cannot provide that view.
**Alternatives considered**: Enumerating only registered event-sink instances was rejected because it omits registrations from elsewhere and duplicates the existing focused diagnostic.

### Bounded safe inventory representation
**Choice**: Return only stable fingerprint, normalized callback identity, filters, matchers, content type, expiry or TTL state, and secret-presence state, with a documented registration-count limit.
**Rationale**: Argus items can include credentials, secrets, raw storage fields, and uncontrolled strings. The shared diagnostic endpoint must remain bounded and safe to print or serialize.
**Alternatives considered**: Returning raw Argus items or direct Argus credentials was rejected as an information-disclosure risk. Removing all inventory limits was rejected because diagnostics must not become an unbounded administrative API.

## Data Flow

```text
operator
  |
  | vcpe diagnose --name edge --from webpa --to webhooks
  v
control plane
  | resolves persisted WebPA loopback endpoint
  v
vcpe-healthd in WebPA namespace
  | authenticated, source-owned Argus request
  v
Argus /store/webhooks
  |
  | compatible decoded items
  v
WebPA inventory probe -- validate, normalize, bound, redact
  |
  v
control plane -- validate and render JSON or ASCII
```

## Integration Points

- Reuses `WebhookProbe.Candidates` and its deployed ancla/chrysom item decoding path.
- Extends the versioned diagnostic capability discovery, invocation, endpoint response, central validation, and output rendering surfaces.
- Reuses persisted health endpoints for WebPA selection and transport.
- Preserves the existing `webhook` subscriber inspection and active-callback journey unchanged.

## Security Model

- The operator-facing control plane has no Argus credential or network path; only WebPA-local code contacts Argus.
- Inventory entries exclude webhook secrets, Argus credentials, authorization headers, raw item bodies, query strings, URL userinfo, signatures, and callback payloads.
- Callback identities are normalized and validated before output.
- The endpoint accepts no caller-selected Argus URL, bucket, credential, or arbitrary query parameters.

## Error Handling Strategy

- Argus reachability, authentication, decode, malformed registration, and inventory-limit errors produce bounded diagnostic observations with stable reasons and remediation rather than raw response details.
- A valid empty bucket is a successful inventory result.
- A bucket exceeding the inventory bound is reported as incomplete or refused according to the resulting specification; it never causes arbitrary-sized data to be returned.
- CLI validation rejects cross-journey flags before persistence lookup or HTTP activity.

## Observability Strategy

- Human and JSON output identify the selected WebPA source, inventory state, registration count, bounded entries, and truncation or limit state.
- Existing diagnostic observation timestamps, reason IDs, remediation IDs, redaction, and transport error conventions remain the observability mechanism.
- No registration secrets, raw items, or request credentials are logged or emitted.

## Constraints

- Must retain the diagnostic system's no-container-runtime contract.
- Must use the deployed ancla/chrysom model rather than private Argus storage decoding.
- Must remain passive: no callback, Caduceus event, mutation, or refresh is triggered.
- Must preserve existing `--to webhook`, `--to callback`, `--to webpa`, and `--to parodus` contracts.

## Diagrams

```text
                    existing focused journey
event-sink intent --------------------------> --to webhook
                  match one registration          |
                                                     v
                                                   Argus
                                                     ^
                                                     |
WebPA -------------------------------------> --to webhooks
              enumerate authoritative bucket       |
                                                     v
                                            safe inventory output
```