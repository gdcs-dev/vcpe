## Overview

This change makes the vCPE Go control plane **fully manifest-driven**. Today the
manifest (`manifest/model.go`) only carries services, replicas, and network
roles/CIDRs; everything else — images, bridge names, podman project names, MAC
addresses, gateways, and per-service config — is fabricated in Go from a single
`deployment.customer` string (`catalog/builtin.go`, `render/bng_renderer.go`,
`planner/planner.go`). This directly violates two already-ratified specs:
`dynamic-topology-ipam` ("compute mappings ... **without hardcoded customer ID
tables**") and `desired-state-manifests` ("manifest is source of truth").

The redesign brings the implementation back into compliance with its own specs
**and** extends the schema to absorb what is currently hardcoded. The
architectural pivot: **`customer` is demoted to an opaque label**, the **manifest
becomes the sole source of instance data**, and the built-in service catalog
collapses into a thin **service-type registry** that only describes type-level
*behavior* (which renderer, which config validator, which host-network roles a
type expects) — never instance data.

This is a greenfield cutover. There is no backward compatibility: the legacy
profile/`.env` compatibility layer, `config/profiles/*`, and
`services/*/customers/*` are removed, and persisted state is reset behind a
schema-version stamp.

The expensive orchestration engine (SQLite operation journal, phased apply with
bounded reverse-order rollback, IPAM, compose/podman adapters, host-network
controller, daemon) is **kept intact**. The change is concentrated in the policy
layer: schema, validation, catalog→registry, planner derivation, and the render
pipeline.

## Components

- **Manifest schema (`manifest`)** — *Modified.* Adds top-level `metadata`
  (name + labels, with `customer` as an optional label), promotes `networks[]`
  to carry explicit `name`/`bridge`, `gateway`, `requiresNat`,
  `requiresFirewall`, and IPv4/IPv6 `pools`, and expands `services[]` with
  `type`, `image`, `interfaces[]`, `dependsOn`, and a type-discriminated typed
  `config` block. `metadata.name` becomes the deployment identity.

- **Schema + per-type validation (`manifest/validate`)** — *Modified.* Performs
  cross-object validation (no hardcoded service allow-lists) and dispatches each
  service's `config` to the validator registered for its `type`. Validates that
  declared interfaces/networks satisfy each type's expected host-network roles.

- **Service-type registry (`catalog` → `typeregistry`)** — *Replaces
  `catalog/builtin.go`.* Maps a `type` to its type-level behavior only:
  `{configValidator, renderer, expectedRoles, defaultImagePolicy,
  dependencyHints}`. Holds **no** customer- or instance-derived data. A type is
  "supported" iff it supplies validator + renderer + expectedRoles. This registry
  is the single source of truth that preflight checks "unsupported type" against.

- **Planner (`planner`)** — *Modified.* Builds desired state directly from the
  manifest: network attachments use the explicit `networks[].name`; interface
  identities come from `services[].interfaces[]`, with MACs assigned by explicit
  value or deterministic fallback hashed on the canonical key. Removes all
  `bng-%s` / `wan-%s` / `deterministicNetworkName(role, customer)` derivation.

- **Render engine + typed renderers (`render`)** — *Replaces
  `bng_renderer.go`.* The engine dispatches by `type` to a typed renderer. A
  `generic-container` renderer handles plain services (env/ports/volumes/command);
  a typed **BNG renderer** consumes the bng-type `config` (DHCP ranges, RA, VLAN,
  interface addressing) and emits artifacts — with **no embedded IP/MAC
  literals**. Unsupported type fails before mutation.

- **IPAM (`ipam`)** — *Largely intact, sole IP authority.* Allocates IPv4/IPv6
  from `networks[].pools`. Explicit interface addresses are validated in-CIDR and
  reserved here. No other component assigns IPs.

- **Runtime-init contracts (`runtimeinit/contract`)** — *Modified.* Startup
  contracts use the same canonical hash key and IPAM-assigned addresses;
  device-ordering semantics preserved.

- **Host-network controller (`hostnet`)** — *Largely intact.* Consumes explicit
  bridge names + NAT/firewall flags straight from `networks[]`.

- **Image manager (`image`)** — *Largely intact.* Consumes `services[].image`
  from the manifest (already policy-aware).

- **Apply pipeline / persist / compose / podman / daemon** — *Intact.* Project
  names now derive from `metadata.name`. `persist` gains a schema-version stamp
  on the state root for the clean cutover.

## Key Architectural Decisions

### Manifest as sole source of truth; catalog demoted to a type registry
**Choice**: All instance data (image, bridges, replicas, interfaces, addresses)
lives in the manifest. The former built-in catalog becomes a behavior-only
service-type registry.
**Rationale**: The hardcoded catalog is the root cause of the customer-ID
coupling and already contradicts the `desired-state-manifests` and
`dynamic-topology-ipam` specs. Separating *type behavior* (stable, code-owned)
from *instance data* (manifest-owned) lets operators onboard new deployments and
services without source edits, satisfying the "dynamic onboarding" requirement.
**Alternatives considered**: Keep a built-in catalog seeded with defaults —
rejected because any built-in instance data reintroduces the customer-ID table
the specs forbid and keeps two competing sources of truth.

### `metadata.name` is the identity key; `customer` is an opaque label
**Choice**: Resource naming (compose project, default network/bridge names,
hash seeds) derives from `metadata.name`. `customer` carries no behavior.
**Rationale**: A single string used both as a human label and as the key that
derives ~70 resource names is the core fragility. Promoting a dedicated identity
field decouples naming from business semantics and makes multi-deployment and
non-customer use cases first-class.
**Alternatives considered**: Keep `customer` as the key — rejected; it perpetuates
the exact coupling this change exists to remove.

### Per-type discriminated config (`service.type` + typed `service.config`)
**Choice**: Each service declares a `type`; its `config` is validated against a
type-specific sub-schema. BNG's DHCP/RA/VLAN/addressing knobs live in the
bng-type config schema, not at the top level.
**Rationale**: The goal is "fully generic including BNG internals," but hoisting
every BNG knob into the universal manifest would create a god-schema coupled to
one service's domain. A discriminated union keeps the top-level schema small
while still expressing every service's config declaratively and type-safely.
**Alternatives considered**: (a) Flat top-level fields for every service's config
— rejected as an unmaintainable god-object. (b) Untyped passthrough maps for all
types — rejected because it loses validation and reintroduces the brittle,
unvalidated substitution the `rendering-and-secrets-contract` spec prohibits.

### IPAM is the sole IP authority; deterministic fallback is MAC-only
**Choice**: IPv4/IPv6 addresses come only from IPAM (explicit-and-reserved, or
pool-allocated). The deterministic-hash fallback assigns **MACs only**. Gateways
are a network-level property (`networks[].gateway`), explicit or defaulted to the
first usable host in the CIDR.
**Rationale**: The coherence review found that hashing IPs could land outside the
CIDR or collide with IPAM allocations. Giving IPAM exclusive ownership of
addressing removes the dual-authority conflict and preserves the spec's
"reject overlapping allocations" guarantee. MACs have no pool/overlap semantics,
so deterministic hashing is safe and keeps interface identity stable.
**Alternatives considered**: Hash IPs too — rejected (CIDR-escape and collision
risk). Require every address explicitly — rejected as poor ergonomics for the
common case.

### Canonical hash key for deterministic identity
**Choice**: `metadata.name + "/" + service.name + "/" + interface.role[ + "/" +
index]`, used identically by the planner and runtime-init contract generation.
**Rationale**: The review found name-only hashing collides across services and
interfaces. Pinning one fully-qualified key guarantees stable, collision-free
MACs and keeps planner and runtime-init in agreement.
**Alternatives considered**: Hash on `metadata.name` only — rejected (collisions).
Hash on `customer` — rejected (reintroduces the demoted key).

### Clean-slate state cutover (no migration)
**Choice**: The persist state root is stamped with the schema version. Legacy
state is abandoned: operators tear down legacy deployments with the old binary,
then apply v1. The first v1 apply has no prior desired snapshot.
**Rationale**: D2 renames nearly every resource, so diffing v1 desired state
against legacy persisted state would flag everything as a disruptive
teardown/recreate and break idempotency across the upgrade boundary. A versioned
clean cutover is honest about the greenfield decision and keeps reconcile
idempotent within v1.
**Alternatives considered**: Write a state migrator that remaps legacy names —
rejected as high-cost throwaway work for a no-back-compat greenfield.

### Typed renderer dispatch by type; unsupported type fails at preflight
**Choice**: The render engine selects a renderer by `type`; preflight rejects any
type missing a registry entry (validator + renderer + expectedRoles) before any
runtime mutation.
**Rationale**: Satisfies the `rendering-and-secrets-contract` requirement to
"fail with a clear unsupported-renderer error before runtime mutation" and makes
the registry the one place that defines what "supported" means.
**Alternatives considered**: Best-effort generic rendering for unknown types —
rejected; it would silently produce wrong runtime config.

## Data Flow

```
                       manifest.yaml (v1)
                              │
                              ▼
            ┌───────────────────────────────────┐
            │ Load + schema validate            │
            │ + per-type config validate        │  fail → reject (no mutation)
            └───────────────────────────────────┘
                              │
                              ▼
            ┌───────────────────────────────────┐
            │ PREFLIGHT                          │
            │  - type supported? (registry)      │  fail → reject (no mutation)
            │  - expectedRoles satisfied by      │
            │    declared interfaces/networks    │
            │  - host caps: bridge/NAT/firewall  │
            │  - disruptive classification       │  disruptive → require --allow-disruptive
            │    (diff vs last v1 desired snap)  │
            └───────────────────────────────────┘
                              │
                              ▼
            ┌───────────────────────────────────┐
            │ PLAN (planner)                     │
            │  attachments  ← networks[].name    │
            │  identities   ← interfaces[]       │
            │     MAC = explicit | hash(key)     │
            │  lifecycle    ← dependsOn order    │
            └───────────────────────────────────┘
                              │
                              ▼
            ┌───────────────────────────────────┐
            │ IPAM ALLOCATION (sole IP authority)│
            │  explicit addr → validate+reserve  │
            │  else          → allocate from pool│  overlap → reject
            └───────────────────────────────────┘
                              │
                              ▼
            ┌───────────────────────────────────┐
            │ RENDER (dispatch by type)          │
            │  generic-container | bng | gateway ... │
            │  typed config in → artifacts out   │  unsupported type → fail
            └───────────────────────────────────┘
                              │
                              ▼
   runtime-init startup contracts (same hash key, IPAM addrs)
                              │
                              ▼
   host-network reconcile → podman networks → compose up → health
                              │
                              ▼
   persist: journal phases + save v1 desired snapshot (schema-stamped)
```

Reconcile = re-apply the same manifest. Within v1 this is idempotent and converges.

## Integration Points

- **`persist` (SQLite state root)** — Desired-state snapshots and the operation
  journal. Gains a `schemaVersion` stamp; the disruptive-change diff reads the
  *latest v1* desired snapshot only.
- **`ipam`** — Consumes `networks[].pools`; owns all address assignment and
  overlap rejection.
- **`hostnet` + `backend/podman`** — Consume explicit bridge names and NAT/
  firewall flags from `networks[]`.
- **`compose` + `backend/podman`** — Project names derive from `metadata.name`;
  env-file artifacts come from the typed renderers.
- **`image`** — Consumes `services[].image` (repo/tag/build-context/pull-policy).
- **`secrets`** — `secretRef` resolution at apply time (`env`/`file` providers).
- **`daemon` + CLI (`app`)** — `--allow-disruptive` is the approval surface for
  disruptive changes; commands and JSON output contracts are unchanged.

## Security Model

- **Secrets**: Never appear in the manifest or persisted state. Only `secretRef`
  (provider + key) is declared; values are resolved at apply time via `env`/`file`
  providers and scrubbed from logs (existing `redaction`). No secret is written to
  the journal or render artifacts.
- **Manifest is non-sensitive config** and may be version-controlled.
- **Trust boundary at host mutation**: bridge/NAT/firewall changes happen only
  after preflight confirms host capabilities; preflight is read-only.
- **Disruptive-change gate**: changes that alter CIDRs, reset identities, remap
  volumes, or scale active services to zero require explicit `--allow-disruptive`,
  classified by diffing against the last v1 desired snapshot. On the first v1
  apply there is no prior snapshot, so a clean create is never mis-flagged.
- **No new auth surface**: the CLI/daemon remain local; this change introduces no
  network-exposed endpoints.

## Error Handling Strategy

- **Validation errors** (schema, per-type config, unsatisfied expected roles)
  fail before planning — no mutation.
- **Preflight failures** (unsupported type, missing host capability, unapproved
  disruptive change) fail before mutation with a clear, actionable error.
- **IPAM conflicts** (overlap, explicit address outside CIDR) reject the plan.
- **Phased apply**: stops on the first terminal phase failure; each phase outcome
  is journaled durably; bounded rollback runs in **reverse** of the successfully
  applied resource/service order (existing engine behavior, preserved).
- **Idempotency**: re-applying the same v1 manifest converges; partial failures
  are recoverable on the next apply via journal recovery.

## Observability Strategy

- **INFO**: command start, each phase start/succeed, plan summary, image actions,
  render artifact summary, IPAM allocations, compose lifecycle, apply success.
- **WARN**: disruptive change detected (and whether approved), drift detected.
- **ERROR**: validation failure, unsupported type/renderer, IPAM conflict,
  preflight host-capability failure, phase failure (with rollback outcome).
- **Metrics/status**: phase outcomes and drift count surfaced in `status`/`logs`;
  `plan` emits a stable JSON diff with `--json`.
- **Secret values are never logged** (redaction applied to all phase messages).

## Constraints

- **No hardcoded customer-ID or service-name tables** anywhere in Go — enforced
  by the registry-as-single-source-of-truth design. (Hard requirement from
  `dynamic-topology-ipam`.)
- **No embedded IP/MAC/gateway literals** in renderers.
- **Typed rendering only** — no regex/string-substitution config generation
  (`rendering-and-secrets-contract`).
- **IPAM is the only component that assigns IP addresses.**
- **Interface device naming is MAC-order sensitive** at container start; planner
  and runtime-init ordering must stay deterministic and agree on the canonical key.
- **Podman + compose runtime** on Linux / podman-machine; the engine, journal,
  and rollback semantics from the reconciliation-engine spec must be preserved.
- **Greenfield**: no backward compatibility with legacy profiles/state.

## Diagrams

Component relationships (policy layer vs. preserved engine):

```
                         MANIFEST (v1)  ── sole source of instance data
                              │
          ┌───────────────────┴────────────────────┐
          ▼                                          ▼
  ┌────────────────┐   behavior only        ┌───────────────────┐
  │ manifest +     │◀──────────────────────▶│ type registry     │
  │ per-type valid.│   (validator/renderer/ │ (no instance data)│
  └────────────────┘    expectedRoles)      └───────────────────┘
          │                                          │
          ▼                                          ▼
  ┌────────────────┐                        ┌───────────────────┐
  │ planner        │  identities/attach.    │ render engine     │
  │ (names from    │───────────────────────▶│ dispatch by type  │
  │  metadata.name)│                        │  generic | bng |… │
  └────────────────┘                        └───────────────────┘
          │   addresses                              │ artifacts
          ▼                                          ▼
  ┌────────────────────────────────────────────────────────────┐
  │             PRESERVED ENGINE (unchanged semantics)          │
  │  ipam  ·  hostnet  ·  image  ·  compose  ·  backend/podman   │
  │  persist (journal + rollback, schema-stamped)  ·  daemon     │
  └────────────────────────────────────────────────────────────┘
```

Discriminated service config (avoids god-schema):

```
services[]:
  - name: edge-bng
    type: bng            ─┐ selects validator + renderer
    image: {repo,tag,…}   │
    interfaces:           │
      - role: wan         │  binds to networks[role=wan]
        mac: <opt|hash>   │  addr: <opt-in-CIDR | IPAM>
    config:              ─┘ validated by bng-type schema
      dhcp: {…}             (DHCP ranges, RA, VLAN, addressing)
      ra:   {…}
  - name: probe
    type: generic-container   selects passthrough validator + renderer
    config: {env,ports,volumes,command}
```
```
