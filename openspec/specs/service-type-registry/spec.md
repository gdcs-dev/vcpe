## Purpose
Define the compile-time service type registry that maps each service `type` to behavior-only metadata (config validator, renderer, expected host-network roles, and default image policy) used to validate, plan, and render deployments.

## Requirements

### Requirement: Behavior-only service type registry
The system SHALL maintain a compile-time service type registry that maps each service `type` to behavior and metadata: a config validator, a renderer, an interface validator, expected host-network roles, a health behavior declaration, a human-readable description, a default OCI image repository, and a default image policy. The registry MUST NOT contain any deployment-, customer-, or instance-derived data. A service `type` is "supported" if and only if it is registered with a validator, a renderer, an interface validator, expected roles, health behavior metadata, a description, and a default image policy.

#### Scenario: Registered type is supported
- **WHEN** a manifest declares a service whose `type` is registered
- **THEN** preflight resolves the type's validator, renderer, interface validator, expected roles, health behavior metadata, description, default image policy, and default image from the registry

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

### Requirement: Expected host-network roles per type — bng
The `bng` service type SHALL declare expected host-network interface roles: `wan` (required), `cm` (optional), `mgmt` (optional). Preflight SHALL reject a BNG service declaration that does not include an interface with role `wan`.

#### Scenario: BNG without wan interface is rejected
- **WHEN** a manifest declares a service of type `bng` with no interface referencing role `wan`
- **THEN** preflight fails with an error identifying the service and the missing required role `wan`

#### Scenario: BNG with wan interface passes role check
- **WHEN** a manifest declares a service of type `bng` with an interface referencing role `wan`
- **THEN** preflight passes the expected-roles check for that service

### Requirement: Expected host-network roles per type — gateway
The `gateway` service type SHALL declare expected host-network interface roles: `wan` (required), `cm` (optional), `lan-p1` (optional). Preflight SHALL reject a gateway service declaration that does not include an interface with role `wan`.

#### Scenario: Gateway without wan interface is rejected
- **WHEN** a manifest declares a service of type `gateway` with no interface referencing role `wan`
- **THEN** preflight fails with an error identifying the service and the missing required role `wan`

#### Scenario: Gateway with wan interface passes role check
- **WHEN** a manifest declares a service of type `gateway` with an interface referencing role `wan`
- **THEN** preflight passes the expected-roles check for that service
