## Context

Health-capable services normally publish container port 9878 to a stable host-loopback port reserved in persisted state. A workload whose topology networks use `ipamDriver: none` has no Podman-assigned address suitable for forwarding, so the gateway renderer currently attaches both the workload and a per-instance proxy container to `<deployment>-00-health`; only the proxy publishes the host port. The workload already runs `vcpe-healthd` on `:9878`, and gateway startup already preserves the extra Podman-managed interface as `vcpe-health0`.

The status collector must remain HTTP-only and must not inspect Podman at observation time. The same behavior must work when Podman runs locally on Linux or remotely through Podman Machine on macOS.

Generic-container probe helpers are a separate mechanism: they share the workload network namespace and run a configured probe when the workload does not provide the standard response itself. They are not transport proxies.

## Goals / Non-Goals

**Goals:**

- Publish every supported instance's standard health endpoint directly from its workload network namespace.
- Use one private Podman-managed health network per deployment only when topology attachments cannot support forwarding.
- Eliminate per-instance transport proxy containers and transport opt-in from manifests.
- Provide collision-free loopback ports, HTTP-only collection, replica isolation, and generic probe execution without preserving unreleased health-state identities or transport behavior.
- Verify direct forwarding on Linux and macOS Podman Machine before introducing the production transport.

**Non-Goals:**

- Expose health endpoints on topology networks or non-loopback host addresses.
- Discover runtime container addresses or ports with Podman inspection.
- Replace generic-container command/HTTP probe helpers when probe execution still requires them.
- Add a persistent host daemon or a deployment-level relay unless direct forwarding is disproved by the required platform spike.

## Decisions

### Publish directly from the workload over the private health attachment

When a health-capable instance has no Podman-managed topology network, rendering SHALL attach its workload Compose service to the deployment's `aa-health` external network and add `127.0.0.1:<reserved>:9878` to that workload's `ports`. Podman then forwards to the workload network namespace through the managed attachment; the endpoint's topology address, including a DHCP-assigned address, is irrelevant.

This removes the proxy process, image reuse, service-name lookup, proxy timeout, restart policy, and extra lifecycle dependency. Directly dialing planned topology addresses was rejected because DHCP addresses may be unknown and macOS cannot route directly into Podman Machine networks. A host process or one relay container per deployment was rejected as unnecessary infrastructure. If the cross-platform spike falsifies direct forwarding, this design SHALL be revised before implementation; the unreleased per-instance proxy is not a compatibility fallback.

### Keep health-network selection automatic and plan-driven

The control plane SHALL reserve a health endpoint for every instance whose registered type has valid health behavior, subject only to an optional type's health configuration being present. It SHALL require the private health network when such an instance has no topology attachment with `ipamDriver != "none"`. No runtime Podman query or manifest-selected upstream interface participates in this decision.

The existing `<deployment>-00-health` network and `aa-health` Compose alias remain. The alias SHALL be rendered deterministically before topology network aliases where interface order matters, preserving gateway's `vcpe-health0` handling.

### Attach only the workload service

Health-network and port helpers SHALL mutate the intended workload Compose service explicitly rather than walking every service in a generated document. In particular, a generic-container probe helper using `network_mode: service:<workload>` MUST NOT also receive a `networks` entry; it inherits both the health attachment and published endpoint through the workload namespace.

Services that already have a Podman-managed topology attachment continue to publish directly without receiving `aa-health`. Operator-declared application ports remain unchanged and separate from allocated health mappings.

### Remove `healthUpstream`

`services[].interfaces[].healthUpstream` SHALL be removed from manifest and plan models, validation, examples, and renderer conditions. Strict manifest decoding will reject the unsupported field. No compatibility alias, deprecation period, or replacement field is required because the health-check implementation has not been released.

### Replace the proxy helper with a direct-publication helper

The shared renderer package SHALL remove `AttachProxySidecar` and provide a narrowly scoped helper for adding the external health network, workload attachment, and loopback port mapping. `vcpe-healthd` SHALL remove proxy-only flags and code, including `--proxy-url`. The namespace-sharing probe helper remains because it executes configured probes and is part of the new design, not for compatibility. Orchestration owns whether the deployment health network is provisioned; rendering owns the per-workload Compose shape.

## Risks / Trade-offs

- [Podman may select an unmanaged attachment for forwarding in some multi-network configuration] -> Add a focused runtime test with `aa-health` plus multiple `ipamDriver: none` networks on Linux and macOS Podman Machine; require direct HTTP success before deleting proxy code.
- [The extra managed interface can disturb service interface naming] -> Preserve deterministic health-network ordering and verify gateway maps the unmatched interface to `vcpe-health0`; add equivalent focused coverage for every self-addressed built-in type.
- [A health daemon bound only to loopback would not accept forwarding through `aa-health`] -> Keep the standard endpoint bound to `:9878` and test it through the published host port.
- [Development manifests may still contain `healthUpstream`] -> Remove the field from all repository-owned artifacts and reject it through strict schema validation; no compatibility path is needed.
- [Automatic publication increases port reservations for instances previously skipped] -> Continue using the collision-safe persisted allocator and verify teardown releases all newly eligible records.

## Pre-release Cutover

