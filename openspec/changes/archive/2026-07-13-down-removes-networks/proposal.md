## Why

`vcpe up` creates Podman networks (`podman network create ...`) for every network role declared in the manifest, but `vcpe down` never removes them. After tearing down a deployment the networks remain in Podman's network database, accumulating stale entries, holding their subnet CIDRs, and preventing re-use of those CIDRs by other deployments. An operator must run `podman network rm ...` manually for each network after every `down`.

## What Changes

- Add `RemoveNetwork(ctx context.Context, name string) error` to the `networkProvisioner` interface and implement it in `podman.Adapter` using `podman network rm`.
- After `teardownComposeLifecycle` succeeds in `runDown`, call a new `teardownNetworks` function that loads the network bridge names from the deployment's saved snapshot, runs the planner to resolve bridge names, and removes each network. Failures are best-effort: a warning is logged and teardown continues so partial failures don't block state cleanup.
- Add `RemoveNetwork` to the test double in `lifecycle_test.go`.

## Capabilities

### Modified Capabilities
- `podman-reconciliation-engine`: `vcpe down` SHALL remove the Podman networks that were created for the deployment after all compose services have been stopped. Network removal failures SHALL be treated as warnings — teardown proceeds and state is cleaned up regardless.
- `local-control-plane-cli`: The `down` command SHALL tear down both the compose services AND the Podman networks for the named deployment.

## Impact

- **`internal/backend/podman/adapter.go`** — add `RemoveNetwork(ctx, name string) error`
- **`internal/app/orchestrator.go`** — add `RemoveNetwork` to `networkProvisioner` interface; add `teardownNetworks` function called after `teardownComposeLifecycle`
- **`internal/app/commands.go`** — call `teardownNetworks` in `runDown`
- **`internal/app/lifecycle_test.go`** — add `RemoveNetwork` to `recordingNetworkProvisioner`
- No manifest schema changes; no state format changes; no CLI flag changes
