## ADDED Requirements

### Requirement: Versioned manifest schema
The system SHALL define a versioned manifest schema that separates Profile defaults, Customer Deployment desired state, and Runtime Status observed state.

#### Scenario: Manifest version is validated
- **WHEN** a manifest with unsupported schema version is submitted
- **THEN** validation fails before planning or apply

### Requirement: Cross-object validation
The system MUST validate required-service constraints, scaling caps, and network intent consistency across manifest objects.

#### Scenario: Invalid cross-object constraints are rejected
- **WHEN** a deployment references a profile that disallows the requested service scale
- **THEN** the system returns a validation error identifying the violated profile constraint
