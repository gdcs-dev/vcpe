## MODIFIED Requirements

### Requirement: Idempotent reconcile and apply pipeline
The system SHALL execute apply through deterministic phases and converge to desired state when the same manifest is applied repeatedly. The phases SHALL include service catalog planning, host-network preflight, image lifecycle decisions, typed rendering, compose group application, health verification, status inspection, and generated artifact state recording.

#### Scenario: Repeated apply is convergent
- **WHEN** `apply` is run multiple times against unchanged desired state
- **THEN** the system reports no additional mutations after initial convergence

#### Scenario: Apply uses catalog-driven phases
- **WHEN** an operator applies a profile-backed deployment through `vcpe up`
- **THEN** the plan includes service catalog ordering, required image actions, render artifacts, compose groups, host-network intents, and health checks before runtime mutation begins

### Requirement: Fail-fast rollback with durable operation journal
The system MUST stop on terminal phase failure, record phase outcomes durably, and attempt bounded rollback for resources created in the current operation. Rollback order MUST follow the reverse of the planner's successfully applied service and resource order.

#### Scenario: Apply failure triggers bounded rollback
- **WHEN** a container lifecycle phase fails after network allocation succeeds
- **THEN** the system records failure details and executes compensating rollback for resources created by that operation

#### Scenario: Rollback follows reverse plan order
- **WHEN** a compose group fails after dependent resources were created
- **THEN** the system rolls back current-operation resources in reverse dependency order and records the rollback result in the operation journal

## ADDED Requirements

### Requirement: Typed service catalog planning
The system SHALL plan service lifecycle actions from a typed service catalog that defines service metadata, dependencies, image policy, render contracts, compose groups, health checks, and log selectors.

#### Scenario: Service ordering is deterministic
- **WHEN** a deployment enables multiple services with catalog dependencies
- **THEN** the planner emits deterministic start, health, stop, and rollback order without relying on bash command order

### Requirement: Go-owned image lifecycle
The system SHALL manage build, pull, push, tag, and image existence decisions through Go image lifecycle components backed by typed Podman operations.

#### Scenario: Up builds missing image by policy
- **WHEN** `vcpe up` requires a missing local image and the selected image policy allows build-if-missing behavior
- **THEN** the system builds the image before applying the dependent compose group

### Requirement: Typed compose adapter
The system SHALL apply compose-backed services through a typed compose adapter that owns generated inputs, project names, command timeouts, operation journal entries, status inspection, and rollback bookkeeping.

#### Scenario: Compose command is journaled
- **WHEN** the operator applies a compose group
- **THEN** the system records the compose group identity, project name, generated input paths, phase result, and rollback eligibility in the operation journal

### Requirement: Host-network preflight
The system MUST preflight bridge, NAT, firewall, and required host capabilities before mutating runtime resources, and MUST fail before mutation if required capabilities are unavailable.

#### Scenario: Missing host capability blocks apply
- **WHEN** a deployment requires privileged host networking and the current environment lacks required capabilities
- **THEN** apply fails during preflight with an actionable error and no runtime resources are mutated

### Requirement: Versioned generated artifact state
The system SHALL store generated manifests, rendered files, operation journals, and compatibility snapshots under versioned control-plane state paths.

#### Scenario: Generated artifacts are state-scoped
- **WHEN** the operator renders artifacts for an apply operation
- **THEN** the artifacts are written under the selected control-plane state root with a versioned operation or deployment path
