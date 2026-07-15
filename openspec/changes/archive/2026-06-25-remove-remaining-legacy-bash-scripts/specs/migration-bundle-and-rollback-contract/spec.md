## ADDED Requirements

### Requirement: Versioned migration bundle generation
The system SHALL provide a Go-owned command path to generate a single versioned migration bundle per deployment/customer containing upgrade inputs and rollback payload artifacts.

#### Scenario: Migration bundle includes upgrade and rollback payload
- **WHEN** an operator generates a migration bundle before upgrading
- **THEN** the resulting bundle contains canonical upgrade inputs, startup-contract snapshots, and rollback payload artifacts for the selected deployment/customer

#### Scenario: Migration bundle metadata is versioned
- **WHEN** a migration bundle is generated
- **THEN** the bundle includes explicit schema and tool-version metadata required for validation and compatibility checks

### Requirement: Migration bundle validation gate
The system MUST provide validation for migration bundle integrity and compatibility, and apply MUST reject upgrade flows that require bundles when validation fails.

#### Scenario: Invalid bundle blocks upgrade apply
- **WHEN** an operator applies an upgrade with an invalid or incompatible migration bundle
- **THEN** apply fails before runtime mutation with actionable validation errors

#### Scenario: Missing required bundle blocks upgrade apply
- **WHEN** an operator applies an upgrade path that requires a migration bundle but does not provide one
- **THEN** apply fails with guidance to generate the required bundle using the canonical `vcpe` migration command