## MODIFIED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for customer deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, `config`, and `profile` commands as Go-owned operator commands rather than bash-owned behavior. The system SHALL provide first-class service-scoped workflows through `vcpe service <name> ...` for documented services.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

#### Scenario: Go operator owns public command behavior
- **WHEN** an operator runs `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `profile`, or `service <name>`
- **THEN** the command is handled by the Go operator command surface and uses control-plane validation, state, and output contracts

### Requirement: Primary Go operator binary
The system SHALL package `vcpe` as the only supported user-facing operator command path for deployment and service lifecycle workflows.

#### Scenario: User invokes primary operator
- **WHEN** a user runs `vcpe status`
- **THEN** the command executes the Go operator implementation used by the control-plane command path

#### Scenario: Wrapper command path is rejected
- **WHEN** a user invokes a removed legacy wrapper command path
- **THEN** the system fails with an actionable message that points to the equivalent `vcpe` command

## REMOVED Requirements

### Requirement: Script compatibility shims
**Reason**: The compatibility-window wrapper layer is intentionally removed in this release to complete command-surface ownership transfer to Go.
**Migration**: Replace wrapper invocations with direct `vcpe` commands, including `vcpe service <name> ...` for service-scoped workflows.