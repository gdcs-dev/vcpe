## Context

The `generic-container` renderer today produces two or three artifacts per service: `compose.env`, `compose.yaml`, and optionally a static `resolv.conf` when the service connects to a LAN network whose `PodmanDNS` is set. The static `resolv.conf` is bind-mounted over `/etc/resolv.conf:ro` to override Podman's aardvark-dns injection. Network setup — bringing interfaces up, running DHCP, flushing existing addresses — is the operator's responsibility and must be embedded in `config.command` as a shell one-liner.

This change adds a generated `entrypoint.sh` artifact that the container executes before the user's command. The entrypoint reads `VCPE_INIT_*` environment variables to perform ordered initialization, then `exec`s the command. The `resolv.conf` bind-mount approach is removed; DNS setup becomes the entrypoint's responsibility.

## Goals / Non-Goals

**Goals:**
- Provide a standardized init sequence (identity → network → dns → exec) for every generic-container service.
- Allow operators to express network-layer identity (MAC, hostname) and network mode (DHCP/static) declaratively via `config.env` rather than imperative shell in `config.command`.
- Expose `defaultRoute: true` interface flag to the container via `IFACE_<ROLE>_DEFAULT_ROUTE=1` so the entrypoint can auto-configure the default route without additional manifest config.
- Replace the static resolv.conf bind-mount with dynamic DNS configuration driven by DHCP response or `VCPE_INIT_NAMESERVER`.

**Non-Goals:**
- Supporting images without `/bin/sh` (Alpine and Debian/Ubuntu are the only target images).
- Per-replica identity differentiation (each replica gets the same env; use separate named services for distinct identities).
- Dynamic entrypoint customization beyond the `VCPE_INIT_*` protocol (no user-supplied hooks or scripts).
- Changing the init protocol for curated service types (bng, gateway, webpa, xb10) — those have their own entrypoints baked into images.

## Decisions

### D1: Entrypoint script embedded in the Go renderer, not a repo file

**Decision**: The entrypoint.sh content is a Go string constant (`entrypointSH`) in `genericcontainer.go`. The renderer emits it as a render artifact. It is never read from disk at render time.

**Rationale**: The renderer binary is self-contained; no runtime path dependency on a repo file. Versioned with the binary — guaranteed consistency between renderer logic and script behavior. The existing curated services bake their entrypoints into images at build time; generic-container achieves equivalent self-containment via the embedded constant.

**Alternative considered**: `services/generic-container/container/entrypoint.sh` read at render time. Rejected — introduces a fragile path dependency and makes the binary non-portable.

### D2: Always inject the entrypoint, no opt-in flag

**Decision**: Every generic-container render always includes `entrypoint.sh` and sets `entrypoint:` in compose. The script is a no-op when no `VCPE_INIT_*` vars are set — it finds no default-route interface and `exec`s `"$@"` immediately.

**Rationale**: Consistency — all generic-container services behave the same way. Avoids a per-service boolean that operators must remember to set. The no-op path is fast and transparent; existing manifests that embed network setup in `config.command` continue to work since the command is still exec'd.

**Alternative considered**: `config.initEntrypoint: true` opt-in flag. Rejected — unnecessary cognitive overhead and inconsistent behavior across services.

### D3: DHCP is default for the `defaultRoute: true` interface; `VCPE_INIT_STATIC_ROLE` opts out

**Decision**: The entrypoint finds the interface with `IFACE_<ROLE>_DEFAULT_ROUTE=1`, brings it up, and runs `udhcpc -f -i $device -x hostname:$(hostname) &` in the background followed by a wait for the lease. If `VCPE_INIT_STATIC_ROLE=<role>` is set for that role, it instead applies `IFACE_<ROLE>_IPV4` statically and adds a default route via `IFACE_<ROLE>_GATEWAY4`.

