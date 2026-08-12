## Context

Today, IP address assignment for a service interface is decided by three different, inconsistent mechanisms depending on service type:

- **`gateway`**: always static. `interfaces[].ipv4` is applied via `ip addr add` in `services/gateway/container/entrypoint.sh`. If `ipv4` is omitted, the interface is left unconfigured — there is no DHCP fallback despite manifest comments claiming "DHCP client, erouter0."
- **`event-sink` / `webpa`**: unconditionally run `dhclient` on their mgmt interface in their entrypoints, regardless of whether the manifest also sets an explicit `ipv4` (which is pinned into compose's `ipv4_address` by their renderers). The explicit `ipv4`, when set, is silently clobbered by the unconditional DHCP lease.
- **`generic-container`**: has a working DHCP/static convention, but it is driven entirely by `config.env` (`VCPE_INIT_DHCP_ROLE`, `VCPE_INIT_STATIC_ROLE`), keyed off whichever single interface has `defaultRoute: true`. Every other interface on the service gets no address configuration at all.
- **`bng`**: always static (it's the DHCP *server*; a real dnsmasq instance already serves DHCP on both `ipamDriver: none` segments — WAN/CM — and on the Podman-managed `mgmt` network, confirmed in `controlplane/internal/types/bng/bng.go`'s `renderDnsmasqConf`).
- **`xb10`**: has a `start_dhcp_client` function in its entrypoint, but the actual `udhcpc` invocation is commented out — real DHCP negotiation, if it happens at all, happens inside the RDK-B userspace stack the image ships. This change does not touch `xb10`.

The manifest schema (`controlplane/internal/manifest/model.go`) has no field expressing "this interface should DHCP" — presence/absence of `ipv4` is the only signal, and its meaning differs by service type (see above). `planner.go`'s `resolveInstance` does a pure passthrough of `iface.IPv4`/`iface.IPv6` into the plan; there is no IPAM-driven auto-derivation of host addresses despite a stale doc comment on `Interface` suggesting otherwise.

`healthUpstream` (used by `gateway` today) does **not** depend on knowing an interface's resolved address at plan time: the health sidecar dials the workload container by its Podman service name over a dedicated `aa-health` network (see `controlplane/internal/types/gateway/gateway.go`'s `hasHealthUpstream`/proxy-url construction), and the host-facing side is a fixed loopback port from `persist.Store.ReserveHealthEndpoint`. So `dhcp` and `healthUpstream` can coexist on the same interface without any special-casing.

## Goals / Non-Goals

**Goals:**
- One manifest field, one place to look, for every service type and every network (`ipamDriver: none` or Podman-managed).
- `dhcp` is the default; operators opt into `static` explicitly.
- Fix the `event-sink`/`webpa` "explicit ipv4 silently clobbered by DHCP" bug as a side effect of making DHCP conditional.
- Supersede `generic-container`'s `config.env`-driven convention with the same schema field, generalizing it from "one interface" to "every interface."

**Non-Goals:**
- No changes to `xb10` (its entrypoint, image, or manifest interfaces are untouched by this change).
- No validation guarding against nonsensical combinations like setting `dhcp` on `bng`'s own DHCP-server-side interface — that is left to break at runtime.
- No change to how `healthUpstream` resolves its dial target (already independent of interface addressing, see Context).
- No attempt to make Podman's own network-level IPAM (`ipamDriver` other than `none`) aware of the new field — `addressing` only governs in-container behavior (whether the entrypoint runs a DHCP client and/or applies a static address), not Podman's `network create` invocation.

## Decisions

### Field shape: `addressing: dhcp | static` (default `dhcp`)

```yaml
interfaces:
  - role: wan
    device: erouter0
    addressing: dhcp        # default; may be omitted
    healthUpstream: true
  - role: cm
    device: wan0
    addressing: static
    ipv4: "10.7.201.10"     # required when addressing: static
  - role: lan-p1
    device: eth0
    bridge: brlan0           # bridge-enslaved — addressing is ignored
```

Chosen over a boolean (`dhcp: true`) because it leaves room for a future `none` value for interfaces that intentionally get no address at all (already the de facto behavior for bridge-enslaved LAN ports, formalized here as "ignored" rather than a real third enum value — see below).

### Validation rules (`controlplane/internal/manifest/validate.go`)

