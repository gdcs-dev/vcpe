## Context

`routerd` is a vendored Rust reconciliation control plane (source referenced under `services/routerd/source/routerd/`, not committed) whose only external contract is `routerctl apply <file>` / `routerctl replace <file>`, which parse a `ConfigDocument` — `{"resources": [...]}` (or a bare JSON array) of envelope objects `{apiVersion, kind, metadata, spec}`. The kinds relevant here, with their `spec` shapes (from `crates/l2/l2-types`, `crates/l3/l3-types`, `crates/dhcp-client/dhcp-client-types`, `crates/wan/wan-types`):

- `Interface { name, admin_up?, mtu? }`
- `Bridge { name, members: [ResourceRef], admin_up?, mtu? }`
- `IpAddress { interface_ref, address, prefix_len }`
- `Route { table, destination, gateway?, interface_ref, metric }`
- `DhcpClient { interface_ref, hostname?, client_id? }`
- `WanPolicy { interface_ref, source: Static{address,prefix_len,gateway} | Dhcp, priority }` — the WAN controller derives `DhcpClient`/`IpAddress`/`Route` intents from this, so a single `WanPolicy` resource is normally sufficient to express "this interface gets its address via DHCP/statically."

Today's container pipeline (`services/routerd/container/entrypoint.sh` + `render-config.sh`) predates this contract: it renames interfaces from a hardcoded `LAN1_MAC`/`WAN0_MAC`/`EROUTER0_MAC` table (never populated for routerd's manifest), then builds a `{"version":"1","controllers":{"interfaces":{...},"network":{...}}}` blob from `BRLAN0_*`/`WAN0_*`/`EROUTER0_*` env vars that `routerctl` cannot parse. The manifest ([manifests/dev/routerd.yaml](../../../manifests/dev/routerd.yaml)) declares routerd as `type: generic-container` with one `wan`/`eth0` interface and no `config:` block, so none of those env vars are ever even produced.

The control plane already has an established pattern for this exact problem — `bng` (`controlplane/internal/types/bng`): a typed `Config` decoded from the manifest's `config:` block, a `Renderer` that compiles it plus resolved interface identities into config-file artifacts, and a curated/generated compose that mounts those files in. This design applies the same pattern to routerd, targeting its resource-kind wire format instead of BNG's flat-file formats.

## Goals / Non-Goals

**Goals:**
- Let an operator declare routerd's LAN bridge topology (which interface roles are enslaved to which bridge) directly in the manifest `config:` block.
- Reuse the existing per-interface `addressing: dhcp|static` field as the sole source of routerd's WAN addressing intent (no parallel/duplicate schema).
- Compile the manifest into a `ConfigDocument` server-side (Go, at render time), not at container boot.
- Make container startup a thin `routerctl apply` against a pre-rendered file, consistent with the `VCPE_RUNTIME_CONFIG_PATH` seam already stubbed in `servicecmd.Run`.
- Bring routerd's interface renaming onto the generic `IFACE_<ROLE>_MAC`/`IFACE_<ROLE>_DEVICE` mechanism (`manifest-driven-interface-names`), retiring its bespoke MAC table.

**Non-Goals:**
- Modeling every routerd resource kind (no `WanPolicy` failover/multi-priority UI, no arbitrary `Route`/`IpAddress` authoring beyond what addressing + topology imply). Static extra routes can be a follow-up.
- Changing routerd's Rust source or its `routerctl`/config-service wire contract — this change only produces documents that contract already accepts.
- A generalized "any resource kind" manifest escape hatch. The typed config models exactly the two concepts this change scopes: bridge topology and per-interface addressing.

## Decisions

**1. New `routerd` service type instead of extending `generic-container`.**
`generic-container` always emits its own `entrypoint.sh` and forces `entrypoint: ["/bin/sh", "/run/vcpe/entrypoint.sh"]` in compose (`generic-container-init-entrypoint`), which conflicts with routerd's Containerfile-baked `ENTRYPOINT runtime-init-routerd` / `CMD` chain and gives routerd no way to own a typed config schema or a resource-kind renderer. A dedicated type is the same shape as `bng`/`gateway` and lets `service-type-registry` express routerd's expected roles and config validation explicitly. Alternative considered: keep `type: generic-container` and add an opaque `config.routerdTopology` escape hatch — rejected because it can't get strict per-type config validation or a dedicated renderer without registry support anyway, so it buys nothing over a real type.

