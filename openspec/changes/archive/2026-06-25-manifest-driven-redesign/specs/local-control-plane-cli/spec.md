## MODIFIED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, and `config` commands as Go-owned operator commands rather than bash-owned behavior.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

#### Scenario: Go operator owns public command behavior
- **WHEN** an operator runs `init`, `build`, `up`, `down`, `status`, `logs`, or `config`
- **THEN** the command is handled by the Go operator command surface and uses control-plane validation, state, and output contracts

## ADDED Requirements

### Requirement: Deployment selection by name
The system SHALL identify a target deployment by `--name` (matching `metadata.name`) for the `down`, `destroy`, `logs`, `status`, and `service` commands.

#### Scenario: Command targets a deployment by name
- **WHEN** an operator runs `vcpe status --name <metadata.name>`
- **THEN** the command operates on the deployment whose `metadata.name` matches

#### Scenario: Unknown name is reported
- **WHEN** an operator runs a deployment command with a `--name` that matches no known deployment
- **THEN** the command fails with an error identifying the unknown name

### Requirement: State schema-version reset command
The system SHALL stamp the persisted state root with the `vcpe.dev/v1` schema version and MUST refuse to operate when the stamp is missing or mismatched on a non-empty state root, directing the operator to run an explicit `vcpe state reset`.

#### Scenario: Mismatched state is refused
- **WHEN** the persisted state root has data with a missing or non-`vcpe.dev/v1` schema stamp
- **THEN** the command fails with an actionable error instructing the operator to run `vcpe state reset`

#### Scenario: State reset clears legacy state
- **WHEN** an operator runs `vcpe state reset`
- **THEN** the system clears the state root and stamps it with the current schema version
