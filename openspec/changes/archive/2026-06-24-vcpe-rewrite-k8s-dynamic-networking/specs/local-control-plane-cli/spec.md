## ADDED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for customer deployments.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

### Requirement: Safe destructive operation guard
The system SHALL require explicit user confirmation or force flag semantics before destroying an active deployment.

#### Scenario: Destroy blocked without explicit confirmation
- **WHEN** an operator runs `destroy` without confirmation and without force override
- **THEN** the system refuses to remove runtime resources and returns a guardrail error
