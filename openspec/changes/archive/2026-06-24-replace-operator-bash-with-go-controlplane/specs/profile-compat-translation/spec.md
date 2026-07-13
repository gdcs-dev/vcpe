## MODIFIED Requirements

### Requirement: Legacy profile import to canonical manifests
The system SHALL import supported legacy env profile fields into canonical manifest objects with explicit mapping rules, and SHALL parse env-style profile files without sourcing shell code.

#### Scenario: Legacy profile conversion succeeds
- **WHEN** an operator imports a supported env profile
- **THEN** the system emits canonical manifest output that passes schema and policy validation

#### Scenario: Profile import avoids shell execution
- **WHEN** a legacy env-style profile is imported
- **THEN** the system parses supported key-value fields as data and does not execute shell code from the profile file

### Requirement: Round-trip compatibility reporting
The system MUST report unsupported or lossy field mappings during import/export and SHALL preserve supported fields across round-trip conversion. Unsupported fields MUST produce structured warnings that include field name, severity, impact, and preservation status.

#### Scenario: Unsupported field is surfaced
- **WHEN** a legacy profile contains a field without canonical mapping
- **THEN** conversion completes with a structured warning that identifies the field and impact

#### Scenario: Supported field round-trips exactly
- **WHEN** a supported legacy profile field is imported to a canonical manifest and exported back to an env-style profile snapshot
- **THEN** the exported supported field value matches the original supported field value

## ADDED Requirements

### Requirement: Go-owned profile and config commands
The system SHALL implement profile and config management commands in Go, including profile listing, showing, selection, creation, mutation, import, export, and config show/set behavior.

#### Scenario: Profile use updates active profile through Go
- **WHEN** an operator runs `vcpe profile use <name>`
- **THEN** the Go profile store records the active profile using the existing user config location and returns a structured success or error result

### Requirement: Compatibility snapshots
The system SHALL write profile compatibility export snapshots under the versioned control-plane state root while preserving user profile files under existing config paths.

#### Scenario: Profile export creates state snapshot
- **WHEN** an operator exports a profile for compatibility review
- **THEN** the exported snapshot is written under the control-plane state root and the original user profile remains unchanged
