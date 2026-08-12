## Why

Interface IP assignment is currently static-only and inconsistent across service types: `gateway`'s WAN/CM interfaces always apply whatever `ipv4` the manifest sets (or are left unconfigured if it's omitted — there is no DHCP path at all), `event-sink`/`webpa` unconditionally run `dhclient` on mgmt regardless of whether the manifest pins a static `ipv4` (silently clobbering it), and `generic-container` has its own one-off DHCP/static convention driven entirely by `config.env` (`VCPE_INIT_DHCP_ROLE`/`VCPE_INIT_STATIC_ROLE`) that only ever configures a single interface — the one marked `defaultRoute: true`. Operators have no consistent, manifest-visible way to say "this interface gets a static IP" vs. "this interface should DHCP," and BNG already runs real DHCP servers on both WAN/CM (`ipamDriver: none` segments) and mgmt (a Podman-managed network) that nothing but `generic-container` and `event-sink`/`webpa` currently leverage.

## What Changes

- Add a first-class `addressing: dhcp | static` field to `spec.services[].interfaces[]`, defaulting to `dhcp` when omitted.
- `addressing: static` requires an explicit `ipv4` and/or `ipv6` on the same interface; `addressing: dhcp` (or omitted) forbids setting `ipv4`/`ipv6` on that interface. Both are validation errors.
- `addressing` is ignored entirely on bridge-enslaved interfaces (those with `bridge:` set) — no validation, no runtime effect.
- No cross-check against a service's own DHCP-server role (e.g. `bng`'s `config.access[]`); a manifest that sets `dhcp` on BNG's own server-side interface is not blocked and will simply fail at runtime.
- **BREAKING**: `generic-container`'s existing `VCPE_INIT_DHCP_ROLE`/`VCPE_INIT_STATIC_ROLE`/`defaultRoute`-implied-DHCP convention is superseded by the new per-interface `addressing` field. The entrypoint now configures every declared interface (not only the default-route one) according to its own `addressing` value.
- `gateway`'s container image and entrypoint gain a real DHCP client (none exists today) and per-role static/DHCP branching keyed on `addressing`.
- `event-sink`'s and `webpa`'s existing unconditional `dhclient` call on mgmt becomes conditional on `addressing` (fixes the latent bug where an explicit static `ipv4` is currently overwritten by DHCP).
- `oktopus` gains a DHCP client in its image so `addressing: dhcp` (the default) works on its mgmt interface.
- `bng` is unaffected beyond gaining the schema field; its own interfaces are expected to stay `static` by convention, unenforced.
- Out of scope: `xb10` is explicitly excluded from this change; its existing (stubbed) DHCP handling is left untouched.

## Capabilities

### New Capabilities
- `interface-addressing-mode`: the per-interface `addressing` field, its validation rules, its default, and the per-service-type runtime contract (which service types run a real DHCP client, which stay static-only, and which are out of scope).

### Modified Capabilities
- `desired-state-manifests`: `services[].interfaces[]` schema gains the `addressing` field and new cross-field validation rules (static requires an address; dhcp forbids one; bridge-enslaved interfaces are exempt).
- `rendering-and-secrets-contract`: the canonical `IFACE_<ROLE>_*` environment contract gains `IFACE_<ROLE>_ADDRESSING`, and the `generic-container` renderer section is updated to reflect the new per-interface (not single default-route) initialization behavior.
- `generic-container-init-entrypoint`: the `VCPE_INIT_DHCP_ROLE`/`VCPE_INIT_STATIC_ROLE`/default-route-implied-DHCP requirements are replaced by requirements describing the entrypoint reading `IFACE_<ROLE>_ADDRESSING` for every declared interface.

## Impact

- **Schema/validation**: `controlplane/internal/manifest/model.go`, `controlplane/internal/manifest/validate.go`.
- **Planning**: `controlplane/internal/planner/planner.go`, `controlplane/internal/plan/model.go` (thread `Addressing` through to the plan).
- **Rendering**: `controlplane/internal/render/env.go` (emit `IFACE_<ROLE>_ADDRESSING`), `controlplane/internal/types/gateway/gateway.go`, `controlplane/internal/types/webpa/webpa.go`, `controlplane/internal/types/eventsink/eventsink.go`, `controlplane/internal/types/oktopus/oktopus.go`, `controlplane/internal/types/genericcontainer/genericcontainer.go`.
- **Container images/entrypoints**: `services/gateway/Containerfile` + `services/gateway/container/entrypoint.sh` (new DHCP client + branching), `services/event-sink/container/entrypoint.sh`, `services/webpa/container/entrypoint.sh` (gate existing `dhclient`), `services/oktopus/Containerfile` (new DHCP client) + its entrypoint.
- **Manifests**: existing manifests (`manifests/**/*.yaml`) that set explicit `ipv4` on `gateway`/`webpa`/`event-sink` interfaces need `addressing: static` added alongside, or need the `ipv4` removed to adopt the new `dhcp` default.
- **Not touched**: `services/xb10/**`, `controlplane/internal/types/xb10/**`.
