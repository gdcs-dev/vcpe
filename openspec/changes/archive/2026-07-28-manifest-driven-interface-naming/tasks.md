## 1. Go — ServiceType interface: ValidateInterfaces

- [x] 1.1 Add `ValidateInterfaces(interfaces []manifest.Interface) error` to the `ServiceType` interface in `controlplane/internal/typeregistry/registry.go`
- [x] 1.2 Implement `ValidateInterfaces` returning nil in `controlplane/internal/types/bng/bng.go`
- [x] 1.3 Implement `ValidateInterfaces` returning nil in `controlplane/internal/types/webpa/webpa.go`
- [x] 1.4 Implement `ValidateInterfaces` returning nil in `controlplane/internal/types/eventsink/eventsink.go`
- [x] 1.5 Implement `ValidateInterfaces` returning nil in `controlplane/internal/types/genericcontainer/genericcontainer.go`
- [x] 1.6 Implement `ValidateInterfaces` returning nil in `controlplane/internal/types/oktopus/oktopus.go`
- [x] 1.7 Implement `ValidateInterfaces` returning nil in `controlplane/internal/types/xb10/xb10.go`
- [x] 1.8 Implement `ValidateInterfaces` in `controlplane/internal/types/gateway/gateway.go` — return an error for any interface with an empty `device` field, naming the interface role
- [x] 1.9 Call `st.ValidateInterfaces(svc.Interfaces)` in `controlplane/internal/app/preflight.go` after the existing `assertExpectedRoles` call; fail with the returned error
- [x] 1.10 Update `controlplane/internal/typeregistry/registry_test.go` to verify `ValidateInterfaces` is non-panicking for all registered types

## 2. Go — Gateway: LAN bridge config and synthesized env vars

- [x] 2.1 Add `Bridge string` field to `LANConfig` in `controlplane/internal/types/gateway/gateway.go` with `yaml:"bridge,omitempty"` tag
- [x] 2.2 In the gateway renderer `Render()`, emit `LAN_BRIDGE=<cfg.LAN.Bridge>` (defaulting to `brlan0` when empty)
- [x] 2.3 In the gateway renderer `Render()`, emit `LAN_DEVICES=<space-delimited device names>` for all interfaces whose role matches `lanPrefix`; derive device names from `ifaceByRole[role].Device`
- [x] 2.4 Remove `LAN1_MAC=`/`LAN2_MAC=`/`LAN3_MAC=`/`LAN4_MAC=` emission from the gateway renderer
- [x] 2.5 Remove `WAN0_MAC=` emission from the gateway renderer (cm role alias)
- [x] 2.6 Remove `EROUTER0_MAC=` emission from the gateway renderer (wan role alias)
- [x] 2.7 Update `controlplane/internal/types/gateway/gateway_golden_test.go` to reflect removed legacy aliases and new `LAN_BRIDGE` / `LAN_DEVICES` env vars

## 3. Go — BNG renderer: remove legacy MAC aliases

- [x] 3.1 Remove the `for _, iface := range inst.Interfaces { ... key + "_MAC" ... }` legacy alias loop from `controlplane/internal/types/bng/bng.go` `Render()` (lines that emit `MGMT_MAC=`, `WAN_MAC=`, `CM_MAC=`)
- [x] 3.2 Update `controlplane/internal/types/bng/bng_golden_test.go` to remove `MGMT_MAC=`, `WAN_MAC=`, `CM_MAC=` from expected env output

## 4. Go — XB10 renderer: remove legacy MAC aliases (if present)

- [x] 4.1 Inspect `controlplane/internal/types/xb10/xb10.go` renderer for legacy `LAN*_MAC=`/`EROUTER0_MAC=`/`WAN0_MAC=` emission and remove it
- [x] 4.2 Update `controlplane/internal/types/xb10/xb10_golden_test.go` if affected

## 5. Go — Build and test

- [x] 5.1 Run `cd controlplane && go build ./...` — confirm no compile errors
- [x] 5.2 Run `cd controlplane && go test ./...` — confirm all tests pass including updated golden tests

## 6. Container: BNG entrypoint — generic rename loop

- [x] 6.1 Rewrite `rename_interfaces_by_mac()` in `services/bng/container/entrypoint.sh` to build `target_by_mac` by iterating `IFACE_*_MAC` + `IFACE_*_DEVICE` env var pairs using `compgen -v`
- [x] 6.2 Remove all references to `MGMT_MAC`, `WAN_MAC`, `CM_MAC` from `services/bng/container/entrypoint.sh`
- [x] 6.3 Verify `rename_interfaces_by_order()` fallback still works (uses `IFACE_*_DEVICE` order) or update to use the same generic pattern

