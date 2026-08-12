## MODIFIED Requirements

### Requirement: Behavior-only service type registry
The system SHALL maintain a compile-time service type registry that maps each service `type` to behavior and metadata: a config validator, a renderer, an interface validator, expected host-network roles, a health behavior declaration, a human-readable description, a default OCI image repository, and a default image policy. The registry MUST NOT contain any deployment-, customer-, or instance-derived data. A service `type` is "supported" if and only if it is registered with a validator, a renderer, an interface validator, expected roles, health behavior metadata, a description, and a default image policy.

#### Scenario: Registered type is supported
- **WHEN** a manifest declares a service whose `type` is registered
- **THEN** preflight resolves the type's validator, renderer, interface validator, expected roles, health behavior metadata, description, default image policy, and default image from the registry

#### Scenario: Unregistered type is rejected before mutation
- **WHEN** a manifest declares a service whose `type` is not registered
- **THEN** preflight fails with an unsupported-type error identifying the service and `type` before any runtime mutation