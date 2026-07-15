## MODIFIED Requirements

### Requirement: Typed rendering contract
The system SHALL render runtime configuration artifacts from validated typed inputs rather than unbounded regex substitutions. The rendering contract SHALL be implemented through typed renderer interfaces for all documented service paths, and runtime-critical rendering MUST NOT depend on bash or Python script ownership.

#### Scenario: Rendering fails on invalid typed input
- **WHEN** validated input is missing a required template value
- **THEN** rendering fails with a structured error before runtime mutation

#### Scenario: Documented services render through typed contract
- **WHEN** a deployment enables a documented service path
- **THEN** rendering produces validated runtime artifacts through the typed renderer contract before runtime mutation

### Requirement: First-pass BNG renderer support
The system SHALL provide typed renderer support for all documented service deployment paths (`bng`, `gateway`, `routerd`, `webpa`, `xb10`, and `client`) before this change ships.

#### Scenario: Representative service path renders successfully
- **WHEN** an operator plans or applies a supported documented service deployment
- **THEN** the renderer produces validated runtime artifacts through the typed render contract before container mutation

### Requirement: Unsupported renderer paths fail before mutation
The system MUST fail with a clear unsupported-renderer error before runtime mutation when a requested service path has not been migrated to a typed renderer.

#### Scenario: Non-migrated renderer is blocked
- **WHEN** a deployment requires a service renderer with no typed renderer
- **THEN** planning or preflight fails with the service name, profile/customer context, and unsupported-renderer reason