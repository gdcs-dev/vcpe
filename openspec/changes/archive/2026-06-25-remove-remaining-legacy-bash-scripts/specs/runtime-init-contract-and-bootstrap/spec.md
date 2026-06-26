## ADDED Requirements

### Requirement: Versioned startup contract artifacts
The system SHALL generate a versioned JSON startup contract for each service instance from typed planning and rendering outputs. The startup contract SHALL be rendered into runtime payload paths and SHALL be persisted as both operation-scoped artifacts and deployment-scoped state snapshots.

#### Scenario: Startup contract is generated per service instance
- **WHEN** apply prepares runtime artifacts for a service instance
- **THEN** the system writes a versioned startup contract JSON document for that instance into the runtime payload

#### Scenario: Startup contract is persisted for operations and deployment state
- **WHEN** startup contracts are generated during apply
- **THEN** the same contract content is persisted under operation and deployment-scoped state paths for rollback and diagnostics

### Requirement: Ordered runtime-init execution
The system SHALL execute runtime-init bootstrap phases in deterministic order: interface identity resolution, interface rename/verification, IPv6 readiness gating, runtime config application, bootstrap side effects, and service process exec.

#### Scenario: Runtime-init enforces required phase order
- **WHEN** a service container starts through runtime-init
- **THEN** phase execution follows the required order and records phase markers for observability

#### Scenario: Runtime-init stops on terminal phase error
- **WHEN** runtime-init encounters a terminal error in a bootstrap phase
- **THEN** the container exits non-zero with structured diagnostics that identify the failed phase

### Requirement: Shared bootstrap library with thin service binaries
The system SHALL implement runtime-init through a shared Go bootstrap library with thin per-service binaries and SHALL keep service-specific behavior limited to typed wiring inputs.

#### Scenario: Service bootstrap implementations remain behavior-consistent
- **WHEN** multiple documented services are migrated to runtime-init binaries
- **THEN** shared bootstrap semantics remain consistent across services while allowing service-specific typed configuration inputs