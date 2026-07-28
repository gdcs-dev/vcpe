## Context

Container interface naming across BNG, Gateway, and XB10 is controlled by hard-coded MAC alias tables in bash entrypoints. The manifest's `Interface.device` field already exists in the schema and the planner already passes it through as `IFACE_*_DEVICE` via `render.IfaceEnv()`. The gap is entirely in the renderers (which emit redundant legacy aliases) and the container entrypoints (which use those aliases instead of `IFACE_*_DEVICE`).

The three service types with renaming logic each have the same structural problem:
- Renderer emits `<ROLE>_MAC=` aliases (e.g., `MGMT_MAC=`, `WAN0_MAC=`, `LAN1_MAC=`)
- Entrypoint uses those aliases to build a hard-coded `target_by_mac` table

The fix is symmetric for all three: drop the aliases from the renderer, rewrite the rename loop to iterate `IFACE_*_MAC`+`IFACE_*_DEVICE` pairs. Gateway has additional complexity because `configure_networking()` also references hard-coded names and creates the `brlan0` bridge.

## Goals / Non-Goals

**Goals:**
- `Interface.device` in the manifest is the single source of truth for the container-internal interface name
- All complex service entrypoints use a generic MAC rename loop driven by `IFACE_*_MAC`/`IFACE_*_DEVICE` env vars
- Gateway `configure_networking()` reads interface names from env vars, not hard-coded strings
- Gateway LAN bridge name comes from `config.lan.bridge` (defaulting to `brlan0`)
- `ServiceType.ValidateInterfaces()` enforces device names on gateway interfaces at preflight
- Visual editor property panel makes `device` editable per interface

**Non-Goals:**
- Changing the planner's `eth{pos}` default for services whose type does not require device names
- Making WebPA/event-sink entrypoints do MAC-based renaming (they use Podman's default single-interface naming which consistently produces `eth0`)
- Renaming the `brlan0` bridge across existing deployments (state migration)

## Decisions

### ValidateInterfaces on ServiceType interface

Adding `ValidateInterfaces([]manifest.Interface) error` to the existing `ServiceType` interface is a breaking change (all 7 implementations must add the method). The default implementation returns `nil`. Gateway's implementation rejects any interface with an empty `device` field.

Alternative considered: gateway-specific check in `preflight.go`. Rejected because it would mix per-type validation logic into the generic preflight layer and violates the extension pattern already established by `ExpectedRoles()`.

### Canonical env var contract: IFACE_*_MAC / IFACE_*_DEVICE

`render.IfaceEnv()` already emits these for every interface. Entrypoints previously ignored them in favor of role-specific aliases. The new contract: entrypoints consume ONLY `IFACE_*_{DEVICE,MAC,IPV4,...}` — the legacy aliases (`MGMT_MAC=`, `EROUTER0_MAC=`, etc.) are removed from all renderers.

This is a hard cut: no backward compatibility shim. The entrypoints and renderer changes must be deployed as a unit (a rebuilt image is required before `vcpe up` will succeed).

### Gateway: synthesized LAN_DEVICES and LAN_BRIDGE env vars

The entrypoint needs to know which interfaces to bridge. Rather than parsing role names inside bash, the renderer synthesizes:
- `LAN_DEVICES=eth0 eth1 eth2 eth3` — space-delimited list of device names for roles matching `lanPrefix`
- `LAN_BRIDGE=brlan0` — from `cfg.LAN.Bridge`, defaulting to `brlan0`

The entrypoint iterates `$LAN_DEVICES` and bridges each to `$LAN_BRIDGE`. This keeps the entrypoint simple while the role-to-function mapping stays in Go.

### Runtime-init binaries

The runtime-init binaries in `services/*/container/` are pre-compiled from `controlplane/cmd/runtime-init-*`. They are NOT entrypoints — they are called BY the entrypoints. Entrypoint changes do not require restaging runtime-init binaries. Only changes to `controlplane/cmd/runtime-init-*` or `internal/runtimeinit/contract` require restaging.

### Visual editor: device field editable in ServiceForm

The `ServiceForm` in `PropertyPanel.tsx` currently shows interfaces read-only. The `device` field will be added as an editable `Field` per interface row, using `setScalar(['spec','services',i,'interfaces',j,'device'], value)` on blur. The interface `role` remains read-only (rename is blocked by the visual editor's existing constraint).

## Risks / Trade-offs

**[Risk] Coordinated deployment required** — The renderer change (dropping legacy aliases) and the entrypoint change (using `IFACE_*_MAC` directly) must ship together. Deploying one without the other will break the MAC rename. Container images must be rebuilt after the entrypoint change before `vcpe up` is run.
→ Mitigation: The example manifests are updated as part of this change to include explicit `device` fields, providing a clear signal that the deployment requires rebuilt images.

**[Risk] Existing manifests without device fields break gateway preflight** — After `ValidateInterfaces` is wired in, any gateway manifest without explicit `device` fields on all interfaces will fail `vcpe up`. The three example manifests in `manifests/` must be updated.
→ Mitigation: Updating the example manifests is an explicit task in this change. The error message from `ValidateInterfaces` identifies the missing field and the interface role.

**[Risk] WebPA/event-sink entrypoints hard-code `eth0`** — These services don't rename by MAC; they assume Podman assigns `eth0`. If a manifest sets `device: mgmt0` on a WebPA interface, the entrypoint won't rename and the wrong interface name will be used.
→ Mitigation: WebPA and event-sink are single-interface services; Podman consistently assigns `eth0`. Making them rename would require adding MAC-based rename logic to their entrypoints. This is deferred (out of scope). The planner still passes the `device` field through as `IFACE_MGMT_DEVICE=eth0` (the default), so when `device` is unset or explicitly `eth0`, it continues to work correctly.
