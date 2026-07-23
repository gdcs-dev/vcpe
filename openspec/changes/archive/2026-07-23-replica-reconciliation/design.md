## Context

`vcpe up` applies a manifest by planning all desired instances from scratch and passing them to the compose adapter. When the same deployment already has containers running, the adapter `up`s the compose project which creates any containers not present — but it has no concept of "containers that exist but are no longer desired." The naming scheme for single-replica services differs from multi-replica services (no index suffix vs. `-1`, `-2`, …), so changing `replicas: 1 → 2` produces an entirely new set of container names, orphaning the original.

The two independent bugs compound each other:

1. **Naming is unstable across replica count changes.** `CanonicalMAC` already uses `index == 0` as the "single-replica" sentinel to omit the index from the MAC key; the same inconsistency exists in the compose service name produced by the renderer — `client` vs `client-1`.
2. **Apply has no delta awareness.** After a plan is computed the apply phase drives `podman compose up` against the full desired set; it never inspects running containers to compute what to add or remove.

## Goals / Non-Goals

**Goals:**
- Replica container names are stable: a service at `replicas: 1` and the same service at `replicas: 2` use the same name for index 0 (`{service}-1`).
- `vcpe up` with an already-deployed manifest only creates the new replicas and removes excess replicas; running replicas within the desired count are left untouched.
- `vcpe down` correctly tears down all live replicas regardless of the replica count that was used when they were created.
- The BREAKING rename (single-replica services gain an `-1` suffix) is clearly communicated so operators can plan a teardown/redeploy window.

**Non-Goals:**
- Live migration of container workloads (running state is replaced, not migrated).
- Partial restarts of individual replicas that haven't changed (only missing/excess replicas are touched).
- Cross-deployment replica rebalancing.

## Decisions

### Decision: Always use 1-based indexed names for compose services

The compose service name for replica `i` (0-based) SHALL always be `{service}-{i+1}`, even when `replicas == 1`. This produces `client-1` for the first replica, `client-2` for the second, and so on. The current `client` (no index) form is eliminated.

**Why**: Stable names make it possible to enumerate desired vs running containers without special cases. The alternative — keeping `client` for `replicas == 1` and switching to `client-1` only when `replicas > 1` — forces special-case logic in every reconciliation path and was the direct cause of the bug.

**Breaking impact**: Any deployment where `replicas == 1` currently has containers named `{project}_{service}_1`. After this change they will be named `{project}_{service}-1_1`. Operators must `vcpe down` before upgrading.

### Decision: Delta reconciliation driven by persisted desired replica count

After computing the desired plan, the apply phase SHALL read the persisted deployment state to determine the *previously applied* replica count for each service. It then computes:
- **to-add**: indices in `[0, desired)` not currently running
- **to-remove**: indices that were in `[0, previous)` but are no longer in `[0, desired)`

Only the delta is acted on. This avoids unnecessary container restarts.

**Why this over "inspect live Podman state"**: Podman `ps` is authoritative but slow and fragile across daemon restarts. Persisted state is already required for `vcpe down`; using it as the reconciliation baseline is consistent with the existing state schema and faster.

### Decision: Replica count is stored per-service in deployment state

The persisted `DeployedService` record SHALL include a `ReplicaCount` field. On each successful apply, the planner updates this field to the replica count that was applied. This becomes the baseline for the next delta computation.

### Decision: Scale-down removes excess replicas in reverse index order

When decreasing replica count the system SHALL stop and remove the highest-index replicas first (e.g. scaling 3 → 1 removes index 2 then index 1, leaving index 0). This is deterministic and minimises disruption to the lowest-index replica which is the most likely to hold state (e.g. a primary in a leader/follower set).

## Risks / Trade-offs

- **State drift after manual container removal**: If an operator manually removes a container outside `vcpe`, persisted state will still record it as present. The planner will not re-create it. Mitigation: the existing `vcpe status` command can surface state-vs-reality divergence; a future `vcpe reconcile` command can force a full re-sync.
- **Breaking rename**: Every existing single-replica deployment is broken by the naming change. Mitigation: the breaking change is flagged clearly in the proposal and release notes; `vcpe down` before upgrade is sufficient.
- **Compose project `up` on partial state**: `podman compose up` on a project with some services already running will start missing services without touching running ones. This is the intended behavior for scale-up but must be confirmed to not produce ordering side-effects for `dependsOn` services.

## Migration Plan

1. `vcpe down` any active deployments before upgrading the control plane binary.
2. Apply the new binary.
3. `vcpe up` re-deploys with the new stable indexed names and writes the new `ReplicaCount` state field.

Rollback: revert to the previous binary and `vcpe down` / `vcpe up` again (container names revert to un-indexed form).

## Open Questions

- Should `vcpe status` report a warning when persisted replica count differs from the number of running containers (state drift detection)?
- Should `CanonicalMAC` be updated to always include the index in the key, or should it preserve the zero-index-omits-index behavior for MAC stability on existing deployments?
