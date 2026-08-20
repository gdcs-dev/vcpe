## Context

The existing `cpe-webpa` journey uses Talaria `GET /api/v2/devices` only to verify a caller-selected device registration. Talaria already maintains the authoritative in-memory registry of currently connected device sessions and serializes device ID, pending queue depth, message/byte counters, connection time, and uptime. The recently added `argus-webhooks` journey establishes the intended pattern for a WebPA-local structured inventory.

This is a cross-cutting diagnostic change: it adds model fields and validation, a WebPA-local probe, expected graph provider/resolver behavior, health-daemon dispatch, provider registration, container capability configuration, CLI grammar/help, rendering, and focused coverage. The architecture and operator-visibility decisions are recorded in [architecture.md](architecture.md) and [decisions.md](decisions.md).

## Goals / Non-Goals

**Goals:**
- Provide `vcpe diag` and `vcpe diagnose --from <webpa-service> --to devices` for a selected deployed WebPA replica.
- Report a deterministic bounded list of Talaria's current connected-device sessions.
- Expose operator-approved raw device IDs and Talaria's non-secret session counters and timestamps.
- Keep Talaria URL and Basic authentication inside the WebPA workload.
- Preserve passive behavior: one bounded GET with no device-directed request or mutation traffic.

**Non-Goals:**
- Do not create a durable fleet or historical device inventory.
- Do not proxy arbitrary Talaria routes, expose the Talaria control API, or query Scytale, Tr1d1um, Caduceus, Themis, or Consul in this journey.
- Do not include device metadata, WebSocket details, headers, credentials, payloads, or container-runtime data.
- Do not change existing CPE registration, webhook, callback, or Parodus diagnostic behavior.

## Decisions

### Journey and graph identity

Use `talaria-devices` as the internal journey and semantic target type `talaria`; keep the operator target as `devices`. The expected graph is WebPA to Talaria with blocking `talaria-reachability` and `talaria-device-inventory` edges. This mirrors the `argus-webhooks` arrangement while accurately naming the data authority.

The resolver selects only one WebPA source and its persisted endpoint. `--replica` remains the only journey-specific selector allowed; no target service endpoint is resolved.

### Response model and validation

Add a `TalariaDevice` record and a `TalariaDevices` list pointer to both endpoint and final results. A valid record contains:

- `id`
- `pending`
- `bytesSent`, `messagesSent`, `bytesReceived`, `messagesReceived`, and `duplications`
- `connectedAt`
- `upTime`

The translated model validates non-empty bounded IDs, non-negative counters, a non-zero connection time, and a parseable non-negative Go duration. The list must be strictly sorted by ID and contain no more than 64 entries. A list pointer distinguishes an authoritative empty registry from an unavailable or incomplete collection.

### One bounded Talaria read

Extend the existing WebPA-local CPE probe or extract a focused Talaria inventory operation from it. The operation uses the configured Talaria devices URL and Basic auth, bounded HTTP response size, strict JSON decoding, and no redirect following. It does not reuse caller-supplied device IDs or request options.

Empty input is a passed collection with an explicit empty list. Authentication failure, transport failure, unexpected HTTP status, decoding failure, invalid record, or over-limit response produces the relevant bounded observation and omits the list. An over-limit result is unknown rather than a partial success.

### Rendering and defensive copying

Central result validation and deep-copy/redaction paths own the list contract. JSON uses a `talariaDevices` structured field only for `talaria-devices`; ASCII names it as a Talaria connected-device inventory and shows an explicit empty state. Existing journeys must omit the field.

### CLI and health-daemon wiring

`--to devices` accepts common `--name`, `--from`, `--replica`, and `--json` behavior. It rejects `--client-service`, `--subscriber`, `--subscriber-replica`, `--allow-active-callback`, `--allow-active-event`, `--event`, and `--device-id` during CLI parsing. WebPA's health daemon advertises and dispatches `talaria-devices`; built-in type registration supplies the provider.

## Risks / Trade-offs

- **Talaria sessions are ephemeral, not durable device state** → Label output as connected-device inventory and document that an empty list is valid.
- **Talaria's configured maximum (100) exceeds the diagnostic bound (64)** → Return an incomplete result with no device entries rather than partial output.
- **Talaria output types change across upstream releases** → Translate and validate only explicitly selected fields; pin tests to valid, malformed, and excess responses.
- **Operator-visible raw IDs may be sensitive outside local development** → This is an explicit operator decision; continue excluding credentials and payloads.
- **Existing CPE probe already uses the Talaria list API** → Keep focused registration behavior unchanged and add an inventory-specific operation to avoid coupling inventory response shape to selected-device checks.

## Migration Plan

1. Add the capability to the WebPA health daemon and control-plane registry.
2. Deploy updated control plane and WebPA image together through normal `vcpe up` reconciliation.
3. Existing diagnostics continue using their current capability names; no persisted-state migration or manifest change is required.
4. Roll back by deploying the prior control plane and WebPA image; the new capability is additive and has no stored state.

## Open Questions

None. The inventory authority, output visibility, passivity, and bounded-result semantics are decided.