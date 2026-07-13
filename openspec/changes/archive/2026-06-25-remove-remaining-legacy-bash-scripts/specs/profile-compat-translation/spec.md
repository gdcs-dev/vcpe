## MODIFIED Requirements

### Requirement: Go-owned profile and config commands
The system SHALL implement profile and config management commands in Go, including profile listing, showing, selection, creation, mutation, import, export, and config show/set behavior. The canonical typed manifest model SHALL be the only supported profile import/export format.

#### Scenario: Profile use updates active profile through Go
- **WHEN** an operator runs `vcpe profile use <name>`
- **THEN** the Go profile store records the active profile using the existing user config location and returns a structured success or error result

#### Scenario: Legacy env import is rejected
- **WHEN** an operator requests import of a legacy env-style profile
- **THEN** the command fails with an actionable migration error describing canonical manifest conversion requirements

## REMOVED Requirements

### Requirement: Legacy profile import to canonical manifests
**Reason**: Legacy `.env` compatibility is intentionally removed in this release so canonical typed manifests are the only supported operator profile model.
**Migration**: Convert existing `.env` profiles to canonical manifests before upgrade using the migration bundle tooling and documented migration workflow.

### Requirement: Round-trip compatibility reporting
**Reason**: Round-trip env compatibility behavior is removed together with `.env` import/export support.
**Migration**: Use canonical manifest validation and migration-bundle checks to identify unsupported legacy fields before upgrade.

### Requirement: Compatibility snapshots
**Reason**: Snapshot semantics tied to `.env` compatibility exports are removed.
**Migration**: Use versioned migration bundles and deployment/operation artifacts as the supported compatibility and rollback snapshot mechanism.