## Why

XB10 now runs the patched Parodus route that exposes receive-enabled registered clients, but `vcpe diag --from xb10 --to parodus` remains unavailable because the existing journey is restricted to Gateway. Operators need the same bounded source-local inventory for XB10 without treating it as callback-delivery parity.

## What Changes

- Extend the existing `parodus-clients` diagnostic journey so selected XB10 instances can serve `vcpe diag --name <deployment> --from <xb10-service> --to parodus`.
- Advertise the existing `parodus-clients` capability from the XB10 health daemon.
- Make the expected diagnostic graph source-neutral while preserving Gateway behavior, bounded output, and persisted-loopback-only collection.
- Add XB10-focused resolver, capability, health-daemon, and end-to-end diagnostic coverage.
- Do not add the Gateway-only active callback/event diagnostic path to XB10.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `parodus-client-enumeration`: Expand the supported source instances from Gateway to Gateway and XB10 for the existing bounded source-owned client-list journey.

## Impact

- `controlplane/internal/diagnostic`: provider graph and source-type registration tests.
- `controlplane/internal/types`: built-in XB10 diagnostic provider registration.
- `services/xb10/container/vcpe-healthd.service`: capability advertisement.
- Existing CLI target, source-local Scytale WRP transport, structured result model, renderer, and response bounds are reused without contract changes beyond supported source type.