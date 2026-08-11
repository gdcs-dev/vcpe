## Why

`generic-container` services have no standardized way to perform network-layer initialization — operators must embed `ip` and `udhcpc` incantations directly in `config.command`, and device identity (MAC, hostname) cannot be applied before the workload starts. In a simulated RDK gateway network with extenders, phones, and other CPE types, each client container needs to acquire an address via DHCP and present a distinct network-layer identity before its workload runs.

## What Changes

- The `generic-container` renderer always emits an `entrypoint.sh` artifact alongside `compose.yaml`, and sets `entrypoint:` in the generated compose to execute it.
- The entrypoint is a shell script embedded in the renderer; it reads `VCPE_INIT_*` environment variables from `compose.env`/`config.env` to perform ordered initialization: identity → network → exec.
- `VCPE_INIT_*` defines an extensible init protocol: `VCPE_INIT_MAC_ROLE`, `VCPE_INIT_HOSTNAME`, `VCPE_INIT_DHCP_ROLE`, `VCPE_INIT_STATIC_ROLE`, `VCPE_INIT_NAMESERVER`, `VCPE_INIT_SLEEP`.
- DHCP is the default for any interface declaring `defaultRoute: true`; `VCPE_INIT_STATIC_ROLE` opts out to static assignment from `IFACE_<ROLE>_IPV4`/`GATEWAY4`.
- `render.IfaceEnv()` is extended to emit `IFACE_<ROLE>_DEFAULT_ROUTE=1` for interfaces where `DefaultRoute` is true, making the default-route role discoverable by the entrypoint without extra manifest config.
- **BREAKING (generic-container only)**: The static `resolv.conf` artifact and its bind-mount volume are removed; DNS configuration is now the entrypoint's responsibility (written by DHCP response or `VCPE_INIT_NAMESERVER`).
- Non-`VCPE_INIT_*` vars in `config.env` (e.g., `DEVICE_SERIAL`, `TR069_ACS_URL`) are untouched by the entrypoint and pass through to the workload.

## Capabilities

### New Capabilities
- `generic-container-init-entrypoint`: The `VCPE_INIT_*` init protocol, the generated `entrypoint.sh` artifact, the entrypoint's initialization sequence (identity → network → dns → exec), and the embed/mount mechanism in the generated compose.

### Modified Capabilities
- `rendering-and-secrets-contract`: The generic container renderer's artifact set changes (adds `entrypoint.sh`, removes `resolv.conf`), and the canonical `IFACE_*` contract gains `IFACE_<ROLE>_DEFAULT_ROUTE`.

## Impact

- `controlplane/internal/render/env.go`: emit `IFACE_<ROLE>_DEFAULT_ROUTE=1` when `plan.Interface.DefaultRoute` is true.
- `controlplane/internal/types/genericcontainer/genericcontainer.go`: embed `entrypointSH` constant; emit it as a render artifact; set `entrypoint:` and the bind-mount volume in the generated compose; remove `resolv.conf` artifact and its volume.
- `controlplane/internal/types/genericcontainer/genericcontainer_golden_test.go`: update golden fixtures to reflect new artifacts and compose shape.
- No manifest schema changes. No changes to curated service types (bng, gateway, webpa, xb10, etc.).
