## Why

XB10 already runs a root-only, bounded AppArmor simulator diagnostic socket that accepts the same validated correlated-event protocol as Gateway, but vCPE does not advertise or select XB10 for the active CPE-to-callback journey. Operators therefore cannot establish one correlated XB10-to-subscriber callback path even though the source-local capability is available.

## What Changes

- Extend the existing `cpe-webpa-callback` journey so XB10, as well as Gateway, can be a supported CPE source when it exposes the fixed AppArmor simulator diagnostic socket.
- Advertise the callback journey from the XB10 health daemon and enable its source-local emitter through the fixed root-only socket path.
- Register an XB10 callback provider while retaining the existing graph, prerequisite ordering, explicit active-event consent, validation, correlation, redaction, and event-sink subscriber constraints.
- Add focused unit, protocol, capability, and opt-in deployed XB10 smoke coverage proving that exactly one marked event is emitted only after prerequisites succeed.
- Update operator documentation to describe both supported CPE sources and clarify that callback diagnostics are distinct from Parodus client inventory.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `end-to-end-event-journey`: Permit XB10 as a supported source for the bounded correlated CPE-to-callback diagnostic journey when its source-local simulator capability is present.

## Impact

- `controlplane/internal/diagnostic`: callback provider source support and fixed-socket emitter eligibility.
- `controlplane/internal/types`: XB10 callback provider registration.
- `services/xb10/container/vcpe-healthd.service`: callback capability advertisement and fixed diagnostic socket configuration.
- `docs/health.md`, `docs/runbook.md`, and `docs/cpe-webpa-callback-diagnostic.md`: supported-source guidance and XB10 command examples.
- Focused Go diagnostic, daemon, provider/registry, and optional Podman smoke tests; no manifest, public CLI flag, container-exec, or arbitrary WRP injection changes.