- `addressing: static` with no `ipv4` and no `ipv6` on the same interface → error.
- `addressing: dhcp` (or omitted) with `ipv4` or `ipv6` set on the same interface → error ("declares an address but addressing is dhcp; set addressing: static or remove the address").
- `addressing` on an interface that also sets `bridge:` → **not validated at all**; the field is accepted but has zero effect on rendering or runtime behavior for that interface, since the bridge (not the enslaved port) is what carries an address.
- The existing "explicit ipv4/mac requires replicas: 1" rule is unchanged and, combined with the new static-requires-ipv4 rule, means `addressing: static` is transitively only valid when `replicas: 1`. No new replica-specific check is needed.
- No new rule cross-referencing a service's own `config` (e.g. `bng`'s `access[].role`) — deliberately, per proposal scope.

### Plan/render plumbing

- `manifest.Interface` gains `Addressing string` (`"dhcp"` or `"static"`; validation guarantees no other value survives; empty string in the manifest is normalized to `"dhcp"` at load or validate time, not left ambiguous downstream).
- `plan.Interface` gains the same field, threaded through `planner.resolveInstance` alongside the existing `IPv4`/`IPv6` passthrough.
- `render.IfaceEnv()` emits a new `IFACE_<ROLE>_ADDRESSING` variable (`dhcp` or `static`) for every interface, including bridge-enslaved ones (emitting the manifest's literal value even though it's ignored keeps the env contract mechanical/uniform — renderers and entrypoints that care about bridge membership already skip bridged roles via `IFACE_<ROLE>_BRIDGE`).

### Per-service-type runtime behavior

| Service | Mechanism when `addressing: dhcp` | Mechanism when `addressing: static` |
|---|---|---|
| `gateway` | New: real DHCP client (package added to the image) invoked per-role in `entrypoint.sh`, keyed on `IFACE_<ROLE>_ADDRESSING` | Existing: `ip addr add $IFACE_<ROLE>_IPV4` (already present, just gated) |
| `event-sink`, `webpa` | Existing `dhclient` call, now gated on `IFACE_MGMT_ADDRESSING=dhcp` instead of running unconditionally | New: skip `dhclient`; rely on the renderer's existing `ipv4_address` pin in compose |
| `oktopus` | New: DHCP client package added to the image, invoked on mgmt | Existing `ipv4_address` compose pin (already present) |
| `bng` | Not expected to be used; no guard — will fail at runtime if misconfigured | Existing behavior, unchanged |
| `generic-container` | Generalized entrypoint change: loop over every `IFACE_<ROLE>_*` set (not just the default-route one), run a DHCP client per role where `ADDRESSING=dhcp` | Generalized: apply static address + gateway per role where `ADDRESSING=static`, replacing `VCPE_INIT_STATIC_ROLE` |
| `xb10` | **Untouched** | **Untouched** |

For Podman-managed networks (e.g. `mgmt`), `dhcp` means running a real client against BNG's dnsmasq (already proven by `event-sink`'s existing `dhclient` call) — Podman's own address assignment on that network is a separate, pre-existing mechanism this change does not disable; the in-container DHCP lease is expected to take over addressing for the interface after Podman brings it up. (This mirrors existing production behavior for `event-sink`/`webpa`, just gated instead of unconditional.)

### Managed-network exclusion (revised after initial implementation)

Every interface now resolves a `ManagedNetwork` bool (`plan.Interface.ManagedNetwork`, emitted as `IFACE_<ROLE>_NETWORK_MANAGED=1` when true) computed by the planner from the interface's network: `true` when the network's `IPAMDriver` is anything other than `"none"` (Podman assigns the address itself before the container starts, e.g. `mgmt`), `false` for `ipamDriver: none` (container/self-managed) networks like `wan`/`cm`.

`addressing: dhcp` on a `ManagedNetwork` interface is accepted (no new validation error, per explicit decision) but has **no runtime effect** — no DHCP client is invoked, since a real DHCP negotiation would be redundant with (and could conflict with) the address Podman already assigned. This is the same "accepted but ignored" treatment as bridge-enslaved interfaces. `addressing: static` is unaffected on managed networks — the existing `ipv4_address` compose pin still applies. Every entrypoint that invokes a DHCP client (`gateway`, `generic-container`, `webpa`, `event-sink`, `oktopus`) checks `IFACE_<ROLE>_NETWORK_MANAGED` before doing so.

### `generic-container` migration

The current contract (`openspec/specs/generic-container-init-entrypoint/spec.md`) is superseded:

- Remove: `VCPE_INIT_STATIC_ROLE`, the default-route-implied-DHCP special case, and the single-interface limitation.
- Add: the entrypoint iterates every `IFACE_<ROLE>_DEVICE` present and, for each, checks `IFACE_<ROLE>_ADDRESSING`: `dhcp` → bring the device up and run `udhcpc` (same flags/hostname/MAC-role behavior as today, generalized to loop); `static` → apply `IFACE_<ROLE>_IPV4`/`GATEWAY4` (and v6 equivalents) via `ip addr add` + `ip route`.
- `VCPE_INIT_MAC_ROLE`, `VCPE_INIT_HOSTNAME`, `VCPE_INIT_NAMESERVER`, `VCPE_INIT_NAMESERVER_FROM[_ROUTE]`, and `VCPE_INIT_SLEEP` are unaffected and remain as-is — only the DHCP/static role-selection mechanism is replaced.
- Existing manifests using `config.env.VCPE_INIT_STATIC_ROLE` or relying on default-route-implied DHCP must be updated to set `addressing` on the relevant `interfaces[]` entries instead (see Migration Plan).