1. Prove direct forwarding with an isolated multi-network Podman test on Linux and macOS Podman Machine.
2. Replace health endpoint reservation and direct-publication rendering as one implementation; health-specific persisted records may be discarded and regenerated.
3. Migrate gateway and other self-addressed types, then remove proxy services, proxy helpers, proxy command flags, and their tests.
4. Remove `healthUpstream` from schema, plan, validation, examples, and tests.
5. Reset development deployments and health-specific state before end-to-end verification.

No health-specific migration, dual-read behavior, compatibility alias, or rollback conversion is required. Source control can revert the whole unreleased change during development; partially converted runtime or persisted health state is disposable.

## Open Questions

- Does every supported Podman version choose the managed `aa-health` attachment for forwarding when earlier topology attachments have `ipamDriver: none`? The mandatory spike resolves this before implementation proceeds past the transport layer.
- If direct forwarding fails on a supported platform, should the proposal be revised to one relay per deployment or should that platform require a newer Podman/network backend? This is a design decision to make from spike evidence rather than silently retaining per-instance proxies.

## Spike Evidence (Tasks 1.1–1.3)

`tests/smoke/aa-health-multi-network-forwarding.sh` attaches one workload to two
`--ipam-driver none` networks plus one Podman-managed network (mirroring
`aa-health`), publishes `127.0.0.1:<port>:9878` on the workload, and asserts the
HTTP response without any container inspection.

- **macOS Podman Machine**: PASS. Podman client 6.0.2, network backend
  `netavark` 6.0.2, machine arch `aarch64`. The published port forwarded to the
  workload's `vcpe-healthd` through the managed attachment even with two
  unmanaged topology networks also attached, confirming Podman selects the
  managed attachment's assigned address for the DNAT rule regardless of
  interface count or order.
- **Local Linux Podman**: not verified in this session (no bare-metal Linux
  Podman host was available in the working environment). No forwarding failure
  was observed on the platform that was available, so implementation proceeds
  per the "Publish directly from the workload" decision; Linux-native
  verification remains a follow-up before considering the platform matrix
  fully closed. netavark's DNAT rule construction (assigned-address port
  forwarding, independent of unmanaged sibling networks) is not
  platform-specific to Podman Machine, so no divergent Linux behavior is
  expected, but this is inference rather than direct evidence.

### End-to-end gateway smoke findings (Task 5.2)

A full `vcpe up`/`vcpe status` run against a self-addressed gateway (`wan`
`ipamDriver: none`, no other topology attachment) surfaced two real,
pre-existing `services/gateway/container/entrypoint.sh` bugs that the isolated
spike above did not exercise (it used a plain alpine workload with no routing
or NAT of its own). Both are now fixed:

1. **Silent early exit under `set -e`.** `start_lan_dhcp`'s last statement was
   a bare `[[ $has_config -eq 1 ]] && dnsmasq ...`. When no bridge has DHCP
   configured (`has_config` stays 0), that compound statement's exit status
   (1) became the function's — and therefore `main()`'s — exit status,
   silently terminating the entrypoint under `set -e` before it ever reached
   `exec /sbin/init`, with no error output. Fixed by using an explicit `if`
   block instead of `test && action` as a standalone statement.
2. **Tunnel pseudo-devices could steal the `vcpe-health0` rename.** The
   MAC-based interface-identification scan did not exclude kernel tunnel
   pseudo-devices (`gre0`, `gretap0`, `erspan0`, ...), which report an
   all-zero address. Depending on `/sys/class/net` iteration order, one of
   these could be misidentified as "the unmatched interface" and renamed to
   `vcpe-health0` instead of the real managed `aa-health` attachment, leaving
   the real interface un-renamed (functionally reachable, but not matching
   the documented `vcpe-health0` naming contract). Fixed by excluding
   all-zero addresses (in both 6-octet and shorter tunnel-device forms) from
   the interface map.
3. **Default-route hijack of the health reply path.** Gateway's own
   `ip route replace default via ... dev erouter0` (needed so LAN-client NAT
   egress reaches the simulated WAN) unconditionally overrides the managed
   health attachment's own default route. A health-check request whose source
   address isn't in any locally-attached subnet — which happens when the
   request is forwarded through Podman Machine's host↔VM tunnel — then has
   its reply black-holed via `erouter0` instead of returning via
   `vcpe-health0`, even though the same request succeeds when it originates
   from inside the Podman Machine VM (whose own address falls in the health
   network's subnet, matching a more specific route). Fixed with a
   source-based policy route (`ip rule add from <vcpe-health0 address> table
   100` + a matching default route in table 100) that keeps `vcpe-health0`'s
   own gateway authoritative for anything sourced from its own address,
   independent of the global default route erouter0 needs for its actual
   purpose. This is a no-op when no `vcpe-health0` interface exists (i.e.
   every topology attachment is already Podman-managed).

After both fixes, a full `vcpe up` → `vcpe status --name <deployment>` cycle
against this self-addressed gateway produced exactly one workload container
(`<deployment>-gateway-1`, no per-instance proxy service), a loopback-only
published mapping (`127.0.0.1:<port>:9878`), and a reachable, well-formed
health response from the host (`vcpe status` reported `health gateway/0:
unhealthy` — a valid HTTP response; `unhealthy` reflects gateway's own
webpa/parodus application checks in a minimal single-service smoke topology,
unrelated to this change).