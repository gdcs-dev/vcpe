## Purpose
Define the local declarative CLI contract and safety guarantees for planning and lifecycle operations.

## Requirements

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

### Requirement: Safe destructive operation guard
The system SHALL require explicit user confirmation or force flag semantics before destroying an active deployment, and SHALL require explicit disruptive-change approval before applying changes that alter CIDRs, reset identities, remap volumes, or scale active services to zero.

#### Scenario: Destroy blocked without explicit confirmation
- **WHEN** an operator runs `destroy` without confirmation and without force override
- **THEN** the system refuses to remove runtime resources and returns a guardrail error

#### Scenario: Disruptive apply blocked without approval
- **WHEN** an operator applies a manifest that requires a disruptive change without `--allow-disruptive` or explicit confirmation
- **THEN** the system refuses to mutate runtime resources and returns a plan summary identifying the disruptive change

### Requirement: Primary Go operator binary
The system SHALL package `vcpe` as the primary user-facing Go operator command and SHALL keep `vcpectl` available as an alias or debug path during the compatibility window.

#### Scenario: User invokes primary operator
- **WHEN** a user runs `vcpe status`
- **THEN** the command executes the same Go operator implementation used by the control-plane command path

### Requirement: Script compatibility shims
The system SHALL preserve existing documented script paths as compatibility shims for one release window after Go parity, and those shims MUST translate arguments to Go commands without owning deployment behavior.

#### Scenario: Script shim delegates to Go
- **WHEN** a user runs a documented `./scripts/vcpe` or service script command during the compatibility window
- **THEN** the script invokes the Go operator command, propagates its exit code, and does not source profiles or mutate runtime resources directly

### Requirement: Human and JSON output contracts
The system SHALL provide human-readable output by default for operator commands that report state, and SHALL provide stable JSON output when `--json` is requested.

#### Scenario: Status supports automation
- **WHEN** an operator runs `vcpe status --json`
- **THEN** the system emits machine-readable desired, planned, and observed state without relying on human formatting

### Requirement: Deployment selection by name
The system SHALL identify a target deployment by `--name` (matching `metadata.name`) for the `down`, `destroy`, `logs`, `status`, and `service` commands.

#### Scenario: Command targets a deployment by name
- **WHEN** an operator runs `vcpe status --name <metadata.name>`
- **THEN** the command operates on the deployment whose `metadata.name` matches

#### Scenario: Unknown name is reported
- **WHEN** an operator runs a deployment command with a `--name` that matches no known deployment
- **THEN** the command fails with an error identifying the unknown name

### Requirement: State schema-version reset command
The system SHALL stamp the persisted state root with the `vcpe.dev/v1` schema version and MUST refuse to operate when the stamp is missing or mismatched on a non-empty state root, directing the operator to run an explicit `vcpe state reset`.

#### Scenario: Mismatched state is refused
- **WHEN** the persisted state root has data with a missing or non-`vcpe.dev/v1` schema stamp
- **THEN** the command fails with an actionable error instructing the operator to run `vcpe state reset`

#### Scenario: State reset clears legacy state
- **WHEN** an operator runs `vcpe state reset`
- **THEN** the system clears the state root and stamps it with the current schema version

### Requirement: Test environment image skip
The system SHALL support a `VCPE_SKIP_IMAGE=1` environment variable that substitutes a no-op image backend for the real Podman backend in the `build` command and the `images` phase of `apply`, enabling unit tests to exercise the full command path without a container runtime.

#### Scenario: Build command runs without Podman when skip is set
- **WHEN** `VCPE_SKIP_IMAGE=1` is set and an operator runs `vcpe build --manifest <path>`
- **THEN** the system resolves image actions against the no-op backend (all images report as existing, no builds or pulls are executed) and reports a summary with `action: noop` for each service


---

### Requirement: Optional manifest path for apply, build, and plan
The `--manifest` flag SHALL be optional for `vcpe apply`/`up`, `vcpe build`, and `vcpe plan`. When omitted, the system SHALL attempt manifest discovery before flag validation.

If exactly one manifest is discovered: the command proceeds as if `--manifest <path>` was provided.
If zero manifests are discovered: error "no manifests found in search path; provide `--manifest` or run `vcpe manifest list`".
If two or more manifests are discovered: error listing discovered names with "specify `--manifest <name>`".

The `--manifest` flag continues to accept: absolute paths, relative paths (containing `/` or ending in `.yaml`), and bare names (no path separators, no `.yaml`). For bare names, the system searches discovery directories for `<name>.yaml`. Path-like values that do not exist on disk return a file-not-found error (no discovery fallback).

#### Scenario: --manifest omitted, single manifest available
- **WHEN** `vcpe apply` is run without `--manifest` and exactly one manifest exists in the search path
- **THEN** the command proceeds using the discovered manifest (logged at DEBUG level)

#### Scenario: --manifest omitted, multiple manifests
- **WHEN** `vcpe apply` is run without `--manifest` and multiple manifests are discovered
- **THEN** the command errors listing the discovered names and instructs the user to specify `--manifest <name>`

#### Scenario: --manifest omitted, no manifests
- **WHEN** `vcpe apply` is run without `--manifest` and no manifests are discovered
- **THEN** the command errors with "no manifests found; run `vcpe manifest list`"

#### Scenario: --manifest bare name
- **WHEN** `vcpe apply --manifest single-gateway` is run
- **THEN** the system searches discovery directories for `single-gateway.yaml` and uses it

#### Scenario: --manifest path-like value that doesn't exist
- **WHEN** `vcpe apply --manifest ./missing.yaml` is run
- **THEN** a file-not-found error is returned (no discovery fallback attempted)
