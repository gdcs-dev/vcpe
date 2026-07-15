## ADDED Requirements

### Requirement: Service-scoped command namespace
The system SHALL provide service-scoped workflows under `vcpe service <name> ...` for documented services and SHALL support lifecycle and diagnostic operations needed for partial service workflows.

#### Scenario: Service-scoped lifecycle command executes
- **WHEN** an operator runs a valid `vcpe service <name> <operation>` command for a documented service
- **THEN** the command executes through Go-owned control-plane validation, planning, and state flows

#### Scenario: Unknown service command is rejected
- **WHEN** an operator runs `vcpe service <name> ...` with an unsupported service name
- **THEN** the command fails with a structured error listing supported service names

### Requirement: First-class startup diagnostics in status and logs
The system SHALL expose runtime-init and startup-contract diagnostics as first-class per-service-instance data in `vcpe status` and `vcpe logs` outputs, including machine-readable output mode.

#### Scenario: Status reports startup contract diagnostics
- **WHEN** an operator runs `vcpe status --json`
- **THEN** each service instance includes startup-contract version and bootstrap phase status fields

#### Scenario: Logs include runtime-init phase diagnostics
- **WHEN** an operator runs `vcpe logs` for a service instance with bootstrap failure
- **THEN** logs include structured runtime-init phase markers and failure reason metadata