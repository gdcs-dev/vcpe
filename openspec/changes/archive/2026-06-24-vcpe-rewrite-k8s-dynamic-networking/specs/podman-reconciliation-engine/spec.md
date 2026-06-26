## ADDED Requirements

### Requirement: Idempotent reconcile and apply pipeline
The system SHALL execute apply through deterministic phases and converge to desired state when the same manifest is applied repeatedly.

#### Scenario: Repeated apply is convergent
- **WHEN** `apply` is run multiple times against unchanged desired state
- **THEN** the system reports no additional mutations after initial convergence

### Requirement: Fail-fast rollback with durable operation journal
The system MUST stop on terminal phase failure, record phase outcomes durably, and attempt bounded rollback for resources created in the current operation.

#### Scenario: Apply failure triggers bounded rollback
- **WHEN** a container lifecycle phase fails after network allocation succeeds
- **THEN** the system records failure details and executes compensating rollback for resources created by that operation
