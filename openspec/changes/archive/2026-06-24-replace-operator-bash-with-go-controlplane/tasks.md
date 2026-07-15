## 1. Command Surface And Binary Wiring

- [x] 1.1 Add shared Go command registration so `vcpe` and `vcpectl` execute the same implementation
- [x] 1.2 Add Go public commands for `init`, `build`, `up`, `down`, `logs`, `config`, and `profile` without removing existing `plan`, `apply`, `status`, and `destroy`
- [x] 1.3 Implement `up` and `down` as Go lifecycle aliases over apply/destroy semantics with control-plane locking and state usage
- [x] 1.4 Add human output defaults and `--json` output support for status-like command results
- [x] 1.5 Update packaging and build references so `vcpe` is the primary user-facing binary and `vcpectl` remains an alias/debug path

## 2. Profile Store And Compatibility Translation

- [x] 2.1 Implement Go profile store loading for defaults, built-in profiles, user profiles, and active-profile selection without shell sourcing
- [x] 2.2 Implement `vcpe config show` and `vcpe config set` against the Go config/profile store
- [x] 2.3 Implement `vcpe profile list/show/use/create/set` against the Go profile store
- [x] 2.4 Implement legacy env profile import to canonical manifest translation with structured `ProfileTranslationWarning` values
- [x] 2.5 Implement profile export snapshots under versioned control-plane state paths while preserving supported field round-trip behavior
- [x] 2.6 Add golden tests for built-in profiles and supported `.env` import/export round trips

## 3. Service Catalog And Planning Models

- [x] 3.1 Add functional `ServiceCatalog`, `ServiceImage`, `ServiceDependency`, `RenderContract`, `ComposeGroup`, and `HostNetworkIntent` models
- [x] 3.2 Add catalog entries for smoke-gated BNG `bng-7` and `bng-20` paths with image, dependency, render, compose, health, log, and host-network metadata
- [x] 3.3 Wire catalog dependency ordering into planner output for start, health, teardown, and rollback order
- [x] 3.4 Ensure planner output reports unsupported non-migrated service paths before runtime mutation
- [x] 3.5 Add unit tests for catalog validation, dependency ordering, and unsupported service planning

## 4. Image Lifecycle And Podman Backend

- [x] 4.1 Add typed Podman backend methods for image existence, build, pull, push, and tag operations
- [x] 4.2 Implement Go image manager behavior for explicit `vcpe build` and policy-driven image actions during `up`/`apply`
- [x] 4.3 Implement build-if-missing behavior only when the selected service image policy permits it
- [x] 4.4 Add operation journal entries and structured errors for image lifecycle phases
- [x] 4.5 Add unit tests for image policy decisions and backend command construction

## 5. Render, Compose, And Host-Network Adapters

- [x] 5.1 Add typed renderer interface and artifact validation contract
- [x] 5.2 Implement real adapter-backed rendering for smoke-gated `bng-7` and `bng-20` paths
- [x] 5.3 Add typed compose adapter that invokes `podman-compose` with Go-owned project names, generated inputs, timeouts, journal entries, and rollback metadata
- [x] 5.4 Add host-network adapter with bridge, NAT, firewall, and capability preflight boundaries
- [x] 5.5 Ensure apply fails before mutation when required host-network capabilities or renderer adapters are unavailable
- [x] 5.6 Store generated manifests, rendered outputs, operation journals, and compatibility snapshots under versioned control-plane state paths
- [x] 5.7 Add golden render tests for BNG smoke paths and unit tests for compose/host-network adapter preflight behavior

## 6. Lifecycle, Status, Logs, And Rollback Integration

- [x] 6.1 Wire `vcpe up`/`apply` through profile translation, catalog planning, image lifecycle, render, compose, health, and state persistence
- [x] 6.2 Wire `vcpe down`/`destroy` through catalog teardown and reverse-order rollback/cleanup semantics
- [x] 6.3 Implement `vcpe status` desired-vs-planned-vs-observed reporting with human and JSON output
- [x] 6.4 Implement `vcpe logs` with service/customer filters, catalog log selectors, and separate operation journal context
- [x] 6.5 Enforce disruptive-change approval for CIDR changes, identity reset, volume remap, and scale-to-zero before mutation
- [x] 6.6 Add unit and integration tests for lifecycle phase ordering, rollback ordering, disruptive guards, status, and logs

## 7. Script Shim Migration

- [x] 7.1 Refactor `scripts/vcpe` into an argument-translation shim that invokes the Go `vcpe` command and propagates exit codes
- [x] 7.2 Refactor `scripts/bng`, `scripts/gateway`, `scripts/routerd`, `scripts/webpa`, `scripts/xb10`, `scripts/client`, and `scripts/net` into compatibility shims
- [x] 7.3 Remove profile sourcing and direct runtime mutation behavior from script shims
- [x] 7.4 Add command-contract tests proving documented script paths delegate to Go commands and preserve expected exit behavior
- [x] 7.5 Add deprecation/migration warnings for script paths where appropriate for the one-release compatibility window

## 8. Documentation, Packaging, And Verification

- [x] 8.1 Update README, runbook, Makefile usage, and packaging docs to identify `vcpe` as the primary command and scripts as compatibility shims
- [x] 8.2 Update smoke tests or add new smoke tests for Go primary command paths while preserving compatibility script coverage
- [x] 8.3 Run Go unit tests for control-plane packages
- [x] 8.4 Run golden profile/render tests
- [x] 8.5 Run command-contract tests for script shims
- [x] 8.6 Run Podman smoke tests for representative `bng-7` and `bng-20` flows when Podman is available
- [x] 8.7 Review generated spec coverage against implementation and record any deferred follow-up items for daemon/watch, native compose reconciliation, and full renderer rewrites
