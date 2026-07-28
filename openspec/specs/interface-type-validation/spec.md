## Purpose
Define the per-interface validation contract for service types, allowing each registered type to enforce its own interface constraints at preflight.

## Requirements

### Requirement: Service types can validate per-interface constraints
The `ServiceType` interface SHALL define `ValidateInterfaces(interfaces []manifest.Interface) error`. Preflight SHALL call this method for each service and reject deployment when it returns an error. Service types that have no interface constraints SHALL return nil.

#### Scenario: ValidateInterfaces is called for every service at preflight
- **WHEN** `vcpe up` processes a manifest
- **THEN** preflight calls `ValidateInterfaces` for each service's registered type with the service's declared interfaces before any runtime mutation

#### Scenario: Type with no constraints returns nil
- **WHEN** a service type's `ValidateInterfaces` is called
- **THEN** it returns nil if the type imposes no per-interface requirements

#### Scenario: Gateway rejects interfaces without device names
- **WHEN** a gateway service declares an interface without a `device` field
- **THEN** `ValidateInterfaces` returns an error identifying the interface role that is missing a device name, and preflight fails before runtime mutation

#### Scenario: ValidateInterfaces error is surfaced before apply
- **WHEN** a gateway manifest is missing device names on any interface
- **THEN** `vcpe plan` and `vcpe up` both fail with the error message from `ValidateInterfaces` without modifying runtime state
