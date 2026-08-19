## Context

The existing `parodus-clients` journey already performs a bounded source-local Scytale WRP Retrieve and renders structured client names and truncation state. Its implementation is generic in `CPEWebPAProbe`, but its provider registration, expected graph, and Gateway health-daemon configuration restrict the journey to Gateway. XB10 now exposes the same patched `<device>/parodus/client-list` route and its readiness probe derives the same `mac:` identity from `erouter0` with the same source-local Talaria/Scytale defaults.

The implementation must preserve the existing loopback-only control-plane boundary, 64-entry validation, sorted output, and unsupported-source behavior. It must not infer active callback support from the newly available XB10 Parodus route.

## Goals / Non-Goals

**Goals:**
- Allow `vcpe diag --name <deployment> --from <xb10-service> --to parodus` to select an XB10 replica and its persisted health-daemon loopback endpoint.
- Reuse the existing `parodus-clients` route, Scytale WRP request, result model, and renderer.
- Make the expected graph label the selected service rather than a hard-coded Gateway node.
- Advertise the capability from XB10 and cover capability discovery, resolution, collection, and rendering.

**Non-Goals:**
- Do not alter Gateway enumeration behavior, client-list protocol, output fields, or the 64-client bound.
- Do not create an XB10-specific CLI target, generic WRP proxy, or direct control-plane-to-Scytale path.
- Do not add `cpe-webpa-callback`, AppArmor simulation, or active event injection to XB10.

## Decisions

### Register a shared provider per supported source type

Refactor the existing Parodus provider construction to accept a source type, registering it for both `gateway` and `xb10`. The provider retains `JourneyParodusClients` and semantic target type `parodus`; its graph source node is derived from the selected service identity rather than a Gateway literal.

This maintains the registry's exact `(journey, source type, target type)` lookup semantics. A single untyped provider would not represent the registry key model cleanly, while duplicated providers would invite graph drift.

### Reuse XB10's existing CPEWebPAProbe handler

Add `--diagnostic=parodus-clients` to XB10's health-daemon service. `buildDiagnosticJourneys` already maps that capability to `CPEWebPAProbe.RunParodusClients`, which validates invocation fields, checks local Parodus state, performs the bounded correlated Scytale query, and returns no list on unknown outcomes.

No new endpoint, environment variable, credential, or device identity wiring is required. XB10 uses the existing defaults and its `erouter0` MAC derivation, matching its readiness registration probe.

### Test source-specific integration without duplicating protocol tests

Keep shared protocol tests in `cpewebpa_test.go`. Add focused tests that demonstrate:

- the registry finds `parodus-clients/xb10/parodus`;
- resolver selection reaches only the selected XB10 persisted loopback endpoint and enforces `--replica` for multiple XB10 replicas;
- XB10's health-daemon unit declares the capability;
- orchestration returns the same structured list for an XB10 source.

This gives confidence in the new support boundary while avoiding a duplicated WRP fixture suite.

## Risks / Trade-offs

- **XB10 image lacks the patched Parodus route at deployment time** → Capability discovery succeeds but the source-local handler produces the existing bounded `unknown` observation; it never exposes raw failures.
- **XB10 derives an unexpected device identity** → The correlated request fails through the existing response mismatch/unavailable paths; readiness and the current `cpe-webpa` journey provide complementary identity evidence.
- **Provider graph refactor affects Gateway labels** → Retain Gateway output as the existing service name and add regression coverage for both source types.
- **Operators assume all Parodus users appear** → Continue documenting the result as receive-enabled registered clients only.

## Migration Plan

1. Build and stage `vcpe-healthd` for the XB10 target platform with the new capability argument.
2. Build and reconcile the XB10 image so its systemd service advertises `parodus-clients`.
3. Run `vcpe diag --name <deployment> --from <xb10-service> --to parodus --json` and compare the bounded fields with the source-owned patched route.
4. Roll back by deploying the prior XB10 image and control plane; the capability is additive and has no persisted-state migration.

## Open Questions

None. The available XB10 patched route, collection boundary, and active-callback exclusion are established.