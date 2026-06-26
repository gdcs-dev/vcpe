## Overview

This change introduces a new declarative control plane for vCPE and decouples orchestration from the current Bash + env-file + hardcoded bridge topology model.

The target architecture supports:
- Dynamic scaling of service workloads (customer-scoped and shared services)
- Dynamic network mapping via topology planning and IP address management (IPAM)
- Structured configuration using YAML/JSON with schema versioning
- Single-runtime operation: Podman backend optimized for local development and testing

Architectural stance:
- Recommended: rewrite the control plane in Go.
- Supported migration path: keep existing service containers and progressively replace orchestration and rendering.
- Scaling scope: bounded single-host scaling only (not large multi-node orchestration).
- Non-goal in this phase: rewriting all service internals at once.

## Components

- **Control Plane API/CLI**: Single entry point for desired state submission, validation, planning, apply, status, and rollback operations.
- **Desired State Model**: Versioned YAML/JSON resource model for profiles, customers, services, scaling policy, and network intents.
- **Schema & Validation Engine**: Validates manifests, migration input, and cross-resource invariants before planning.
- **Profile Translation Layer**: Imports existing .env profiles and emits canonical desired-state manifests; supports export for backward compatibility during migration.
- **Topology Planner**: Converts logical attachment intents (mgmt, wan, cm, lan-p*) into concrete network graph and interface role assignments.
- **IPAM Service**: Allocates/leases/reclaims IPv4/IPv6 CIDRs, host addresses, and (where supported) MAC identities from declared pools.
- **Reconciler**: Declarative reconciliation loop that computes diffs, applies changes idempotently, tracks operation state, and handles rollback.
- **Podman Runtime Backend**: Single orchestration backend for container lifecycle, network attachments, and volume management on a single host.
- **Render Service**: Structured template rendering for BNG/routerd/etc configs from validated model data; replaces regex-driven replacement behavior.
- **State Store**: Durable operation and allocation state in SQLite.
- **Observability Pipeline**: Structured logs, metrics, and traces for planner, reconciler, backend operations, and service health.

## Key Architectural Decisions

### Control Plane Language
**Choice**: Use Go for the control plane core.
**Rationale**: Go provides strong distribution ergonomics (single static binaries), predictable runtime behavior for long-lived reconciliation loops, and lower operational friction than Python virtual environment management for this use case.
**Alternatives considered**:
- Python: Rejected as primary control plane language due to packaging/runtime management complexity at scale and weaker fit for long-running reconciler services.
- Keep Bash: Rejected because it cannot safely support declarative state, dynamic topology planning, and robust reconciliation semantics.

### Runtime Strategy (Breaking vs Non-Breaking)
**Choice**: Keep a single Podman backend and evolve orchestration semantics in-place.
**Rationale**: The primary goal is local development and testing. A single backend minimizes complexity, reduces operational overhead, and avoids cross-backend semantic drift.
**Alternatives considered**:
- Dual backend (Podman + Kubernetes): Rejected as unnecessary complexity for current scope.
- Immediate Kubernetes-only rewrite (breaking): Rejected for high migration risk and mismatch with local-first goals.

### Scaling Scope
**Choice**: Support bounded, single-host scaling with explicit limits.
**Rationale**: Local test environments need predictable resource usage, faster convergence, and straightforward debugging rather than large-cluster elasticity.
**Alternatives considered**:
- Unbounded autoscaling: Rejected due to resource contention and low value in local workflows.
- No scaling features at all: Rejected because controlled scale-up/down is still useful for behavior testing.

### Configuration Model Migration
**Choice**: Make versioned YAML/JSON manifests the source of truth; keep .env import/export compatibility during transition.
**Rationale**: Structured config enables schema enforcement, safer diffs, and explicit intent expression for networking and scaling.
**Alternatives considered**:
- Continue env-only profiles: Rejected due to lack of schema guarantees and poor extensibility.
- One-way env import only: Rejected because it complicates phased migration and rollback.

### Dynamic Networking Architecture
**Choice**: Introduce a topology planner + IPAM pair and remove hardcoded customer IDs/network lists.
**Rationale**: Dynamic customer onboarding and scaling require computed network mappings, deterministic interface role assignment, and conflict-free pool allocation.
**Alternatives considered**:
- Expand hardcoded maps (7/9/20 style): Rejected as non-scalable and error-prone.
- Backend-only ad hoc allocation: Rejected because it fragments logic and creates inconsistent behavior across runtimes.

### MAC and Interface Role Semantics
**Choice**: Preserve deterministic interface roles and fixed MAC conventions within the Podman networking model.
**Rationale**: Existing services and tests depend on stable interface identity; in a single-backend architecture we can preserve current semantics directly.
**Alternatives considered**:
- Dynamic role assignment without fixed MAC conventions: Rejected because service configs and tests rely on role stability.

### Reconciliation and State Management
**Choice**: Use a declarative reconciler with durable operation journal and idempotent apply phases.
**Rationale**: Prevents partial-apply drift and enables crash recovery, rollback, and predictable updates.
**Alternatives considered**:
- Imperative command chain: Rejected as brittle under retries/failures.
- Stateless apply: Rejected because network/IP allocations and recovery require durable state.

### Template Rendering Migration
**Choice**: Replace regex/template-replacement behavior with typed input models and explicit templates under validation.
**Rationale**: Current replacement model is brittle and hard to reason about; typed rendering improves correctness and auditability.
**Alternatives considered**:
- Keep current Python regex renderer indefinitely: Rejected due to maintenance and correctness risk.
- Rewrite all service configs in one step: Rejected as too risky for initial migration.

## Data Flow

Desired-state submission and reconciliation:

