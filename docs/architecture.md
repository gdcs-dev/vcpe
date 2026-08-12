# Architecture

## Goal

Provide a local Podman harness for containerized broadband components driven
entirely by a declarative desired-state manifest. The `vcpe` control plane
reconciles a `vcpe.dev/v1` `Deployment` into Podman projects: manifest -> plan ->
apply.

## Control-plane pipeline

`vcpe` resolves a validated manifest into a concrete plan and reconciles it
through deterministic, journaled phases:

1. **Preflight** — schema validation plus registry-aware checks (unregistered
   type, strict per-type `config` decode, expected-role satisfaction) and
   host-network intent preflight. No mutation happens until preflight passes.
2. **Images** — image lifecycle per service `image.pullPolicy`
   (`build-if-missing` | `always-pull` | `never-build`).
3. **Allocation** — IPAM leases and per-interface address assignment.
4. **Render** — typed renderers dispatched by service `type`.
5. **Runtime-init** — per-service startup contracts generated from the resolved
   plan and verified.
6. **Lifecycle** — compose group application.

A failure after allocation triggers a bounded, reverse-order rollback. Operation
phases are recorded in the state store for status and timeline inspection.

## Service type registry

Service behavior is a compile-time registry rather than a per-deployment
catalog. Each `services[].type` maps to a registered `ServiceType` that provides
a config validator, a renderer, expected host-network roles, health behavior,
descriptive metadata, a default image, and a default image policy. Built-in
types embed registry defaults for common curated health, image policy, and
no-op interface validation, then override behavior that differs. The registry
holds no deployment-, customer-, or instance-derived data; "supported type"
means "registered". The built-in type set is `bng`, `event-sink`, `gateway`,
`generic-container`, `oktopus`, `webpa`, and `xb10`; new types register
additively without schema changes. Tools such as the manifest wizard resolve
type defaults through this registry rather than maintaining a parallel catalog.

## Rendering extension contract

Built-in renderers use the typed lifecycle in
`internal/render/servicetemplate`. It owns config decode timing, resolved
instance traversal, root and per-instance artifact paths, and aggregation of
Compose service and network fragments. A per-instance mode consumes concrete
planned instances; an interpolated mode preserves generic-container's
all-replica `${...}` Compose model.

Service packages continue to own workload policy: environment values, network
attachments, privileges, volumes, commands, health topology, and auxiliary
configuration files. The shared lifecycle is not a universal Compose policy
builder. All modes return the existing `render.Result` contract and preserve
`compose.yaml`, root `compose.env`, and one-based `instances/<n>/...` paths.

## Image backend contract

`internal/image.Manager` owns image lifecycle policy and canonical typed
requests. Docker and Podman adapters implement `image.Backend` directly while
retaining separate runtime-specific command construction and diagnostics. Image
references are formatted by one leaf helper: an absent tag defaults to `latest`
for a non-empty repository, while an absent repository remains empty.

## IPAM as the sole IP authority

IPAM is the only component that assigns IP addresses. Explicit interface
addresses are validated as in-CIDR and reserved; all other addresses are
allocated from the network's `pool`. Overlapping allocations are rejected. The
deterministic-identity fallback assigns MACs only, keyed on
`metadata.name/service/role[/index]`, and never invents IP addresses.

## Identity and naming

Network and bridge names derive from the manifest: an explicit `bridge`, or the
`<metadata.name>-<role>` default. Derived names are capped at the 15-character
kernel interface-name limit (IFNAMSIZ) with a deterministic hash suffix on
overflow. The planner and the runtime-init contract use the same canonical-MAC
and bridge-name helpers, so they never diverge.

## State

Persisted state is stamped `schemaVersion: vcpe.dev/v1`. A non-empty state root
with a missing or mismatched stamp is refused with an actionable error directing
the operator to run `vcpe state reset`; there is no automatic migration.
