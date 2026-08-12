## Why

`vcpe status` currently reports control-plane persistence and reconciliation state, while container health checks are either local OCI probes or absent. Operators cannot determine whether a service is actually ready, including whether a gateway has an active WebPA connection, without manually entering or inspecting containers.

## What Changes

- Add a uniform, structured HTTP `/health` endpoint to curated vCPE service containers.
- Define type-specific health checks and a common JSON response that separates local readiness from dependency readiness.
- Make `vcpe status --name <deployment>` query the published health endpoints over HTTP and include per-instance results in human and JSON output.
- Preserve OCI `HEALTHCHECK` support by making image health checks query the same local endpoint.
- Require Gateway readiness to include an authoritative, current WebPA registration/connection condition; simple Talaria reachability alone is insufficient.
- Define an explicit health configuration policy for `generic-container` services rather than treating a running process as healthy.
- Make the existing manifest `services[].ports` entry render consistently for every registered service type, while keeping control-plane health ports separate and loopback-only.

## Capabilities

### New Capabilities
- `container-health-endpoints`: Uniform in-container HTTP health contract, type-owned checks, and OCI healthcheck integration.
- `deployment-health-reporting`: HTTP-based collection and presentation of live per-instance health in `vcpe status`.

### Modified Capabilities
- `local-control-plane-cli`: `status` gains live deployment health results in its human and JSON output.
- `service-type-registry`: service types declare their health behavior in addition to validation and rendering behavior.
- `desired-state-manifests`: the service `ports` entry has one consistent forwarding contract across all registered types.

## Impact

- `controlplane/internal/app`, persisted deployment state, and CLI output contracts.
- `controlplane/internal/typeregistry` and every built-in service type.
- Curated service images, entrypoints/init configuration, Compose rendering, and committed runtime-init artifacts.
- BNG, WebPA, and event-sink Compose renderers, plus renderer tests for every registered service type.
- Gateway and WebPA need a shared registration-state contract to report an active Parodus/WebPA connection.