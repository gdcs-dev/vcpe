## Why

Container interface renaming in BNG, Gateway, and XB10 is driven by hard-coded MAC alias tables (`MGMT_MAC`, `EROUTER0_MAC`, `LAN1_MAC`, etc.) that map to fixed target names (`eth0`, `erouter0`, `wan0`). Changing an interface name requires modifying entrypoint scripts and rebuilding images rather than editing the manifest. This also makes the visual editor inconsistent — moving a connection updates the network role but the container-internal interface name is invisible to the manifest.

## What Changes

- **NEW**: `ServiceType` interface gains `ValidateInterfaces([]manifest.Interface) error` so types can enforce per-interface constraints (e.g., gateway requires `device` on every interface)
- **NEW**: Gateway `Config.LAN.Bridge` field (defaults to `brlan0`) makes the LAN bridge name manifest-driven
- **NEW**: Gateway renderer emits `LAN_BRIDGE` and `LAN_DEVICES` synthesized env vars derived from manifest interface data
- **BREAKING**: All complex service entrypoints (BNG, Gateway, XB10) replace hard-coded MAC alias rename tables with a generic loop over `IFACE_*_MAC` + `IFACE_*_DEVICE` env vars
- **BREAKING**: BNG renderer drops `MGMT_MAC=`/`WAN_MAC=`/`CM_MAC=` legacy aliases; Gateway renderer drops `EROUTER0_MAC=`/`WAN0_MAC=`/`LAN{1..4}_MAC=` legacy aliases — entrypoints now use `IFACE_*_MAC` directly
- **BREAKING**: Gateway `ValidateInterfaces` rejects any gateway service whose interfaces do not all have `device` set
- **BREAKING**: Existing manifests (example.yaml, example-macvlan.yaml, xb10.yaml) must be updated to add explicit `device` fields to gateway, bng, and xb10 interface declarations
- **NEW**: Visual editor property panel makes `device` field editable per interface row

## Capabilities

### New Capabilities

- `manifest-driven-interface-names`: End-to-end contract — `Interface.device` in the manifest flows through planner → renderer → `IFACE_*_DEVICE` env var → container entrypoint generic rename loop; all hard-coded MAC alias tables are eliminated
- `lan-bridge-config`: `Config.LAN.Bridge` field on the gateway service type; renderer emits `LAN_BRIDGE` and `LAN_DEVICES` synthesized env vars; entrypoint reads them instead of hard-coded `brlan0`/`eth0..3`
- `interface-type-validation`: `ServiceType` interface gains `ValidateInterfaces` method; gateway enforces all interfaces have `device` set; other types return nil

### Modified Capabilities

- `service-type-registry`: `ServiceType` interface adds `ValidateInterfaces([]manifest.Interface) error`
- `rendering-and-secrets-contract`: `IFACE_*_MAC` and `IFACE_*_DEVICE` become the canonical env contract for interface identity; renderer-emitted legacy role-specific MAC aliases (`MGMT_MAC`, `WAN0_MAC`, etc.) are removed

## Impact

**Go / controlplane**
- `controlplane/internal/typeregistry/registry.go` — add `ValidateInterfaces` to `ServiceType` interface
- `controlplane/internal/types/bng/bng.go` — implement `ValidateInterfaces` (nil); remove `MGMT_MAC`/`WAN_MAC`/`CM_MAC` emission
- `controlplane/internal/types/gateway/gateway.go` — implement `ValidateInterfaces` (require device on all); add `Config.LAN.Bridge`; emit `LAN_BRIDGE`, `LAN_DEVICES`, drop legacy MAC aliases
- `controlplane/internal/types/xb10/xb10.go` — implement `ValidateInterfaces` (nil); remove legacy MAC alias emission
- All 7 `ServiceType` implementations — add `ValidateInterfaces` method
- `controlplane/internal/app/preflight.go` — call `ValidateInterfaces` for each service

**Container entrypoints**
- `services/bng/container/entrypoint.sh` — generic rename loop; drop `MGMT_MAC`/`WAN_MAC`/`CM_MAC` references
- `services/gateway/container/entrypoint.sh` — generic rename loop; `configure_networking()` reads `IFACE_*_DEVICE`, `LAN_BRIDGE`, `LAN_DEVICES`
- `services/xb10/container/entrypoint.sh` — generic rename loop; drop legacy `LAN1_MAC..4_MAC`/`EROUTER0_MAC`/`WAN0_MAC` references

**Runtime-init binaries** — restaging required after entrypoint changes

**Example manifests**
- `manifests/example.yaml`, `manifests/example-macvlan.yaml`, `manifests/xb10.yaml` — add `device` fields to gateway, bng, xb10 interface declarations

**Visual editor** (`extensions/vcpe-visual-editor/`)
- `webview/src/panels/PropertyPanel.tsx` — add editable `device` field per interface row in `ServiceForm`
- `webview/src/yaml/serialize.ts` — `setScalar` path for `interfaces[i].device`
