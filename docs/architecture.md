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
a config validator, a renderer, the expected host-network roles, and a default
image policy. The registry holds no deployment-, customer-, or instance-derived
data; "supported type" means "registered". The v1 type set is `bng`, `gateway`,
`webpa`, and `generic-container`; new types register additively without schema
changes.

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
