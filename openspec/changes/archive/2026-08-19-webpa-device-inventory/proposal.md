## Why

`vcpe diagnose` can prove one selected CPE's Talaria registration, but operators cannot inspect the deployed WebPA instance's current connected-device sessions. A passive Talaria inventory makes it possible to see active device IDs and session health without generating device, callback, registration, or event traffic.

## What Changes

- Add `vcpe diag`/`vcpe diagnose --name <deployment> --from <webpa-service> --to devices` as a passive WebPA-local Talaria connected-device inventory journey.
- Return a bounded, deterministic structured list of operator-visible device session records: device ID, pending queue depth, connection time, uptime, and message/byte counters.
- Add a WebPA health-daemon capability that owns the Talaria request, endpoint configuration, and authentication.
- Add human and JSON rendering, CLI validation/help, and focused diagnostic, health-daemon, and app-level coverage.
- Reject cross-journey client, subscriber, active-callback, active-event, event, device-ID, and subscriber-replica options before state or network access.

## Capabilities

### New Capabilities
- `webpa-device-inventory`: Deployment-targeted passive inventory of currently connected Talaria device sessions through a selected WebPA source.

### Modified Capabilities
- `local-control-plane-cli`: Extend the diagnose CLI contract with the WebPA-local `--to devices` journey and its input restrictions.

## Impact

- `controlplane/internal/diagnostic`: model, validation, Talaria probe, provider, resolver, orchestrator, safety copying, rendering, and tests.
- `controlplane/cmd/vcpe-healthd`: capability dispatch and tests.
- `controlplane/internal/types` and `services/webpa/container/entrypoint.sh`: provider and WebPA diagnostic registration.
- `controlplane/internal/app`: CLI parsing, help/goldens, and command tests.
- No new external service dependency; consumes Talaria's existing authenticated local `GET /api/v2/devices` endpoint.