## 1. Command Surface And Compatibility Removal

- [x] 1.1 Replace wrapper-owned command entrypoints with canonical `vcpe` and `vcpe service <name> ...` handlers in the Go CLI tree.
- [x] 1.2 Remove or retire top-level wrapper scripts and update any remaining command-dispatch references.
- [x] 1.3 Add structured errors and help mapping from removed wrapper paths to canonical `vcpe` commands.
- [x] 1.4 Add integration tests for service-scoped command grammar and unsupported service-name handling.

## 2. Host Network Controller Migration

- [x] 2.1 Implement Go host-network reconcile phases for bridge, NAT, firewall, and attachment intents.
- [x] 2.2 Replace usage of `platform/scripts/lib/bridges.sh`, `platform/scripts/lib/firewall.sh`, and `platform/scripts/net` with Go-owned mutation paths.
- [x] 2.3 Add host capability preflight checks and actionable failure diagnostics before mutation.
- [x] 2.4 Implement and test macOS Podman-machine or delegated Linux-host execution support in host-network flows.

## 3. Runtime Init And Startup Contract

- [x] 3.1 Add a shared runtime-init Go bootstrap library with deterministic phase ordering.
- [x] 3.2 Build thin per-service runtime-init binaries for documented services (`bng`, `gateway`, `routerd`, `webpa`, `xb10`, `client`).
- [x] 3.3 Define and version the JSON startup contract schema consumed by runtime-init.
- [x] 3.4 Persist startup contracts as operation artifacts and deployment-scoped snapshots.
- [x] 3.5 Replace shell entrypoints in service images with runtime-init binaries and wire contract paths into startup.
- [x] 3.6 Add runtime-init phase diagnostics surfaced through `vcpe status` and `vcpe logs` (including JSON output).

## 4. Topology, Allocation, And Catalog Integration

- [x] 4.1 Remove hardcoded customer/network tables from planning and host-network flows.
- [x] 4.2 Derive deterministic MAC/interface role assignments from typed manifest, catalog, and allocation state.
- [x] 4.3 Persist allocation artifacts with operation identity to support rollback and forensic status inspection.
- [x] 4.4 Add planner tests proving deterministic attachment and identity behavior across repeated applies.

## 5. Renderer Completion Across Documented Services

- [x] 5.1 Migrate runtime-critical rendering for all documented services to typed Go renderer interfaces.
- [x] 5.2 Remove runtime-critical bash/Python renderer ownership and prohibit fallback mutation behavior.
- [x] 5.3 Enforce fail-fast unsupported-renderer checks before runtime mutation.
- [x] 5.4 Add per-service render validation tests and representative integration coverage.

## 6. Profile Model Breaking Change

- [x] 6.1 Remove `.env` import/export compatibility command paths from Go profile management flows.
- [x] 6.2 Enforce canonical typed manifest profile model as the only supported import/export format.
- [x] 6.3 Add migration-focused errors and diagnostics for rejected legacy `.env` operations.
- [x] 6.4 Remove legacy compatibility snapshot behavior tied to `.env` round-trip workflows.

## 7. Migration Bundle And Rollback Contract

- [x] 7.1 Implement `vcpe` migration-bundle generation with versioned schema/tool metadata.
- [x] 7.2 Include canonical upgrade inputs plus rollback payload artifacts in each migration bundle.
- [x] 7.3 Implement migration-bundle validation and enforce upgrade preconditions in apply flows.
- [x] 7.4 Integrate bounded rollback to consume persisted operation/state artifacts when runtime-init or apply phases fail.

## 8. Reconciliation, State, And Failure Semantics

- [x] 8.1 Extend apply phase model to include startup-contract generation and runtime-init verification checkpoints.
- [x] 8.2 Treat runtime-init failures as terminal and trigger bounded rollback in reverse dependency order.
- [x] 8.3 Persist new operation journal fields for startup-contract version, runtime-init phase outcomes, and rollback metadata.
- [x] 8.4 Add regression tests for idempotent repeated apply and rollback behavior under startup failures.

## 9. Documentation, Smoke Gates, And Release Criteria

- [x] 9.1 Update README, runbook, and packaging docs to remove wrapper command guidance and `.env` compatibility usage.
- [x] 9.2 Document canonical `vcpe` and `vcpe service` workflows, migration-bundle requirements, and host-model prerequisites.
- [x] 9.3 Update smoke targets and integration jobs to require direct `vcpe` coverage for all documented service classes.
- [x] 9.4 Add release-gate checks that block shipment unless documented services pass integration and representative Podman smoke tests on new paths.
