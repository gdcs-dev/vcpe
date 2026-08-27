## ADDED Requirements

### Requirement: Expected host-network roles per type — routerd
The `routerd` service type SHALL declare expected host-network interface roles: `wan` (required). Additional roles (e.g. `lan-p1`..`lan-pN`, `cm`) referenced by the service's declared `interfaces[]` MAY be present without being individually enumerated by the type. Preflight SHALL reject a routerd service declaration that does not include an interface with role `wan`.

#### Scenario: Routerd without wan interface is rejected
- **WHEN** a manifest declares a service of type `routerd` with no interface referencing role `wan`
- **THEN** preflight fails with an error identifying the service and the missing required role `wan`

#### Scenario: Routerd with wan interface passes role check
- **WHEN** a manifest declares a service of type `routerd` with an interface referencing role `wan`
- **THEN** preflight passes the expected-roles check for that service
