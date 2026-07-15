## 1. Control Plane Scaffolding

- [x] 1.1 Create Go module and command skeleton for `plan`, `apply`, `status`, and `destroy`
- [x] 1.2 Add shared config loading, state path resolution, and command wiring for CLI-first mode
- [x] 1.3 Implement optional local daemon bootstrap and local-socket client delegation

## 2. Manifest Model and Validation

- [x] 2.1 Define versioned manifest schemas for Profile, Customer Deployment, and Runtime Status
- [x] 2.2 Implement schema validation with explicit unsupported-version errors
- [x] 2.3 Implement cross-object validation for required services, scale caps, and policy constraints

## 3. Topology Planner and IPAM

- [x] 3.1 Implement logical role-to-network attachment planner (`mgmt`, `wan`, `cm`, `lan-p*`)
- [x] 3.2 Implement IPv4/IPv6 pool allocation, lease tracking, and overlap conflict detection
- [x] 3.3 Add deterministic planner outputs and conflict diagnostics to `plan` results

## 4. Reconciliation Engine and State Journal

- [x] 4.1 Implement SQLite schema for desired snapshots, operation journal, IPAM leases, and checkpoints
- [x] 4.2 Implement single-writer lock semantics across CLI and daemon execution authority modes
- [x] 4.3 Implement apply phase pipeline (preflight, allocation, render, lifecycle, health verify)
- [x] 4.4 Implement fail-fast behavior with bounded rollback for resources created in current operation
- [x] 4.5 Implement startup recovery replay for unfinished operations

## 5. Podman Runtime Integration

- [x] 5.1 Implement Podman backend adapters for networks, containers, and volume operations
- [x] 5.2 Add bounded local scaling controls (replica/customer caps and concurrency defaults)
- [x] 5.3 Implement disruptive-change classification and enforce `--allow-disruptive` gating

## 6. Rendering and Secrets

- [x] 6.1 Define typed rendering input models and migrate template rendering entrypoints
- [x] 6.2 Implement phase-1 secret providers (`env`, `file`) and `secretRef` resolution
- [x] 6.3 Add log/state redaction guarantees so secret payloads are never persisted

## 7. Compatibility Refactors [BREAKING]

- [x] 7.1 Refactor `scripts/vcpe` to route orchestration through control-plane commands
- [x] 7.2 Refactor service wrappers (`scripts/bng`, `scripts/gateway`, `scripts/routerd`, `scripts/webpa`, `scripts/xb10`, `scripts/client`) for compatibility mode
- [x] 7.3 Add env profile import/export compatibility commands replacing direct env-only orchestration assumptions
- [x] 7.4 Update `platform/scripts/lib/common.sh` integration points for new state/config paths and control-plane invocation

## 8. Observability and Operator UX

- [x] 8.1 Implement structured JSON logging with operation and resource correlation fields
- [x] 8.2 Expose core metrics (reconcile duration/failures, IPAM usage, drift count)
- [x] 8.3 Implement `status` timeline and drift summary output

## 9. Testing and Rollout

- [x] 9.1 Add unit tests for schema validation, planner determinism, and disruptive-change classification
- [x] 9.2 Add Podman integration tests for lifecycle, network allocation, rollback, and recovery replay
- [x] 9.3 Update smoke flows to execute through new CLI contract for representative profile classes
- [x] 9.4 Document migration/rollback runbook updates for local developers
