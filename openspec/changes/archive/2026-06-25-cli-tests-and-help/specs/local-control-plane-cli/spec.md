## MODIFIED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, `config`, and `state` commands as Go-owned operator commands rather than bash-owned behavior. Every command SHALL support `-h`/`--help` to display structured help text and exit 0.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

#### Scenario: Go operator owns public command behavior
- **WHEN** an operator runs `init`, `build`, `up`, `down`, `status`, `logs`, `config`, or `state`
- **THEN** the command is handled by the Go operator command surface and uses control-plane validation, state, and output contracts

#### Scenario: Help flag exits zero on any command
- **WHEN** an operator runs `vcpe <command> --help` or `vcpe --help`
- **THEN** the system prints structured help text and exits with code 0 without executing the command

## ADDED Requirements

### Requirement: Test environment image skip
The system SHALL support a `VCPE_SKIP_IMAGE=1` environment variable that substitutes a no-op image backend for the real Podman backend in the `build` command and the `images` phase of `apply`, enabling unit tests to exercise the full command path without a container runtime.

#### Scenario: Build command runs without Podman when skip is set
- **WHEN** `VCPE_SKIP_IMAGE=1` is set and an operator runs `vcpe build --manifest <path>`
- **THEN** the system resolves image actions against the no-op backend (all images report as existing, no builds or pulls are executed) and reports a summary with `action: noop` for each service
