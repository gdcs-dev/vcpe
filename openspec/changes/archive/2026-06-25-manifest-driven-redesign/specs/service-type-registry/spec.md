## ADDED Requirements

### Requirement: Behavior-only service type registry
The system SHALL maintain a compile-time service type registry that maps each service `type` to behavior-only metadata: a config validator, a renderer, expected host-network roles, and a default image policy. The registry MUST NOT contain any deployment-, customer-, or instance-derived data. A service `type` is "supported" if and only if it is registered with a validator, a renderer, and expected roles.

#### Scenario: Registered type is supported
- **WHEN** a manifest declares a service whose `type` is registered
- **THEN** preflight resolves the type's validator, renderer, expected roles, and default image policy from the registry

#### Scenario: Unregistered type is rejected before mutation
- **WHEN** a manifest declares a service whose `type` is not registered
- **THEN** preflight fails with an unsupported-type error identifying the service and `type` before any runtime mutation

### Requirement: Strict typed config decoding per type
The system SHALL decode each service's `config` block strictly against the schema registered for its `type`, and MUST reject unknown or malformed config fields before planning or apply.

#### Scenario: Unknown config field is rejected
- **WHEN** a service `config` contains a field not defined by its registered type schema
- **THEN** validation fails with an error identifying the offending field before planning or apply

#### Scenario: Type-specific config is validated
- **WHEN** a service `config` is decoded for a registered type
- **THEN** the registered validator checks the typed fields and rejects invalid values before runtime mutation

### Requirement: Expected host-network roles per type
The system SHALL define, per registered type, the host-network interface roles the type requires, and MUST reject deployments whose declared interfaces do not satisfy a type's expected roles.

#### Scenario: Unmet expected roles are rejected
- **WHEN** a service of a registered type declares interfaces that do not satisfy the type's expected roles
- **THEN** validation fails identifying the service, `type`, and missing role before runtime mutation
