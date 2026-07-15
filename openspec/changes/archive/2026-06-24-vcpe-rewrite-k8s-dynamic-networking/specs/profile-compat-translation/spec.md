## ADDED Requirements

### Requirement: Legacy profile import to canonical manifests
The system SHALL import supported legacy env profile fields into canonical manifest objects with explicit mapping rules.

#### Scenario: Legacy profile conversion succeeds
- **WHEN** an operator imports a supported env profile
- **THEN** the system emits canonical manifest output that passes schema and policy validation

### Requirement: Round-trip compatibility reporting
The system MUST report unsupported or lossy field mappings during import/export and SHALL preserve supported fields across round-trip conversion.

#### Scenario: Unsupported field is surfaced
- **WHEN** a legacy profile contains a field without canonical mapping
- **THEN** conversion completes with a structured warning that identifies the field and impact
