## MODIFIED Requirements

### Requirement: Idempotent reconcile and apply pipeline
The system SHALL execute apply through deterministic phases and converge to desired state when the same manifest is applied repeatedly. The phases SHALL include service catalog planning, host-network preflight, image lifecycle decisions, typed rendering, startup-contract generation, compose group application, runtime-init verification, health verification, status inspection, and generated artifact state recording.

#### Scenario: Repeated apply is convergent
- **WHEN** `apply` is run multiple times against unchanged desired state
- **THEN** the system reports no additional mutations after initial convergence

#### Scenario: Apply uses catalog-driven phases
- **WHEN** an operator applies a profile-backed deployment through `vcpe up`
- **THEN** the plan includes service catalog ordering, required image actions, render artifacts, startup contracts, compose groups, host-network intents, and health checks before runtime mutation begins

### Requirement: Fail-fast rollback with durable operation journal
The system MUST stop on terminal phase failure, record phase outcomes durably, and attempt bounded rollback for resources created in the current operation. Rollback order MUST follow the reverse of the planner's successfully applied service and resource order. Runtime-init contract validation or startup phase failures MUST be treated as terminal deployment failures.

#### Scenario: Apply failure triggers bounded rollback
- **WHEN** a container lifecycle phase fails after network allocation succeeds
- **THEN** the system records failure details and executes compensating rollback for resources created by that operation

#### Scenario: Rollback follows reverse plan order
- **WHEN** a compose group fails after dependent resources were created
- **THEN** the system rolls back current-operation resources in reverse dependency order and records the rollback result in the operation journal

#### Scenario: Runtime-init failure triggers rollback
- **WHEN** runtime-init fails after container start but before service convergence
- **THEN** apply fails the operation and executes bounded rollback using recorded operation artifacts

### Requirement: Host-network preflight
The system MUST preflight bridge, NAT, firewall, and required host capabilities before mutating runtime resources, and MUST fail before mutation if required capabilities are unavailable. Host-network mutation MUST be executed by Go-owned controller logic and MUST NOT fall back to shell-owned mutation paths.

#### Scenario: Missing host capability blocks apply
- **WHEN** a deployment requires privileged host networking and the current environment lacks required capabilities
- **THEN** apply fails during preflight with an actionable error and no runtime resources are mutated

#### Scenario: Shell fallback is prohibited
- **WHEN** host-network preflight or mutation encounters unsupported conditions
- **THEN** the operation fails with actionable diagnostics instead of invoking shell mutation scripts

### Requirement: Versioned generated artifact state
The system SHALL store generated manifests, rendered files, startup contracts, operation journals, and compatibility snapshots under versioned control-plane state paths.

#### Scenario: Generated artifacts are state-scoped
- **WHEN** the operator renders artifacts for an apply operation
- **THEN** the artifacts are written under the selected control-plane state root with a versioned operation or deployment path

#### Scenario: Startup contract artifacts are persisted
- **WHEN** startup contracts are generated for service instances
- **THEN** the contracts are persisted as operation artifacts and deployment-scoped snapshots for status, rollback, and diagnostics