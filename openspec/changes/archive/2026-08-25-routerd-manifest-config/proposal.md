## Why

`routerd` is currently deployed as a bare `type: generic-container` with a single `wan` interface and no `config:` block. Its Containerfile ships its own bespoke `render-config.sh`/`entrypoint.sh`, cloned from GATEWAY/XB10, which scrapes `BRLAN0_*`/`WAN0_*`/`EROUTER0_*`/`LAN*_MAC` env vars that are never produced for routerd's actual manifest, and emits a `{"version":"1","controllers":{...}}` JSON blob that does not match the shape the vendored `routerd`/`routerctl` binary accepts (`routerctl apply` parses a `ConfigDocument` — `{"resources": [...]}` of `{apiVersion, kind, metadata, spec}` resources for kinds `Interface`/`Bridge`/`IpAddress`/`Route`/`DhcpClient`/`WanPolicy`). There is today no working path from an arbitrary manifest to a config routerd can actually apply. This change makes routerd a first-class, manifest-driven service type so its LAN bridge topology and WAN addressing are declared once in the manifest and compiled into routerd's native resource-kind config, matching the pattern already established for `bng`.

## What Changes

- Add a new `routerd` service type (`internal/types/routerd`) registered in the type registry, replacing routerd's current `type: generic-container` declaration. **BREAKING**: existing manifests declaring `type: generic-container` for a routerd workload must change to `type: routerd`.
- Add a typed `config:` schema for the `routerd` type that declares LAN bridge topology: one or more named bridges, each listing the interface roles enslaved to it (mirroring the gateway `lan-bridge-config` pattern but scoped to routerd's own resource model). *(Superseded during implementation — see the final mechanism below.)*
- Reuse the existing per-interface `addressing: dhcp|static` field (see `interface-addressing-mode`) as the source of routerd's WAN addressing intent — no new addressing schema.
- Add a Go renderer that compiles the resolved deployment (interfaces, bridge config, addressing) into a routerd `ConfigDocument`: `Interface`/`Bridge` resources for L2 topology, and `WanPolicy` (+ derived `DhcpClient` or `IpAddress`/`Route`) resources for each addressed interface, emitted as a render artifact (e.g. `etc/routerd/config.json`).
- Replace routerd's runtime `render-config.sh` (GATEWAY-shaped env-var scraping producing a stale JSON format) with consumption of the pre-rendered config file: the entrypoint applies it via `routerctl apply $ROUTERD_CONFIG` and no longer assembles JSON at container start.
- Replace routerd's hardcoded `rename_interfaces_by_mac` (`LAN1_MAC`/`WAN0_MAC`/`EROUTER0_MAC` table) with the generic `IFACE_<ROLE>_MAC`/`IFACE_<ROLE>_DEVICE` renaming mechanism already used by BNG/Gateway/XB10.
- Remove `services/routerd/container/render-config.sh` and the GATEWAY-derived parts of `services/routerd/container/entrypoint.sh`; update `services/routerd/Containerfile` accordingly.
- Update `manifests/dev/routerd.yaml` to use `type: routerd` and declare its LAN bridge topology via the standard `interfaces[].bridge` + `services[].bridges[]` fields (the same mechanism `gateway`/`bng` already use — see design.md decision 2 for why the routerd-specific `config.bridges` schema above was dropped in favor of this for manifest consistency).

## Capabilities

### New Capabilities
- `routerd-manifest-config`: typed manifest schema for routerd's LAN bridge topology, the renderer that compiles resolved interfaces/addressing into a routerd-native `ConfigDocument` (Interface/Bridge/WanPolicy/DhcpClient/IpAddress/Route resources), and the container startup contract that applies the pre-rendered config via `routerctl apply` instead of assembling it at runtime.

### Modified Capabilities
- `service-type-registry`: register the new `routerd` type (validator, renderer, expected host-network roles, health behavior, default image), analogous to the existing `bng`/`gateway` entries.
- `interface-addressing-mode`: extend the per-type runtime contract so that for `routerd`-typed services, each interface's resolved `addressing` value is honored by translating it into `WanPolicy`(+`DhcpClient`/`IpAddress`/`Route`) resources applied via `routerctl`, rather than a shell-level `udhcpc`/`ip addr add`.
- `manifest-driven-interface-names`: extend the interface-renaming contract to cover routerd's entrypoint, replacing its hardcoded `LAN1_MAC`/`WAN0_MAC`/`EROUTER0_MAC` rename table with the generic `IFACE_*_MAC`/`IFACE_*_DEVICE` mechanism already required for BNG/Gateway/XB10.

## Impact

- **Code**: `controlplane/internal/types/routerd/` (new), `controlplane/internal/typeregistry` (registration), `controlplane/internal/types/types.go` (register call).
- **Manifests**: `manifests/dev/routerd.yaml` (`type: generic-container` → `type: routerd`, LAN topology expressed via `interfaces[].bridge` + `services[].bridges[]`).
- **Containers**: `services/routerd/Containerfile`, `services/routerd/container/entrypoint.sh` (rewritten), `services/routerd/container/render-config.sh` (removed).
- **Dependencies**: none new; continues to rely on the vendored `routerd`/`routerctl` binaries already installed via the custom apt repo.
- **Breaking**: manifests using `type: generic-container` for a routerd-shaped workload must migrate to `type: routerd`; the container-internal config path/format changes from the ad hoc `controllers.*` JSON to a `ConfigDocument` resource list.
