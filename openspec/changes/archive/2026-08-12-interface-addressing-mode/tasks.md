## 1. Schema and validation

- [x] 1.1 Add `Addressing string` field to `manifest.Interface` in `controlplane/internal/manifest/model.go` (json/yaml tag `addressing,omitempty`), with a doc comment describing the `dhcp`/`static` enum and the `dhcp` default.
- [x] 1.2 In `controlplane/internal/manifest/validate.go`'s `validateServiceInterfaces`, add: normalize empty `Addressing` to `"dhcp"`; reject any value other than `"dhcp"`/`"static"`; reject `static` with no `ipv4`/`ipv6`; reject `dhcp` (explicit or defaulted) with `ipv4` or `ipv6` set — except when the interface also sets `Bridge != ""`, in which case skip this check entirely.
- [x] 1.3 Add unit tests in `controlplane/internal/manifest/validate_test.go` (or equivalent) covering: default-to-dhcp, static-without-address rejected, dhcp-with-address rejected, bridge-enslaved interface exempt, and no BNG-specific guard (a `bng`-serving-DHCP role set to `dhcp` passes validation).

## 2. Plan and render plumbing

- [x] 2.1 Add `Addressing string` to `plan.Interface` in `controlplane/internal/plan/model.go`.
- [x] 2.2 Thread `iface.Addressing` through `resolveInstance` in `controlplane/internal/planner/planner.go` into the resolved `plan.Interface` (passthrough, same normalization as validation).
- [x] 2.3 Add `IFACE_<ROLE>_ADDRESSING` emission to `render.IfaceEnv()` in `controlplane/internal/render/env.go`, alongside the existing `IPV4`/`IPV6`/`GATEWAY4`/`GATEWAY6` vars, for every interface (including bridge-enslaved ones).
- [x] 2.4 Update/add golden or unit tests asserting `IFACE_<ROLE>_ADDRESSING` appears with the correct value in rendered env output (e.g. extend `controlplane/internal/render` tests if present, and the per-type golden tests touched in sections 3-6).

## 3. Generic-container migration (supersede VCPE_INIT_STATIC_ROLE)

- [x] 3.1 In `controlplane/internal/types/genericcontainer/genericcontainer.go`'s embedded entrypoint script, replace the single default-route-DHCP block with a loop over every `IFACE_<ROLE>_DEVICE` env var: for each role, branch on `IFACE_<ROLE>_ADDRESSING` (`dhcp` → bring device up + `udhcpc`, honoring `VCPE_INIT_MAC_ROLE`/`VCPE_INIT_HOSTNAME` when they name that role; `static` → `ip addr add $IFACE_<ROLE>_IPV4` + optional default route via `IFACE_<ROLE>_GATEWAY4` when `IFACE_<ROLE>_DEFAULT_ROUTE=1`).
- [x] 3.2 Remove `VCPE_INIT_STATIC_ROLE` handling entirely from the script.
- [x] 3.3 Update `controlplane/internal/types/genericcontainer/genericcontainer_golden_test.go` to cover: multiple interfaces each initialized, dhcp-by-default, explicit static role, and removal of the old `VCPE_INIT_STATIC_ROLE` scenario.
- [x] 3.4 Update `openspec/specs/generic-container-init-entrypoint/spec.md` main spec file is NOT edited directly (delta already written under this change) — verify the delta in `openspec/changes/interface-addressing-mode/specs/generic-container-init-entrypoint/spec.md` matches the implemented behavior once code lands.

## 4. Gateway: add real DHCP client

- [x] 4.1 Add a DHCP client package to `services/gateway/Containerfile` (ubuntu 24.04 base — evaluate `isc-dhcp-client` vs `udhcpc`/busybox per the custom apt repo already in use).
- [x] 4.2 In `services/gateway/container/entrypoint.sh`'s `configure_networking`, branch WAN (`erouter0`) and CM (`wan0`) handling on `IFACE_WAN_ADDRESSING`/`IFACE_CM_ADDRESSING`: `static` keeps the existing `ip addr add $EROUTER0_IPV4`/CM equivalent path; `dhcp` runs the chosen DHCP client on the device instead of applying `EROUTER0_IPV4`/CM address.
- [x] 4.3 Update `controlplane/internal/types/gateway/gateway.go` if any config/env derivation needs to skip emitting `EROUTER0_IPV4`/gateway vars when the role resolves to `dhcp` (avoid emitting stale/empty static vars the entrypoint might otherwise misread).
- [x] 4.4 Update `controlplane/internal/types/gateway/gateway_golden_test.go` to cover both `addressing: static` (existing behavior, now explicit) and `addressing: dhcp` (new default) for `wan`/`cm` roles, including the `healthUpstream` + `dhcp` combination.

