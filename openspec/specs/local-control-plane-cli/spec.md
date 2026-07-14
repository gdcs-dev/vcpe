## Purpose
Define the local declarative CLI contract and safety guarantees for planning and lifecycle operations.

## Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, `config`, and `state` commands as Go-owned operator commands rather than bash-owned behavior. Every command SHALL support `-h`/`--help` to display structured help text and exit 0. The `down` command SHALL remove the Podman networks created for the deployment after stopping all compose services. Network removal failures SHALL be treated as warnings and SHALL NOT prevent state cleanup. The `manifest` command group SHALL expose `list` and `build` subcommands; `build` SHALL run the interactive manifest builder wizard.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

#### Scenario: Go operator owns public command behavior
- **WHEN** an operator runs `init`, `build`, `up`, `down`, `status`, `logs`, `config`, or `state`
- **THEN** the command is handled by the Go operator command surface and uses control-plane validation, state, and output contracts

#### Scenario: Help flag exits zero on any command
- **WHEN** an operator runs `vcpe <command> --help` or `vcpe --help`
- **THEN** the system prints structured help text and exits with code 0 without executing the command

#### Scenario: down removes networks
- **WHEN** an operator runs `vcpe down --name <deployment>`
- **THEN** after stopping all compose services the system removes the Podman networks that were created for the deployment

#### Scenario: down completes even if network removal fails
- **WHEN** an operator runs `vcpe down --name <deployment>` and a network cannot be removed
- **THEN** the system logs a warning for the failed network but continues, clears IPAM leases, and removes the deployment snapshot

#### Scenario: manifest build launches wizard
- **WHEN** an operator runs `vcpe manifest build`
- **THEN** the interactive wizard starts, collects deployment identity, networks, and services, and writes a valid manifest to the output path

### Requirement: Safe destructive operation guard
The system SHALL require explicit user confirmation or force flag semantics before destroying an active deployment, and SHALL require explicit disruptive-change approval before applying changes that alter CIDRs, reset identities, remap volumes, or scale active services to zero.

#### Scenario: Destroy blocked without explicit confirmation
- **WHEN** an operator runs `destroy` without confirmation and without force override
- **THEN** the system refuses to remove runtime resources and returns a guardrail error

#### Scenario: Disruptive apply blocked without approval
- **WHEN** an operator applies a manifest that requires a disruptive change without `--allow-disruptive` or explicit confirmation
- **THEN** the system refuses to mutate runtime resources and returns a plan summary identifying the disruptive change

### Requirement: Primary Go operator binary
The system SHALL package `vcpe` as the sole user-facing Go operator command. No separate `vcpectl` alias or debug binary is provided.

#### Scenario: User invokes primary operator
- **WHEN** a user runs `vcpe status`
- **THEN** the command executes the Go operator implementation

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

### Requirement: vcpe version command
The system SHALL provide a `vcpe version` command that prints the embedded version string and exits 0. When built without `-ldflags` override, the version SHALL default to `dev`.

#### Scenario: Tagged build reports version
- **WHEN** `vcpe` is built with `-ldflags "-X main.version=0.1.0"` and an operator runs `vcpe version`
- **THEN** the command prints `0.1.0` to stdout and exits 0

#### Scenario: Untagged build reports dev
- **WHEN** `vcpe` is built without version ldflags and an operator runs `vcpe version`
- **THEN** the command prints `dev` to stdout and exits 0

### Requirement: vcpe build defaults to multi-arch manifest list
The `vcpe build` command SHALL default to building an OCI manifest list for `linux/amd64` and `linux/arm64` via `podman build --platform linux/amd64,linux/arm64 --manifest <tag>`. An optional `--platform <csv>` flag SHALL override the target platform list. QEMU emulation is required on the Podman machine for cross-arch targets.

#### Scenario: Default build produces multi-arch manifest list
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml` without `--platform`
- **THEN** the system invokes `podman build --platform linux/amd64,linux/arm64 --manifest <image-tag> ...` for each buildable service

#### Scenario: Platform override restricts to specified arches
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml --platform linux/amd64`
- **THEN** the system invokes `podman build --platform linux/amd64 --manifest <image-tag> ...` for each buildable service

### Requirement: vcpe push command
The system SHALL provide a `vcpe push --manifest <path>` command that pushes all service images referenced in the manifest to their registries. Each service's image reference (repository and tag) identifies the registry target.

#### Scenario: Push sends all service images
- **WHEN** an operator runs `vcpe push --manifest deploy.yaml`
- **THEN** the system calls `podman push <image-ref>` for each service in the manifest and reports the pushed images

#### Scenario: Push requires manifest flag
- **WHEN** an operator runs `vcpe push` without `--manifest`
- **THEN** the command fails with an error indicating that `--manifest` is required

### Requirement: vcpe build and push support a --backend flag
The `vcpe build` and `vcpe push` commands SHALL accept an optional `--backend <podman|docker>` flag that selects the container runtime for image operations. The default SHALL be `podman`. Passing `--backend` to any other command SHALL be an error. Passing an unrecognized value SHALL produce a clear error.

#### Scenario: Default backend is podman
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml` without `--backend`
- **THEN** the system uses Podman for all image operations, identical to existing behavior

#### Scenario: Docker backend selected for build
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml --backend docker`
- **THEN** the system uses Docker CLI commands; with multiple platforms, invokes `docker buildx build --platform <csv> --tag <image> --push ...`

#### Scenario: --backend on unsupported command is rejected
- **WHEN** an operator runs `vcpe up --backend docker --manifest deploy.yaml`
- **THEN** the command fails with an error stating `--backend` is only supported for `build` and `push`

#### Scenario: Unknown backend value is rejected
- **WHEN** an operator runs `vcpe build --backend invalid --manifest deploy.yaml`
- **THEN** the command fails with an error identifying the unknown backend and listing valid values

### Requirement: pullPolicy compatibility with Docker backend
The help text for `vcpe build --backend docker` SHALL document that manifests using `pullPolicy: build-if-missing` are incompatible with the Docker backend workflow, and that `always-pull` or `missing` must be used so that `vcpe up` pulls the Docker-built images from the registry.

#### Scenario: Help text warns about pullPolicy
- **WHEN** an operator runs `vcpe build --help`
- **THEN** the `--backend` flag description mentions the `pullPolicy` compatibility constraint

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

#### Scenario: manifest build launches wizard
- **WHEN** an operator runs `vcpe manifest build`
- **THEN** the interactive wizard starts, collects deployment identity, networks, and services, and writes a valid manifest to the output path

#### Scenario: --manifest path-like value that doesn't exist
- **WHEN** `vcpe apply --manifest ./missing.yaml` is run
- **THEN** a file-not-found error is returned (no discovery fallback attempted)
