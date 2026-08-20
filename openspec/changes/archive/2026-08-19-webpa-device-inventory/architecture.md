## Overview

Add a passive WebPA-local diagnostic journey that inventories the currently connected Talaria device sessions. Operators invoke `vcpe diag --name <deployment> --from <webpa-service> --to devices`; the control plane resolves one persisted WebPA loopback endpoint, and the WebPA diagnostic handler owns Talaria access and credentials.

## Components

- **Diagnostic model and renderer**: Define a bounded structured device-session list and render it in deterministic human and JSON output.
- **Talaria device probe**: Read Talaria's authenticated `GET /api/v2/devices` endpoint from the WebPA namespace and translate its response into the diagnostic contract.
- **WebPA device provider and resolver**: Select a WebPA source and semantic Talaria target without resolving a CPE, subscriber, Parodus, Argus, or Caduceus participant.
- **Health daemon dispatch**: Advertise and serve the source-local device inventory capability.
- **CLI contract**: Accept `--to devices`, allow only common deployment/source/replica selection, and reject inputs belonging to other diagnostic journeys before state access.

## Key Architectural Decisions

### WebPA-local Talaria collection
**Choice**: Collect the inventory through the selected WebPA health daemon rather than from the control plane.
**Rationale**: Talaria and its Basic-auth credential are local to WebPA; this follows the established Argus webhook inventory ownership boundary.
**Alternatives considered**: Control-plane-to-Talaria HTTP was rejected because it would expose or duplicate endpoint and credential configuration.

### Connected-session inventory, not fleet inventory
**Choice**: Treat the result as Talaria's current in-memory connection registry.
**Rationale**: `GET /api/v2/devices` reports active sessions, so presenting it as durable device inventory would be misleading.
**Alternatives considered**: A generic deployment device list was rejected because no durable fleet authority was identified.

### Explicit bounded structured output
**Choice**: Return sorted structured device records up to an explicit diagnostic limit; omit the list and report an incomplete result when Talaria exceeds it or returns invalid data.
**Rationale**: A passive inventory must not silently serialize an unbounded response or misrepresent a partial result as authoritative.
**Alternatives considered**: Passing Talaria's raw JSON through unchanged was rejected because the CLI output contract would be unstable and unbounded.

### Operator-visible device identities
**Choice**: Expose raw Talaria device IDs and the selected session counters/timestamps to operators.
**Rationale**: Operators explicitly require full inventory visibility for local vCPE diagnostics.
**Alternatives considered**: Opaque identifiers were rejected by the operator requirement.

## Data Flow

```text
vcpe diag --name edge --from webpa --to devices
                    |
                    v
control plane resolver
  - selects one WebPA replica and persisted loopback endpoint
                    |
                    v
WebPA vcpe-healthd: POST /diagnostics/talaria-devices
                    |
                    v
Talaria: GET /api/v2/devices (WebPA-local Basic auth)
                    |
                    v
bounded device-session response
                    |
                    v
validated Result -> ASCII or JSON
```

## Integration Points

- Talaria `GET /api/v2/devices` on the WebPA-local configured endpoint.
- Existing diagnostic model, causality, safety-copy, renderer, provider registry, resolver, orchestrator, and loopback client.
- `cmd/vcpe-healthd` capability registration and the WebPA container's `--diagnostic` arguments.
- Existing local-control-plane CLI specification and help golden fixtures.

## Security Model

- The control plane calls only the selected persisted WebPA loopback endpoint.
- The WebPA probe owns Talaria URL and Basic credentials; neither is caller input nor diagnostic output.
- Device IDs, pending queue depth, message/byte counters, connection time, and uptime are operator-visible.
- Credentials, authorization headers, payloads, WebSocket details, device metadata, container/runtime details, and arbitrary Talaria responses are not returned.
- The journey performs only a bounded Talaria GET and no device message, connection, registration, webhook, or Caduceus mutation traffic.

## Error Handling Strategy

- Invalid CLI options fail before state access.
- Source selection errors fail before diagnostic HTTP activity.
- DNS, transport, authentication, HTTP-status, decode, validation, and limit failures map to bounded failed or unknown observations.
- The handler does not retry requests; the parent diagnostic timeout bounds the one source-local collection attempt.
- An empty list is successful; a result over the diagnostic limit is incomplete and returns no entries.

## Observability Strategy

- Reuse diagnostic graph observations for Talaria reachability and device-list collection.
- Reuse existing diagnostic reason and remediation IDs for source-local transport/protocol failures where applicable; add targeted IDs for inventory decoding and bounds as needed.
- Do not log raw Talaria response bodies or credentials.

## Constraints

- Preserve existing `webpa`, `webhook`, `webhooks`, `callback`, and `parodus` journey behavior and output contracts.
- No generic HTTP proxy or caller-supplied Talaria path, URL, credential, or filter.
- Maintain strict response validation, deterministic ordering, bounded response bodies, and central defensive copying.
- The journey applies only to a WebPA source and requires explicit `--replica` when the selected WebPA service has multiple replicas.

## Diagrams

```text
                  persisted loopback only
control plane ---------------------------------> WebPA health daemon
                                                       |
                              WebPA namespace only     | authenticated read
                                                       v
                                                   Talaria :6200
                                                       |
                                                       v
                                           current connected-device sessions
```