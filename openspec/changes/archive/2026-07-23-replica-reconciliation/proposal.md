## Why

When an operator modifies `replicas` in an already-deployed manifest and re-runs `vcpe up`, the control plane deploys an entirely new set of containers instead of converging to the desired count. Existing replicas become orphaned (not tracked by the deployment) and are never torn down by `vcpe down`. This makes replica scaling unusable without a full teardown/redeploy cycle.

## What Changes

- **Stable replica container naming**: Replica container names MUST NOT change when `replicas` is increased or decreased. Currently, a single-replica service (`replicas: 1`) generates an un-indexed name (e.g. `client`), while multi-replica services generate indexed names (`client-1`, `client-2`). Scaling from 1 → 2 therefore produces an entirely new set of names, orphaning the original container.
- **Reconcile replica count against running state**: `vcpe up` MUST diff the desired replica count against the currently running instances for each service and only create or remove the delta. Running replicas within the desired count are left untouched; excess replicas are stopped and removed; missing replicas are started.
- **BREAKING**: Existing single-replica deployments will have their container names change from the un-indexed form (`{service}`) to the indexed form (`{service}-1`) the next time `vcpe up` is run after this change ships.

## Capabilities

### New Capabilities
- `replica-reconciliation`: Per-service scale-up/scale-down reconciliation that diffs desired replica count against running instance state and applies only the necessary creates and deletes, leaving unchanged replicas running.

### Modified Capabilities
- `podman-reconciliation-engine`: The idempotent reconcile pipeline must be extended to read current deployed instance state per service, compute the delta (instances to add, instances to remove), and apply only that delta rather than re-applying all instances unconditionally.

## Impact

- `controlplane/internal/plan/identity.go` — naming helper must always use indexed form (`{service}-{n}`) regardless of replica count.
- `controlplane/internal/planner/planner.go` — planner must emit a per-service instance delta (desired vs running) rather than unconditionally planning all instances.
- `controlplane/internal/compose/` — compose adapter must apply only the delta: bring up new instances and remove removed instances without disturbing running ones.
- `controlplane/internal/state/` — persisted deployment state must record which replica indices are live so the planner can compare against them.
- Existing single-replica deployments: container names will change on next `vcpe up` (breaking rename). Operators must `vcpe down` before upgrading.
