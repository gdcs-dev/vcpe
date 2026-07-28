## ADDED Requirements

### Requirement: service command group
The system SHALL expose a `service` top-level command group in the `vcpe` CLI, consistent with the existing `manifest` command group. The `service` group SHALL support `-h`/`--help` to display structured help text and exit 0.

#### Scenario: vcpe service --help exits zero
- **WHEN** an operator runs `vcpe service --help`
- **THEN** the system prints the `service` command group help text and exits 0

#### Scenario: vcpe service with no subcommand errors with help hint
- **WHEN** an operator runs `vcpe service` with no subcommand
- **THEN** the system exits non-zero with an error message naming the available subcommands
