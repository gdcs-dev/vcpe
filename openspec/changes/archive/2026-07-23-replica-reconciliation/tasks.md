## 1. Stable Indexed Naming

- [x] 1.1 Update `resolveInstance` in `controlplane/internal/planner/planner.go` to always pass `index+1` (1-based) as the replica index suffix in the compose service name, removing the special case that omits the index when `replicas == 1`
- [x] 1.2 Update `CanonicalMAC` in `controlplane/internal/plan/identity.go` to always include the 1-based index in the MAC derivation key, removing the `if index > 0` guard (breaking: MAC addresses will change for existing single-replica services)
- [x] 1.3 Update any renderer templates or render helpers that derive the compose service name from `plan.Instance` to emit `{service}-{index+1}` unconditionally
- [x] 1.4 Update `plan.Instance` and `plan.Service` doc comments to reflect that `Index` is always 0-based internally and is rendered as `Index+1` in external names

## 2. Persisted Replica Count

- [x] 2.1 Add `ReplicaCount int` field to the deployed-service state record (locate the struct in `controlplane/internal/persist/` or equivalent state package)
- [x] 2.2 Write the applied `ReplicaCount` to persisted state on successful service apply in the app/orchestrator layer
- [x] 2.3 Add a migration/fallback so that existing state records without `ReplicaCount` default to `0` (treated as "no prior apply"), causing a full deploy on next `vcpe up`

## 3. Delta Computation in the Planner

- [x] 3.1 Add a `PreviousReplicaCount int` field to `plan.Service` so the planner can communicate the persisted baseline to the apply phase
- [x] 3.2 In `controlplane/internal/planner/planner.go` `Build()`, read the persisted deployment state and populate `PreviousReplicaCount` for each service
- [x] 3.3 Add a `ReplicaDelta` struct (or equivalent fields `ToAdd []int`, `ToRemove []int`) to `plan.Service` computed from `PreviousReplicaCount` vs `Replicas`
- [x] 3.4 Scale-down: `ToRemove` entries MUST be ordered highest-index-first (reverse order)

## 4. Delta Apply in the Orchestrator

- [x] 4.1 Update the apply phase in `controlplane/internal/app/` to use `plan.Service.ReplicaDelta` instead of unconditionally applying all instances
- [x] 4.2 Scale-up: invoke `podman compose up` scoped to only the new service names (`client-{n}` for each index in `ToAdd`)
- [x] 4.3 Scale-down: invoke `podman compose rm` (or `stop` + `rm`) for each service name in `ToRemove`, in reverse index order
- [x] 4.4 Unchanged replicas (in neither `ToAdd` nor `ToRemove`): no compose operation is issued for them

## 5. vcpe down Uses Persisted Count

- [x] 5.1 Update the teardown path to derive the set of container names to remove from the persisted `ReplicaCount`, not from the current manifest file's `replicas` value
- [x] 5.2 Verify `vcpe down` removes all replicas even when the manifest file has since been modified

## 6. Tests

- [x] 6.1 Update `controlplane/internal/planner/` tests to assert 1-based indexed naming for single-replica services
- [x] 6.2 Add planner unit test: `Build()` with `PreviousReplicaCount=1, Replicas=2` produces `ToAdd=[1]` (0-based index 1), `ToRemove=[]`
- [x] 6.3 Add planner unit test: `Build()` with `PreviousReplicaCount=3, Replicas=1` produces `ToAdd=[]`, `ToRemove=[2,1]` (reverse order)
- [x] 6.4 Add planner unit test: `Build()` with `PreviousReplicaCount=2, Replicas=2` produces empty delta
- [x] 6.5 Add planner unit test: `Build()` with `PreviousReplicaCount=0, Replicas=2` (first deploy) produces `ToAdd=[0,1]`, `ToRemove=[]`
- [x] 6.6 Update or add integration-style tests in `controlplane/internal/app/` covering scale-up and scale-down apply paths

## 7. Documentation

- [x] 7.1 Add a release note or BREAKING CHANGES entry noting the container name change for single-replica services and the required `vcpe down` before upgrade
- [x] 7.2 Update `manifests/example.yaml` comments if they reference container naming conventions