## 5. webpa / event-sink: gate existing DHCP client

- [x] 5.1 In `services/webpa/container/entrypoint.sh`, gate the existing `dhclient` call on `IFACE_MGMT_ADDRESSING=dhcp` (skip it entirely when `static`, relying on the renderer's `ipv4_address` compose pin).
- [x] 5.2 In `services/event-sink/container/entrypoint.sh`, apply the same gating to its `dhclient -v eth0` call.
- [x] 5.3 Update `controlplane/internal/types/webpa/webpa_golden_test.go` to cover `addressing: static` with a pinned `ipv4_address` and `addressing: dhcp` (default) with no pin.
- [x] 5.4 Add/update a golden or unit test for `controlplane/internal/types/eventsink/eventsink.go` covering the same static/dhcp distinction (create a test file if none exists).

## 6. Oktopus: add DHCP client

- [x] 6.1 Add a DHCP client package to `services/oktopus/Containerfile` (ubuntu 22.04 base).
- [x] 6.2 Add/update `services/oktopus`'s entrypoint (or confirm it has none and needs one added) to run a DHCP client on mgmt when `IFACE_MGMT_ADDRESSING=dhcp`, matching the webpa/event-sink pattern.
- [x] 6.3 Add/update a test for `controlplane/internal/types/oktopus/oktopus.go` covering the static/dhcp distinction.

## 7. BNG

- [x] 7.1 Confirm `bng`'s renderer/entrypoint requires no code change beyond accepting the new schema field (its interfaces are expected to stay `addressing: static` by convention; no validation guard is added per design.md).
- [x] 7.2 Add a golden test case (or a comment in an existing test) documenting that `bng` with `addressing: dhcp` on a role it also serves DHCP on passes validation and is not blocked (matches the "no guard" requirement).

## 8. Manifest updates

- [x] 8.1 Update `manifests/example.yaml`, `manifests/example-full.yaml`, `manifests/xb10.yaml`, `manifests/dev/example.yaml`, `manifests/dev/example-full.yaml`, `manifests/dev/example-macvlan.yaml`, and `manifests/dev/xb10.yaml`: add `addressing: static` to every `gateway`/`webpa`/`event-sink`/`bng` interface that currently sets an explicit `ipv4`.
- [x] 8.2 Confirm `manifests/dev/xb10.yaml`'s `client` service (generic-container, `defaultRoute: true`, no explicit `config.env.VCPE_INIT_*` role vars) continues to work unchanged under the new default-dhcp behavior; add `addressing` explicitly if useful for clarity.
- [x] 8.3 Grep the full manifest set for any remaining `VCPE_INIT_STATIC_ROLE`/`VCPE_INIT_DHCP_ROLE` usage and migrate to `addressing` on the corresponding `interfaces[]` entry.

## 9. Cross-cutting verification

- [x] 9.1 Run `cd controlplane && go build ./...` and `go test ./...` (expected to still fail overall per the repo's mid-rewrite state — confirm no *new* failures are introduced by this change beyond pre-existing ones).
- [x] 9.2 Run affected smoke scripts under `tests/smoke/` for `gateway`, `bng`, `webpa`, `event-sink`, `oktopus` if runnable in the current environment.
- [x] 9.3 Restage `runtime-init-*` binaries via `scripts/stage-runtime-init-binaries` only if any `runtimeinit/contract` package changed (not expected per design.md — confirm during implementation).

## 10. Managed-network exclusion (post-implementation refinement)

- [x] 10.1 Add `ManagedNetwork bool` to `plan.Interface` (`controlplane/internal/plan/model.go`), computed in `planner.resolveInstance` from `net.IPAMDriver != "none"`.
- [x] 10.2 Emit `IFACE_<ROLE>_NETWORK_MANAGED=1` from `render.IfaceEnv()` when `ManagedNetwork` is true, omitted otherwise.
- [x] 10.3 Guard every DHCP-client invocation (`gateway` WAN/CM, `generic-container`, `webpa`, `event-sink`, `oktopus`) on `IFACE_<ROLE>_NETWORK_MANAGED` being unset, so `addressing: dhcp` is a no-op on Podman-managed networks (e.g. `mgmt`) instead of running a redundant/conflicting DHCP client.
- [x] 10.4 Add test coverage: `planner_test.go` (`ManagedNetwork` resolution from `IPAMDriver`), `render/env_test.go` (`NETWORK_MANAGED` emission), and updated `webpa`/`event-sink`/`oktopus` golden tests asserting the var appears for a Podman-managed mgmt fixture.
- [x] 10.5 Update `design.md` and the `interface-addressing-mode`/`rendering-and-secrets-contract`/`generic-container-init-entrypoint` spec deltas to document the managed-network exclusion.