**Rationale**: The simulated RDK environment is DHCP-driven. Making DHCP the default eliminates the most common `config.command` boilerplate. Static opt-out handles future cases (e.g., infrastructure services that don't participate in DHCP).

### D4: `IFACE_<ROLE>_DEFAULT_ROUTE=1` added to `render.IfaceEnv()`

**Decision**: `IfaceEnv()` emits `IFACE_<ROLE>_DEFAULT_ROUTE=1` (or omits the var entirely when false) for each interface where `plan.Interface.DefaultRoute` is true.

**Rationale**: The `DefaultRoute` flag already exists in both the manifest and plan models and is validated (at most one per service). Exposing it in the env contract makes it discoverable by the entrypoint without encoding role names in Go. Omitting the variable (rather than emitting `=0`) keeps the env file clean for the common case where no interface has `defaultRoute: true`.

### D5: `resolv.conf` artifact and bind-mount removed entirely

**Decision**: The renderer no longer generates a `resolv.conf` artifact or adds `./resolv.conf:/etc/resolv.conf:ro` to volumes. DNS is configured by the entrypoint: `udhcpc` writes `/etc/resolv.conf` via its default script when the DHCP server includes option 6; `VCPE_INIT_NAMESERVER` provides an explicit fallback.

**Rationale**: Without the bind-mount, `/etc/resolv.conf` is writable inside the container, so both the DHCP script and the entrypoint's explicit write path work correctly. The static bind-mount approach was a workaround for the read-only constraint it imposed; removing it is cleaner.

### D6: Identity init precedes network init

**Decision**: The entrypoint applies MAC (`ip link set <dev> address`) and hostname (`hostname`) before running DHCP, so DHCP requests reflect the configured identity. The ordering is fixed: identity → network → dns (set by DHCP or explicit override) → exec.

**Rationale**: The RDK simulation requires that DHCP server logs and provisioning server interactions see the device's simulated identity, not the container's random Podman MAC or default hostname.

### D7: `VCPE_INIT_*` namespace; all other vars pass through

**Decision**: The entrypoint interprets only vars with the `VCPE_INIT_` prefix. Everything else in the environment is untouched and available to the exec'd command.

**Rationale**: Clean separation between init protocol and application config. Operators can freely add `DEVICE_SERIAL`, `TR069_ACS_URL`, or any other app-level var in `config.env` without risk of collision.

## Risks / Trade-offs

- **Images without `udhcpc`**: Debian-based images don't include `udhcpc` by default (they use `dhclient`). The entrypoint could check for `udhcpc` vs `dhclient`. Mitigation: document Alpine as the primary target; add `dhclient` support in a follow-up if needed.
- **DHCP settle time**: `udhcpc` may not have a lease before the command needs the network. Mitigation: `VCPE_INIT_SLEEP=<n>` provides an explicit settle delay; future work can use `udhcpc -s` with a custom script that signals readiness.
- **Breaking existing manifests with embedded network init in command**: If a manifest already runs `udhcpc` in `config.command`, the entrypoint will also attempt DHCP on the default-route interface — double DHCP. Mitigation: operators must remove the network setup from `config.command` when migrating; the change docs call this out.
- **Image ENTRYPOINT override**: Setting `entrypoint:` in compose unconditionally overrides the image's ENTRYPOINT. Images that rely on their own entrypoint must have it called explicitly from `config.command`. Mitigation: document this; Alpine's default entrypoint is nil and its CMD is `/bin/sh`, so the common case is unaffected.

## Migration Plan

Existing generic-container manifests that embed network setup in `config.command` (e.g., the `client` service in `manifests/dev/xb10.yaml`) should:
1. Move `ip addr flush` and `udhcpc` from `command` to `config.env` as `VCPE_INIT_DHCP_ROLE: <role>` (or rely on the `defaultRoute: true` auto-detection).
2. Set `VCPE_INIT_HOSTNAME` if a custom hostname was being set.
3. Update `command` to only contain the application workload.

No manifest schema changes are required. The old pattern still works mechanically (the entrypoint does DHCP, then the command does it again), but produces a double-DHCP race; migration removes the redundancy.