**2. Bridge topology reuses the manifest's existing `interfaces[].bridge` + `services[].bridges[]` fields — no routerd-specific schema.**
Initially explored a routerd-specific `config.bridges: [{name, members: [roles]}]` schema. Live validation surfaced a consistency problem: every other bridging service type (`gateway`, `bng`) already declares bridge membership per-interface via `interfaces[].bridge: <name>` (optionally paired with a `services[].bridges[]` entry for the bridge's own IP/DHCP settings). Maintaining a second, routerd-only way to say the same thing forced operators to remember which schema applies to which service type, and a manifest hand-edited to match the gateway convention silently produced no bridge at all (the renderer only read `config.bridges`). Switched to deriving bridge topology purely from `interfaces[].bridge`: the renderer groups a service's resolved interfaces by their `Bridge` field and compiles each group into one `Bridge` resource. `routerd`'s typed `Config` is now empty — no type-specific fields — and this also eliminates the "undeclared role" validation case entirely (grouping only ever includes interfaces that exist, by construction). Explored inferring topology from role-naming convention instead of any explicit field — rejected for the same reason as before: it hardcodes a single-bridge assumption and breaks silently if role names change.

**3. Addressing reuses `interfaces[].addressing`, translated into `WanPolicy`; priority derived from `defaultRoute`.**
Every interface already resolves an `addressing: dhcp|static` value (`interface-addressing-mode`). The routerd renderer maps each *non-bridged* interface (bridge members don't get their own WAN policy — the bridge itself could, if a future manifest needs an addressed LAN bridge, but that's out of scope here) to one `WanPolicy` resource: `addressing: dhcp` → `WanPolicy{source: Dhcp}`; `addressing: static` → `WanPolicy{source: Static{address, prefix_len, gateway}}` using the interface's `ipv4`/`gateway4`. This keeps the manifest's addressing vocabulary identical across every service type — routerd's runtime translation (resource application) differs, but the schema does not. Initially fixed `priority: 1` unconditionally (single-uplink assumption) — live validation against a manifest with two unbridged DHCP interfaces (`wan` + `cm`, mirroring gateway's `erouter0`/`wan0` pair) immediately produced a routerd route conflict ("equal-authority contenders excluded") since both got the same priority/metric. Fixed by deriving priority from the existing `defaultRoute` field: the interface marked `defaultRoute: true` gets `priority: 1`; any other unbridged interfaces get strictly increasing priorities in role-name order. This reuses a field that already exists for exactly this purpose in every other service type's entrypoint (`manifest-driven-interface-names`: "only the interface marked default-route may keep a default route") rather than inventing new schema.

**4. Config is a render artifact, applied via `routerctl apply` at startup, not templated shell.**
The renderer emits the full `ConfigDocument` as JSON text (Go `encoding/json`, matching `resource_types::document::ConfigDocument`/`RawResource` field names — `apiVersion`/`kind`/`metadata`/`spec` in the wire's expected case) as a `render.Artifact{Key: "etc/routerd/config.json", ...}`, mounted read-only the same way BNG mounts `etc/dhcp/dhcpd.conf`. `entrypoint.sh` is reduced to: rename interfaces (generic IFACE_* mechanism), start `routerd`, wait for the socket, `routerctl apply "$ROUTERD_CONFIG"`, exec. `render-config.sh` and its env-var scraping are deleted outright rather than repaired, since the format it produces has no consumer.

**5. `ResourceRef` construction for bridge members and `WanPolicy.interface_ref`.**
`Bridge.members` and `WanPolicy.interface_ref` are `ResourceRef`s pointing at `Interface` resources by name/id. The renderer names every `Interface` resource after its manifest device name (e.g. `eth0`) — the same identifier already used as the container-internal device name — so `ResourceRef`s are simple by-name references with no separate ID-allocation scheme.

## Risks / Trade-offs

- **[Risk]** The exact JSON field casing/shape of `ConfigDocument`/`RawResource`/kind-specific specs is inferred from the vendored Rust source's `serde` derives (default `serde_json` behavior is field-name-as-written, i.e. `snake_case` per the Rust struct fields, wrapped in an envelope using the derived `Serialize` — this needs confirming against a live `routerctl` round-trip, not just static reading) → **Mitigation**: task list includes a round-trip smoke test (`routerctl apply` against a rendered doc in the actual container) before considering the renderer done; treat the Rust source as the source of truth for wire shape, not this design doc's paraphrase.
- **[Risk]** Introducing a new service type is a breaking manifest change for anyone who already deployed routerd as `generic-container` → **Mitigation**: proposal calls this out explicitly as BREAKING; this is a dev-only manifest (`manifests/dev/routerd.yaml`) with a single known consumer, so there is no compatibility shim planned.
- **[Trade-off]** Priority is derived only from `defaultRoute` + role-name tiebreak, not an explicit manifest priority field — sufficient for today's two-uplink (`wan`/`cm`) case but not a general N-way multi-WAN failover ordering UI; revisit if a future manifest needs explicit control over uplink preference order.
- **[Risk]** The control plane itself is mid-rewrite (`AGENTS.md`: `go build ./...` currently fails); this change lands on top of that rewrite rather than the pre-rewrite schema, so task sequencing should assume the manifest/typeregistry APIs are still shifting under `manifest-driven-redesign` tasks 6–11.

## Migration Plan

1. Land the new `routerd` type + renderer (additive; nothing consumes it yet).
2. Update `manifests/dev/routerd.yaml` to `type: routerd` with LAN topology expressed via `interfaces[].bridge` + `services[].bridges[]` (the same fields gateway uses).
3. Rewrite `services/routerd/container/entrypoint.sh`, delete `render-config.sh`, update the Containerfile.
4. Validate end-to-end with a real `vcpe up` against the dev manifest + a `routerctl` round trip inside the running container.

No rollback tooling is needed beyond reverting the manifest/type change — there is no persisted state format introduced by this change.

## Open Questions

- Should an addressed **bridge** (not just a standalone interface) be expressible in this iteration, or is "bridges are always internal-LAN and never carry a WanPolicy" an acceptable permanent constraint? Left as a non-goal above; revisit if a topology needs it.
- Does the vendored `routerd` require resources to be submitted in dependency order (e.g. `Interface` before `Bridge` before `WanPolicy`) within a single `ConfigDocument`, or is ordering irrelevant to `routerctl apply`? Confirm against the source's `apply_document`/`ApplyMode::Upsert` path before finalizing renderer output ordering.
