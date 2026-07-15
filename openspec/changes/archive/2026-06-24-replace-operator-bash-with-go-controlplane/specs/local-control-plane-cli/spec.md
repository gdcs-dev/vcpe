## MODIFIED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for customer deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, `config`, and `profile` commands as Go-owned operator commands rather than bash-owned behavior.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

#### Scenario: Go operator owns public command behavior
- **WHEN** an operator runs `init`, `build`, `up`, `down`, `status`, `logs`, `config`, or `profile`
- **THEN** the command is handled by the Go operator command surface and uses control-plane validation, state, and output contracts

### Requirement: Safe destructive operation guard
The system SHALL require explicit user confirmation or force flag semantics before destroying an active deployment, and SHALL require explicit disruptive-change approval before applying changes that alter CIDRs, reset identities, remap volumes, or scale active services to zero.

#### Scenario: Destroy blocked without explicit confirmation
- **WHEN** an operator runs `destroy` without confirmation and without force override
- **THEN** the system refuses to remove runtime resources and returns a guardrail error

#### Scenario: Disruptive apply blocked without approval
- **WHEN** an operator applies a manifest that requires a disruptive change without `--allow-disruptive` or explicit confirmation
- **THEN** the system refuses to mutate runtime resources and returns a plan summary identifying the disruptive change

## ADDED Requirements

### Requirement: Primary Go operator binary
The system SHALL package `vcpe` as the primary user-facing Go operator command and SHALL keep `vcpectl` available as an alias or debug path during the compatibility window.

#### Scenario: User invokes primary operator
- **WHEN** a user runs `vcpe status`
- **THEN** the command executes the same Go operator implementation used by the control-plane command path

### Requirement: Script compatibility shims
The system SHALL preserve existing documented script paths as compatibility shims for one release window after Go parity, and those shims MUST translate arguments to Go commands without owning deployment behavior.

#### Scenario: Script shim delegates to Go
- **WHEN** a user runs a documented `./scripts/vcpe` or service script command during the compatibility window
- **THEN** the script invokes the Go operator command, propagates its exit code, and does not source profiles or mutate runtime resources directly

### Requirement: Human and JSON output contracts
The system SHALL provide human-readable output by default for operator commands that report state, and SHALL provide stable JSON output when `--json` is requested.

#### Scenario: Status supports automation
- **WHEN** an operator runs `vcpe status --json`
- **THEN** the system emits machine-readable desired, planned, and observed state without relying on human formatting