```text
User/Automation
    |
    v
Control Plane CLI/API
    |
    +--> Schema Validation ----> Reject early on invalid intent
    |
    v
Topology Planner <----> IPAM
    |
    v
Reconciler (diff current vs desired)
    |
  +--> Podman Backend ------> Local networks/containers/volumes
    |
    v
Render Service ----> Runtime config artifacts/secrets
    |
    v
Health + Status aggregation ----> Operation journal + metrics
```

Profile migration flow:

```text
Legacy .env Profile --> Translation Layer --> Canonical YAML/JSON Manifest
                                               |
                                               +--> Validate + Plan + Apply
                                               |
                                               +--> Optional .env export (compat mode)
```

Failure and rollback flow:

```text
Apply Phase N fails
    |
    v
Reconciler marks operation FAILED
    |
    +--> Executes compensating actions for completed phases
    |
    +--> Releases/reverts IPAM leases if needed
    |
    v
State store records terminal result + diagnostics
```

## Integration Points

- Existing service assets are preserved initially:
  - BNG, GATEWAY, routerd, webpa, xb10 container images and compose-era config semantics
  - Existing per-service templates and startup scripts, progressively replaced behind render contracts
- Existing scripts become compatibility wrappers:
  - `scripts/vcpe` and service scripts call new control plane commands in migration mode
- Networking layer integration:
  - Host bridge and firewall management abstracted behind Podman network providers
  - Dynamic NAT/firewall rules generated from planned topology instead of fixed CIDR assumptions
- Test integration:
  - Existing smoke tests continue through compatibility mode
  - New conformance tests validate planner/IPAM/reconciler invariants on Podman

## Security Model

Trust boundaries:
- Control plane boundary: API/CLI to planner/reconciler
- Runtime boundary: control plane to Podman API and host networking adapters
- Data boundary: manifests/state store/secrets/config artifacts

Security requirements:
- Authenticate and authorize control-plane operations with local ACL/policy suitable for developer environments.
- Treat secrets (credentials, keys, service tokens) as sensitive; never store in plaintext manifests.
- Use least privilege for backend operations; isolate privileged network/firewall actions into constrained adapters.
- Enforce manifest schema + policy admission checks to prevent unsafe network overlaps and privilege escalation.
- Maintain immutable audit trail for apply, scale, and rollback operations.

## Error Handling Strategy

Error model:
- Validation errors: fail-fast, non-retryable, user-actionable.
- Planner/IPAM conflicts: non-retryable until state/config changes.
- Backend transient errors: retry with exponential backoff and bounded attempts.
- Backend persistent errors: mark operation failed and trigger rollback policy.

Propagation and recovery:
- Every reconcile operation has a correlation ID and phase-level status.
- Apply is phase-based and idempotent; repeated execution converges to desired state.
- Rollback policy is explicit per resource class (network allocation, workload instance, config artifact).
- Orphan detection runs periodically to reconcile leaked resources.

## Observability Strategy

Logging:
- Structured JSON logs with `operation_id`, `resource_kind`, `resource_id`, `phase`, `backend`, and `customer_id`.
- Log levels: INFO for state transitions, WARN for degraded retries, ERROR for terminal failures.

Metrics:
- Reconcile duration and success rate
- Plan/apply/rollback counts
- IPAM allocation utilization and conflict rate
- Scaling events and convergence latency
- Service health/readiness outcomes

Tracing:
- Trace boundaries: API request -> validation -> planning -> backend apply -> health verification.
- Include backend API calls and render operations as child spans.

Health surfaces:
- Control-plane readiness/liveness endpoints
- Reconcile queue depth and oldest-operation age
- Per-customer desired vs actual drift indicators

## Constraints

- Must preserve existing service behavior during migration (BNG/GATEWAY/routerd/webpa/xb10/client).
- Must target local developer workflow and testing on a single host.
- Must not require immediate rewrite of all service containers.
- Must support IPv4 and IPv6 network planning.
- Must provide deterministic interface role mapping for service config correctness.
- Must remove hardcoded customer/network IDs from orchestration logic.
- Must support backward compatibility with existing env profiles during phased migration.
- Must implement durable state and idempotent reconciliation before enabling autoscaling.
- Must enforce bounded scaling limits to protect local machine resources.

## Diagrams

System shape:

```text
         +--------------------------+
         |    Control Plane (Go)    |
         |--------------------------|
         | API/CLI                  |
         | Validation               |
         | Topology Planner + IPAM  |
         | Reconciler               |
         +------------+-------------+
             |
             v
        +-----------------------+
        | Podman Backend        |
        | (single runtime)      |
        +-----------+-----------+
              |
              v
          Local containers/networks/volumes
              |
              v
            Service Runtime Layer
          (bng, gateway, routerd, webpa, xb10, client)
```

Sequence (scale up customer workload):

```text
User -> API/CLI: scale customer=42 bng replicas=3
API/CLI -> Validation: validate manifest + policy
Validation -> Planner/IPAM: compute network + address deltas
Planner/IPAM -> Reconciler: plan ready
Reconciler -> Backend: apply workload/network changes
Backend -> Render Service: generate per-instance config artifacts
Backend -> Reconciler: report status
Reconciler -> Health Checker: verify readiness
Health Checker -> Reconciler: healthy
Reconciler -> State Store: commit operation success
Reconciler -> User: converged
```

Sequence (dynamic customer onboarding):

```text
Manifest submit (customer=100)
  -> schema validation
  -> topology planning (attachments + role map)
  -> IPAM allocation (IPv4/IPv6, optional MAC where supported)
  -> backend resource creation
  -> render + deploy service configs
  -> readiness checks
  -> expose status and drift baseline
```
