## 1. Podman Adapter

- [x] 1.1 Add `RemoveNetwork(ctx context.Context, name string) error` to `internal/backend/podman/adapter.go` — invokes `podman network rm <name>` and returns any error

## 2. Interface and Test Double

- [x] 2.1 Add `RemoveNetwork(ctx context.Context, name string) error` to the `networkProvisioner` interface in `internal/app/orchestrator.go`
- [x] 2.2 Add `RemoveNetwork` to `recordingNetworkProvisioner` in `internal/app/lifecycle_test.go`, recording calls to a `removeCalls []string` slice

## 3. teardownNetworks Function

- [x] 3.1 Add `teardownNetworks(ctx, stateRoot, depName string, provisioner networkProvisioner) error` to `internal/app/orchestrator.go`:
  - Load the manifest from the last desired snapshot (re-use the `serviceNamesFromSnapshot` snapshot loading pattern)
  - Run `planner.Build(doc)` to get resolved bridge names
  - For each `dep.Networks`, call `provisioner.RemoveNetwork(ctx, net.Bridge)` — log warning on error, continue
  - Return `nil` always (best-effort)

## 4. Wire into runDown

- [x] 4.1 In `runDown` (`internal/app/commands.go`): after `teardownComposeLifecycle` succeeds and before `ReplaceCustomerLeases`, call `teardownNetworks(ctx, opts.StateRoot, opts.Name, newNetworkProvisioner())`; skip when `skipRuntime()` is true

## 5. Spec Sync

- [x] 5.1 Apply MODIFIED `Declarative local control-plane commands` requirement to `openspec/specs/local-control-plane-cli/spec.md`
- [x] 5.2 Apply MODIFIED `EnsureNetwork accepts NetworkSpec struct` requirement to `openspec/specs/podman-reconciliation-engine/spec.md`

## 6. Verification

- [x] 6.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 6.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 6.3 Run `VCPE_SKIP_RUNTIME=1 VCPE_SKIP_IMAGE=1 controlplane/bin/vcpe plan --manifest manifests/example.yaml` — unchanged behavior