## 7. Container: Gateway entrypoint — generic rename + manifest-driven configure_networking

- [x] 7.1 Rewrite `rename_interfaces_by_mac()` in `services/gateway/container/entrypoint.sh` to build `target_by_mac` from `IFACE_*_MAC` + `IFACE_*_DEVICE` env var pairs
- [x] 7.2 Remove the hard-coded `LAN{1..4}_MAC`, `WAN0_MAC`, `EROUTER0_MAC` variable references from `rename_interfaces_by_mac()`
- [x] 7.3 Update `configure_networking()` to read `WAN_DEV` from `${IFACE_WAN_DEVICE}` (replacing hard-coded `erouter0`)
- [x] 7.4 Update `configure_networking()` to read `CM_DEV` from `${IFACE_CM_DEVICE}` (replacing hard-coded `wan0`)
- [x] 7.5 Update `configure_networking()` to iterate `${LAN_DEVICES}` (replacing hard-coded `eth0 eth1 eth2 eth3`)
- [x] 7.6 Update `configure_networking()` to use `${LAN_BRIDGE:-brlan0}` as the bridge name (replacing hard-coded `brlan0`)
- [x] 7.7 Update `main()` iptables MASQUERADE rule to use `${IFACE_WAN_DEVICE}` (replacing hard-coded `erouter0`)
- [x] 7.8 Determine which role maps to `wanRole` / `cmRole` for the variable lookups — use the same defaults ("wan" and "cm") the renderer uses; the entrypoint reads `IFACE_WAN_DEVICE` and `IFACE_CM_DEVICE` by convention

## 8. Container: XB10 entrypoint — generic rename loop

- [x] 8.1 Rewrite `rename_interfaces_by_mac()` in `services/xb10/container/entrypoint.sh` to build `target_by_mac` from `IFACE_*_MAC` + `IFACE_*_DEVICE` env var pairs
- [x] 8.2 Remove all references to `LAN1_MAC`, `LAN2_MAC`, `LAN3_MAC`, `LAN4_MAC`, `EROUTER0_MAC`, `WAN0_MAC` from the xb10 entrypoint

## 9. Example manifests: add device fields

- [x] 9.1 Add `device` fields to all gateway interfaces in `manifests/example.yaml`: wan→erouter0, cm→wan0, lan-p1→eth0, lan-p2→eth1, lan-p3→eth2, lan-p4→eth3
- [x] 9.2 Add `device` fields to all bng interfaces in `manifests/example.yaml`: mgmt→eth0, wan→eth1, cm→eth2
- [x] 9.3 Add `device` fields to all interfaces in `manifests/example-macvlan.yaml` (same mapping as example.yaml)
- [x] 9.4 Add `device` fields to gateway, bng, and xb10 interfaces in `manifests/xb10.yaml`

## 10. Visual editor: device field editable per interface

- [x] 10.1 In `extensions/vcpe-visual-editor/webview/src/panels/PropertyPanel.tsx`, expand the interface list in `ServiceForm` to show a `device` field per interface row as an editable `Field` component
- [x] 10.2 Wire the `device` field `onCommit` to `applyMutation(rawYaml, { kind: 'setScalar', path: ['spec','services',svcIdx,'interfaces',ifaceIdx,'device'], value })` and call `onMutation`
- [x] 10.3 Rebuild and reinstall the extension: `make install-extension`

## 11. Verification

- [x] 11.1 Run `cd controlplane && go test ./...` — all tests pass
- [x] 11.2 Run `make install-extension` — extension builds clean
- [x] 11.3 Open `manifests/example.yaml` in the visual editor — gateway interfaces show device names (erouter0, wan0, eth0..3) as editable chips
- [x] 11.4 Edit a gateway interface device name in the property panel — YAML updates with new device value
- [x] 11.5 Run `vcpe plan --manifest manifests/example.yaml` — plan succeeds; `IFACE_WAN_DEVICE=erouter0` appears in rendered env; no `EROUTER0_MAC=` in output
- [x] 11.6 Run `vcpe plan --manifest manifests/example.yaml` with a gateway interface missing `device` — plan fails with preflight error identifying the missing device name
