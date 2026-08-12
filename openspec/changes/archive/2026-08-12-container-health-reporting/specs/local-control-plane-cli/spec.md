## MODIFIED Requirements

### Requirement: Human and JSON output contracts
The system SHALL provide human-readable output by default for operator commands that report state, and SHALL provide stable JSON output when `--json` is requested. For `vcpe status --name <deployment>`, both output modes SHALL include live per-instance health observations collected over HTTP from the deployment's persisted health endpoints. The status health collection SHALL NOT invoke Podman, Compose, or container CLI commands.

#### Scenario: Status supports automation
- **WHEN** an operator runs `vcpe status --json`
- **THEN** the system emits machine-readable desired, planned, and observed state without relying on human formatting

#### Scenario: Named status includes live health JSON
- **WHEN** an operator runs `vcpe status --name <metadata.name> --json`
- **THEN** the output includes an observation for every expected service instance with its overall health state, named checks when available, and any transport or protocol error

#### Scenario: Named status includes live health for humans
- **WHEN** an operator runs `vcpe status --name <metadata.name>`
- **THEN** the output identifies each service replica and its live health observation