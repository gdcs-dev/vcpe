## Overview

Extend the existing source-local `parodus-clients` diagnostic journey from Gateway to XB10. Both service types already use the same `CPEWebPAProbe` identity derivation and Scytale WRP transport, so the control plane selects an XB10 source and its persisted loopback endpoint while the XB10 health daemon queries its own patched Parodus route.

## Components

- **Diagnostic provider registry**: Register the existing `parodus-clients` journey for `xb10` as well as Gateway and produce a source-neutral Parodus graph.
- **Diagnostic resolver**: Continue selecting only the requested source instance and its persisted loopback endpoint; no WebPA target endpoint is resolved.
- **XB10 health daemon**: Advertise and dispatch the existing `parodus-clients` handler using the source-owned Scytale configuration, credentials, and device identity.
- **Diagnostic tests and specifications**: Extend Gateway-only coverage and requirements to the supported XB10 source.

## Key Architectural Decisions

### Reuse the existing Parodus client-list journey
**Choice**: Extend `parodus-clients` to XB10 rather than introduce an XB10-specific journey or response model.
**Rationale**: The patched route, bounded payload, Scytale WRP retrieval, and operator output have identical semantics for both CPE types.
**Alternatives considered**: An `xb10-parodus-clients` journey was rejected because it would duplicate CLI routing, response validation, rendering, and operator documentation without adding meaning.

### Preserve source-local authority
**Choice**: The selected XB10 health daemon performs the correlated Scytale retrieve using its own derived device identity and configured credentials.
**Rationale**: This retains the existing loopback-only control-plane boundary and prevents operator input from choosing WRP destinations, endpoints, or credentials.
**Alternatives considered**: A control-plane-to-Scytale request was rejected because it would duplicate workload-local configuration and broaden credential exposure.

### Keep active callback diagnostics separate
**Choice**: Scope this change to registered-client enumeration and do not add `cpe-webpa-callback` support to XB10.
**Rationale**: Gateway callback diagnostics rely on the AppArmor simulator's dedicated active-event socket; XB10 has no equivalent source-owned event injector.
**Alternatives considered**: Declaring feature parity based only on Parodus availability was rejected because callback delivery requires a distinct active-event design.

## Data Flow

```text
vcpe diag --name example --from xb10 --to parodus
                    |
                    v
control-plane resolver
  - selects one XB10 replica
  - loads its persisted loopback endpoint
                    |
                    v
XB10 vcpe-healthd: POST /diagnostics/parodus-clients
                    |
                    v
Scytale: correlated WRP Retrieve
  mac:<xb10-erouter0-mac>/parodus/client-list
                    |
                    v
XB10 patched Parodus -> bounded client-list response
                    |
                    v
validated diagnostic result -> ASCII or JSON
```

## Integration Points

- Existing `CPEWebPAProbe.RunParodusClients` and Scytale WRP retrieval.
- Diagnostic provider registry, resolver, health-daemon capability discovery, and persistent loopback client.
- XB10 `vcpe-healthd.service` capability arguments.
- Existing `parodus-client-enumeration` specification and CLI target `--to parodus`.

## Security Model

- The control plane reaches only the selected XB10 loopback diagnostic endpoint.
- XB10 retains ownership of Scytale URL, Basic credentials, and device identity derivation.
- Output remains limited to validated receive-enabled client identifiers and truncation state.
- Credentials, raw WRP envelopes, arbitrary Parodus data, and container-runtime details remain excluded.

## Error Handling Strategy

- Unsupported source types fail during resolver/provider lookup before diagnostic HTTP activity.
- A missing XB10 capability fails during capability discovery.
- Inactive Parodus, transport/authentication failures, malformed WRP payloads, and invalid or oversized lists remain bounded `unknown` source-local observations.
- No retry behavior is added; existing diagnostic timeout bounds the operation.

## Observability Strategy

- Reuse the existing `parodus-client-list` observation and reason/remediation identifiers.
- Preserve deterministic structured ASCII and JSON rendering.
- Do not log credentials or raw WRP payloads.

## Constraints

- Preserve Gateway enumeration behavior and output contract.
- Preserve the 64-client bound, lexicographic ordering, and explicit truncation state.
- Require `--replica` for a multi-replica XB10 source.
- Do not add WebPA target resolution, arbitrary WRP requests, or active callback traffic.

## Diagrams

```text
                persisted loopback only
control plane -----------------------------> XB10 health daemon
                                                    |
                                                    | source-owned WRP retrieve
                                                    v
                                               Scytale / Talaria
                                                    |
                                                    v
                                             XB10 patched Parodus
```