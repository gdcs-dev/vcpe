## Context

`runDown` in `commands.go` already reads the deployment's last manifest snapshot (via `serviceNamesFromSnapshot`) to know which compose projects to stop. The same snapshot can be used to derive network bridge names — load the manifest, run `planner.Build`, and iterate `dep.Networks` to get bridge names. This avoids storing a separate network list in state.

`teardownComposeLifecycle` is called from `runDown` after which containers are stopped and removed. Network removal must happen after this point so no containers are attached when `podman network rm` is called.

## Goals / Non-Goals

**Goals:**
- `podman network rm <bridge>` for each network bridge after `teardownComposeLifecycle`.
- Best-effort: warn on failure, always proceed with state cleanup.
- `networkProvisioner` interface gains `RemoveNetwork` so tests can record calls.

**Non-Goals:**
- Force-removing networks while containers are still attached (`--force` flag).
- Tracking which networks vcpe created vs pre-existing (all listed in the snapshot are removed).

## Decisions

**Bridge names derived from snapshot + planner, not stored separately**
`serviceNamesFromSnapshot` already parses the snapshot manifest. The same parse + `planner.Build` gives the resolved bridge names (including derived names like `example-wan`). No new state is needed.

**Best-effort removal with `podman network rm` (no `--force`)**
After `teardownComposeLifecycle`, all containers should be stopped. If removal still fails (e.g., another container joined the network manually), we log a warning and continue rather than blocking state cleanup. This matches `teardownComposeLifecycle`'s own best-effort approach.

**`teardownNetworks` called in `runDown`, not inside `teardownComposeLifecycle`**
`teardownComposeLifecycle` doesn't have access to the full deployment plan; it only knows service names. Keeping network teardown in `runDown` (alongside the existing snapshot/lease/state cleanup calls) keeps concerns separated.

## Risks / Trade-offs

**Risk: network shared between two deployments (explicit bridge name override)**
If two manifests specify the same explicit `bridge:` name, removing it on first `down` breaks the second deployment. This is an operator error; we document it and don't add special-case detection.

**Risk: snapshot not available (deployment was never fully applied)**
`serviceNamesFromSnapshot` already handles the no-snapshot case gracefully. `teardownNetworks` follows the same pattern — if no snapshot is found, network teardown is skipped silently.
