## 1. Extend IfaceEnv contract

- [x] 1.1 In `controlplane/internal/render/env.go`: emit `IFACE_<ROLE>_DEFAULT_ROUTE=1` for each interface where `plan.Interface.DefaultRoute` is true; omit the variable entirely when false.
- [x] 1.2 Update `controlplane/internal/render/env_test.go` (or add a test) to assert `DEFAULT_ROUTE=1` appears for a default-route interface and is absent for non-default-route interfaces.

## 2. Implement the entrypoint script

- [x] 2.1 In `controlplane/internal/types/genericcontainer/genericcontainer.go`: add the `entrypointSH` string constant containing the complete `entrypoint.sh` script.
- [x] 2.2 Script identity block: read `VCPE_INIT_HOSTNAME`; if set, call `hostname` and store for DHCP. Read `VCPE_INIT_MAC_ROLE`; if set, resolve `IFACE_<ROLE>_DEVICE` and run `ip link set <dev> down && ip link set <dev> address <mac> && ip link set <dev> up`.
- [x] 2.3 Script network block: iterate env vars to find `IFACE_<ROLE>_DEFAULT_ROUTE=1`; bring up the device. If `VCPE_INIT_STATIC_ROLE` matches the role, apply `IFACE_<ROLE>_IPV4` with `ip addr add` and set default route via `IFACE_<ROLE>_GATEWAY4`. Otherwise run `udhcpc -f -i <dev> -x hostname:<hostname>` in the background and wait for the lease file.
- [x] 2.4 Script DNS block: if `VCPE_INIT_NAMESERVER` is set, write `nameserver $VCPE_INIT_NAMESERVER` to `/etc/resolv.conf` (overrides any DHCP-provided value).
- [x] 2.5 Script timing block: if `VCPE_INIT_SLEEP` is set, `sleep $VCPE_INIT_SLEEP`.
- [x] 2.6 Script exec: `exec "$@"`.

## 3. Wire entrypoint into the renderer

- [x] 3.1 In `renderer.Render()`: add `entrypoint.sh` to the artifacts list (content: `entrypointSH`).
- [x] 3.2 In `generateCompose()` / `buildSvcEntry()`: set `entrypoint: ["/bin/sh", "/run/vcpe/entrypoint.sh"]` on each compose service entry.
- [x] 3.3 Add `./entrypoint.sh:/run/vcpe/entrypoint.sh:ro` to the volumes list in `buildSvcEntry()`.
- [x] 3.4 Remove the `resolv.conf` artifact generation block (the `if len(lanDNS) > 0` artifact append).
- [x] 3.5 Remove the `./resolv.conf:/etc/resolv.conf:ro` volume append from `buildSvcEntry()`.
- [x] 3.6 Remove the `lanDNS` collection logic from `renderer.Render()` (the `roleDNS`/`dnsSet` block) — it is no longer needed.

## 4. Update golden tests

- [x] 4.1 Regenerate or update the golden fixtures in `controlplane/internal/types/genericcontainer/genericcontainer_golden_test.go` to reflect: presence of `entrypoint.sh` artifact, `entrypoint:` key in compose, `./entrypoint.sh:/run/vcpe/entrypoint.sh:ro` in volumes, absence of `resolv.conf` artifact, absence of `./resolv.conf:/etc/resolv.conf:ro` in volumes.
- [x] 4.2 Add a golden case that exercises `VCPE_INIT_STATIC_ROLE` to verify static-assignment path is not broken by the DHCP-default logic.
- [x] 4.3 Add a test that verifies `VCPE_INIT_HOSTNAME`, `VCPE_INIT_MAC_ROLE`, and `VCPE_INIT_SLEEP` env vars in `config.env` pass through into `compose.env` unaltered (they are not consumed at render time).

## 5. Migrate example manifests

- [x] 5.1 In `manifests/dev/xb10.yaml`: update the `client` service — remove the embedded `ip addr flush && udhcpc` from `config.command`, set `config.command` to the actual workload (`["/bin/sh"]` or replace with a real workload command), and add `config.env.VCPE_INIT_DHCP_ROLE: lan-p1` if `defaultRoute: true` is not already set on that interface (or rely on auto-detection if it is).
- [x] 5.2 Verify all other manifests under `manifests/` that use `generic-container` — confirm none embed network setup in `config.command` that would conflict with the entrypoint's DHCP pass.

## 6. Verify

- [x] 6.1 Run `cd controlplane && go test ./internal/render/... ./internal/types/genericcontainer/...` — all tests pass.
- [x] 6.2 Run `cd controlplane && go build ./...` — no compilation errors.
