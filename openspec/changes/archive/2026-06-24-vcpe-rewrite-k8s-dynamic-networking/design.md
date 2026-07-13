## Context

This design implements a local-first rewrite of the vCPE control plane while preserving existing service runtime behavior (BNG, GATEWAY, routerd, webpa, xb10, client) on a single Podman host. Current orchestration is shell-driven and coupled to hardcoded network IDs/topology and env-file profiles.

`architecture.md` defines the target architecture and `decisions.md` captures finalized implementation choices. This document focuses on implementation-level structure, sequencing, and risk handling needed to execute those choices.

Key constraints:
- Single Podman backend only in this phase
- Bounded local scaling and host resource protection
- Deterministic interface-role behavior for compatibility
- Backward compatibility path from env profiles to versioned manifests

## Goals / Non-Goals

**Goals:**
- Deliver a Go control-plane CLI with `plan`, `apply`, `status`, `destroy`
- Define and validate a versioned manifest model (Profile, Deployment, Status)
- Implement topology planning + IPAM to remove hardcoded customer IDs
- Persist reconcile and allocation state in SQLite with replay-safe recovery
- Provide env profile translation and secrets reference contract (`env`, `file`)
- Provide disruptive-change gating and bounded rollback semantics

**Non-Goals:**
- Kubernetes backend or cluster orchestration
- Full rewrite of service container internals in this phase
- External secret managers beyond phase-1 providers
- Unbounded autoscaling or multi-host scheduling

## Decisions

### Control-plane process model
- CLI-first binary with optional local daemon.
- Single execution authority to avoid split-brain writes:
  - CLI mode: command process performs mutation.
  - Daemon mode: daemon owns all mutating operations; CLI delegates via local socket.
- Shared SQLite state and lock contract across both modes.

Alternatives considered:
- Concurrent CLI+daemon mutation: rejected due to race/drift risk.

### Data model and validation contract
- Canonical desired state is manifest-driven with schema versioning.
- Separation:
  - Profile: reusable defaults and policy caps
  - Customer Deployment: concrete desired service/network state
  - Runtime Status: observed reconciliation/output state
- Validation phases:
  - Schema validation
  - Cross-object invariants (resource caps, required services, network uniqueness)
  - Policy checks (disruptive flags, profile restrictions)

Alternatives considered:
- Env-only configuration: rejected due to weak validation and drift handling.

### Reconciler and transaction strategy
- Plan computes desired-vs-actual diff and labels disruptive operations.
- Apply executes phase pipeline with operation journal:
  - Preflight
  - Resource allocation/topology
  - Render/config materialization
  - Podman lifecycle changes
  - Health verification
- On terminal failure: fail-fast + bounded rollback for resources created in current operation.
- Re-apply converges idempotently.

Alternatives considered:
- Best-effort partial continue: rejected due to hidden drift.

### Dynamic topology and IPAM behavior
- Topology planner maps logical roles (`mgmt`, `wan`, `cm`, `lan-p*`) to concrete Podman network attachments.
- IPAM allocates/reclaims IPv4/IPv6 ranges and host/service addresses from configured pools.
- Deterministic role identity preserved; disruptive role/CIDR changes require explicit approval.

Alternatives considered:
- Hardcoded customer network tables: rejected as non-scalable.

### Profile compatibility and rendering
- Translation layer imports legacy env profiles into canonical manifests and can export for compatibility mode.
- Render service consumes typed inputs and validated templates; regex-only ad hoc replacements are retired.
- Secrets are references only (`secretRef`) and resolved at apply time from `env` or `file` providers.

Alternatives considered:
- Persisting secret values in manifest/state: rejected for security reasons.

## Risks / Trade-offs

- Manifest and env parity drift during transition → Build round-trip import/export tests and explicit unsupported-field reporting.
- Topology/IPAM misallocation causing service breakage → Add deterministic planner test corpus and collision simulation tests.
- Daemon/CLI mode confusion for operators → Clear mode introspection in `status` and explicit mutation authority messages.
- Rollback incompleteness under partial Podman failures → Keep phase journal with compensating actions and post-failure orphan sweep.
- Performance regressions on low-resource laptops → Enforce default host budget and conservative reconcile concurrency.

## Migration Plan

1. Introduce control-plane binary and manifest schema with no-op validation path.
2. Add env-to-manifest importer and profile compatibility command set.
3. Implement planner/IPAM and `plan` diff output against existing runtime.
4. Implement reconcile/apply with bounded rollback and SQLite journal.
5. Integrate compatibility wrappers from existing scripts to control-plane commands.
6. Introduce typed rendering and secrets reference resolution.
7. Enable disruptive-change gating and local resource-cap enforcement by default.
8. Expand tests (unit + Podman integration + smoke) and switch local docs/runbook paths.

Rollback strategy:
- Keep legacy scripts operational during phased rollout.
- Preserve previous manifest snapshots and operation journal entries for explicit re-apply rollback.
- Provide command to revert active deployment to last successful snapshot.

## Open Questions

- Should daemon mode be enabled automatically for long-running watch workflows or remain explicit-only?
- What exact default host budget thresholds should be selected per profile class beyond initial global caps?
- Which legacy env fields are deprecated immediately versus supported for one compatibility release window?