## Risks / Trade-offs

- **[Risk]** Existing manifests that set an explicit `ipv4` on `gateway`/`webpa`/`event-sink` interfaces without a new `addressing: static` will fail validation once the new mutual-exclusivity rule lands (dhcp default + explicit ipv4 = error). → **Mitigation**: update in-repo manifests (`manifests/**/*.yaml`) as part of this change's tasks; this is a deliberate breaking change called out in the proposal.
- **[Risk]** `generic-container` migration is a breaking change to a previously-shipped, working feature (`VCPE_INIT_STATIC_ROLE`). → **Mitigation**: accepted per explicit user direction; update in-repo manifests and document the removal in the superseding spec deltas.
- **[Risk]** Real DHCP clients added to `gateway` and `oktopus` images increase image size and build complexity, and introduce a runtime dependency on BNG being up and serving leases before the interface settles. → **Mitigation**: out of scope to solve startup-ordering robustness beyond what `dependsOn` already provides; existing manifests already declare `dependsOn: [bng]` for these services.
- **[Risk]** No BNG-specific validation guard means a manifest can set `dhcp` on BNG's own uplink and silently break the whole simulated network with a confusing runtime failure rather than a clear validation error. → **Accepted**, per explicit scope decision; not mitigated in this change.
- **[Risk]** `webpa`'s entrypoint previously never ran a DHCP client at all, by deliberate design, because Podman's `127.0.0.1:port:port` health-endpoint publishing is bound to the address Podman assigned `eth0` at network-attach time; replacing it via DHCP after startup can invalidate that binding. Since `dhcp` is now the default, this applies to `webpa` by default unless operators set `addressing: static`. → **Superseded**: the managed-network exclusion below means `dhcp` on `webpa`'s (Podman-managed) mgmt interface is a no-op, matching `webpa`'s original behavior exactly; the risk does not materialize.
- **[Trade-off]** Emitting `IFACE_<ROLE>_ADDRESSING` even for bridge-enslaved interfaces (where it's meaningless) keeps the env contract mechanical and uniform, at the cost of one env var per bridged role that no consumer reads.

## Migration Plan

1. Land schema field + validation + plan/render plumbing (non-breaking on its own — `addressing` defaults to `dhcp`, and no existing manifest sets `ipv4` without also needing an update once validation lands).
2. Update in-repo manifests (`manifests/*.yaml`, `manifests/dev/*.yaml`) that set explicit `ipv4` on `gateway`/`webpa`/`event-sink` interfaces to add `addressing: static`.
3. Update `services/gateway` and `services/oktopus` images to add a DHCP client package; update their entrypoints to branch on `IFACE_<ROLE>_ADDRESSING`.
4. Update `services/event-sink` and `services/webpa` entrypoints to gate their existing `dhclient` call on `IFACE_MGMT_ADDRESSING`.
5. Migrate `generic-container`'s entrypoint (embedded script in `genericcontainer.go`) to the per-interface loop; remove `VCPE_INIT_STATIC_ROLE` handling; update any manifests using it (`manifests/dev/xb10.yaml`'s `client` service uses `defaultRoute: true` with implied DHCP already, which continues to work unchanged since `dhcp` is the default).
6. Update specs (`desired-state-manifests`, `rendering-and-secrets-contract`, `generic-container-init-entrypoint`) and archive/update golden tests across affected renderers.
7. Restage `runtime-init-*` binaries if any runtime-init contract package changes are needed (per `scripts/stage-runtime-init-binaries`), though this change is expected to be entrypoint-script-level for `gateway`/`event-sink`/`webpa`/`oktopus`/`generic-container` rather than requiring `runtimeinit/contract` changes — confirm during implementation.

## Open Questions

- Exact DHCP client binary/package for `gateway` and `oktopus` images (`isc-dhcp-client` to match `webpa`/`event-sink`, or `udhcpc`/busybox to match `routerd`) — left to implementation to decide based on each image's base distro.
- Whether `IFACE_<ROLE>_ADDRESSING` values should be exactly `"dhcp"`/`"static"` (matching the manifest field's own enum values) — assumed yes for simplicity, confirm no existing convention conflicts during implementation.
