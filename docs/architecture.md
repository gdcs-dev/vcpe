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
data; "supported type" means "registered". Curated types embed
`typeregistry.BaseServiceType` for the common health, image-policy, and
interface-validation defaults, overriding only behavior that differs. New
workloads register additively without planner or renderer changes.

## Renderer extension and artifacts

New service renderers provide typed hooks to `render/servicetemplate`. The
shared lifecycle decodes config, traverses resolved replicas, validates artifact
keys, and aggregates Compose fragments. Service packages retain their own
Compose fields, network attachments, health topology, commands, volumes, and
generated configuration; there is no universal Compose policy.

Per-instance renderers use root `compose.yaml` and `compose.env` artifacts plus
one-based `instances/<n>/` artifacts, with first-instance auxiliary files
mirrored at the root. Interpolated renderers validate their service-owned
artifact layout without applying per-instance placement. These are internal
generated-artifact conventions, not new external behavior.

## Image backends

`internal/image` owns lifecycle policy and its build, pull, push, and tag
request types. Docker and Podman adapters implement that backend contract
directly, while each adapter retains its runtime-specific command arguments and
diagnostics. The application selects an adapter; it does not translate image
requests through a forwarding wrapper.

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
