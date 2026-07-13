## MODIFIED Requirements

### Requirement: Typed rendering contract
The system SHALL render runtime configuration artifacts from validated typed inputs rather than unbounded regex substitutions. The rendering contract SHALL be implemented through typed renderer interfaces, and temporary subprocess renderers MUST receive structured arguments and validate expected outputs.

#### Scenario: Rendering fails on invalid typed input
- **WHEN** validated input is missing a required template value
- **THEN** rendering fails with a structured error before runtime mutation

#### Scenario: Renderer adapter validates outputs
- **WHEN** a temporary renderer adapter completes successfully
- **THEN** the system verifies the expected artifact paths and metadata before proceeding to runtime mutation

### Requirement: Secret reference resolution and redaction
The system MUST support phase-1 secret providers `env` and `file`, resolve secrets at apply time only, and MUST NOT persist secret values in state or logs.

#### Scenario: Missing secret reference blocks apply
- **WHEN** a `secretRef` points to a non-existent key in the selected provider
- **THEN** apply fails with a non-redacted reference identifier and without logging secret payloads

## ADDED Requirements

### Requirement: First-pass BNG renderer support
The system SHALL provide real typed renderer adapter support for smoke-gated `bng-7` and `bng-20` deployment paths in the first implementation slice.

#### Scenario: BNG smoke profile renders through typed adapter
- **WHEN** an operator plans or applies a smoke-gated `bng-7` or `bng-20` deployment
- **THEN** the renderer produces validated runtime artifacts through the typed render contract before container mutation

### Requirement: Unsupported renderer paths fail before mutation
The system MUST fail with a clear unsupported-renderer error before runtime mutation when a requested service path has not been migrated to a typed renderer or explicit adapter.

#### Scenario: Non-migrated renderer is blocked
- **WHEN** a deployment requires a service renderer with no typed renderer or approved adapter
- **THEN** planning or preflight fails with the service name, profile/customer context, and unsupported-renderer reason
