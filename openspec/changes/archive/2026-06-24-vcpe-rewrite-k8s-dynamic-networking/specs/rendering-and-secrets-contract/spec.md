## ADDED Requirements

### Requirement: Typed rendering contract
The system SHALL render runtime configuration artifacts from validated typed inputs rather than unbounded regex substitutions.

#### Scenario: Rendering fails on invalid typed input
- **WHEN** validated input is missing a required template value
- **THEN** rendering fails with a structured error before runtime mutation

### Requirement: Secret reference resolution and redaction
The system MUST support phase-1 secret providers `env` and `file`, resolve secrets at apply time only, and MUST NOT persist secret values in state or logs.

#### Scenario: Missing secret reference blocks apply
- **WHEN** a `secretRef` points to a non-existent key in the selected provider
- **THEN** apply fails with a non-redacted reference identifier and without logging secret payloads
