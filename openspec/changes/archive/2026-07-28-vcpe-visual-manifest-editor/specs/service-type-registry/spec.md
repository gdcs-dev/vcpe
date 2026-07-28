## MODIFIED Requirements

### Requirement: Behavior-only service type registry
The system SHALL maintain a compile-time service type registry that maps each service `type` to behavior and metadata: a config validator, a renderer, expected host-network roles, a default image policy, a human-readable description, and a default OCI image repository. The registry MUST NOT contain any deployment-, customer-, or instance-derived data. A service `type` is "supported" if and only if it is registered with a validator, a renderer, expected roles, a description, and a default image policy.

#### Scenario: Registered type is supported
- **WHEN** a manifest declares a service whose `type` is registered
- **THEN** preflight resolves the type's validator, renderer, expected roles, default image policy, description, and default image from the registry

#### Scenario: Unregistered type is rejected before mutation
- **WHEN** a manifest declares a service whose `type` is not registered
- **THEN** preflight fails with an unsupported-type error identifying the service and `type` before any runtime mutation

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
