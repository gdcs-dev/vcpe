## Purpose
Define the top-level developer workflow and documentation contract for building, running, and verifying the project during the Go control-plane migration.

## Requirements

### Requirement: Top-level README must provide an end-to-end developer workflow
The project SHALL provide a top-level README that documents project overview, prerequisites, build steps, run steps, and test/verification steps with executable command examples for the primary Go `vcpe` command driven by a `vcpe.dev/v1` `Deployment` manifest.

#### Scenario: New developer follows quick start
- **WHEN** a developer reads the top-level README quick start section
- **THEN** the developer can author a `vcpe.dev/v1` manifest, apply a representative deployment, and view its status by `--name` using documented commands only

#### Scenario: Developer sees Go primary command path
- **WHEN** a developer reads build and run examples
- **THEN** the examples identify `vcpe` as the primary command and use manifest-driven deployment selected by `--name` rather than profile or `--customer` selection

### Requirement: Optional Makefile wrappers must be convenience helpers over vcpe
If a top-level Makefile is provided, it SHALL provide optional convenience wrappers over `vcpe` commands. It SHALL NOT embed divergent orchestration behavior. Targets that accept a deployment name SHALL use a `NAME` variable (defaulting to the quick-start example deployment name) mapped to the `--name` flag. The `build` target SHALL embed the version string from the nearest git tag via `-ldflags`, defaulting to `dev` when no tags exist.

#### Scenario: Make target invokes vcpe with correct flags
- **WHEN** a developer runs `make status NAME=bng-7`
- **THEN** the Makefile executes `vcpe status --name bng-7` and returns its exit code

#### Scenario: Make target uses default name when NAME is unset
- **WHEN** a developer runs `make status` without setting NAME
- **THEN** the Makefile executes `vcpe status --name bng-7` using the built-in default

#### Scenario: build embeds version from git tag
- **WHEN** a developer runs `make build` in a repo with tag v0.1.0 at HEAD
- **THEN** the resulting binary reports `0.1.0` when `vcpe version` is run

### Requirement: README examples must reference stable deeper documentation
The top-level README SHALL link to runbook or detailed docs for advanced operational procedures.

#### Scenario: Advanced operator flow lookup
- WHEN a developer needs deeper operational guidance than quick start
- THEN the README provides direct links to detailed documentation sections

### Requirement: vcpe build defaults to multi-arch manifest list
The `vcpe build` command SHALL default to building an OCI manifest list for `linux/amd64` and `linux/arm64`. An optional `--platform <csv>` flag SHALL override the target platform list. When platforms are specified (including via default), the system SHALL invoke `podman build --platform <platforms> --manifest <tag>` instead of `podman build -t <tag>`.

#### Scenario: Default build produces multi-arch manifest list
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml` without `--platform`
- **THEN** the system invokes `podman build --platform linux/amd64,linux/arm64 --manifest <image-tag> ...` for each buildable service

#### Scenario: Platform override restricts to one arch
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml --platform linux/amd64`
- **THEN** the system invokes `podman build --platform linux/amd64 --manifest <image-tag> ...` for each buildable service

#### Scenario: Help text documents the flag and QEMU requirement
- **WHEN** an operator runs `vcpe build --help`
- **THEN** the output documents the `--platform` flag, its default value, and notes that QEMU emulation is required on the Podman machine for cross-arch targets

### Requirement: vcpe push command
The system SHALL provide a `vcpe push --manifest <path>` command that pushes all service images referenced in the manifest to their registries. Each service's image reference (repository and tag) identifies the registry target; no separate `--registry` flag is required.

#### Scenario: Push sends all service images
- **WHEN** an operator runs `vcpe push --manifest deploy.yaml`
- **THEN** the system calls `podman push <image-ref>` for each service in the manifest and reports the pushed images

#### Scenario: Push requires manifest flag
- **WHEN** an operator runs `vcpe push` without `--manifest`
- **THEN** the command fails with an error indicating that `--manifest` is required

### Requirement: sync-homebrew-vcpe defaults to release channel with auto-detected version
`sync-homebrew-vcpe` SHALL default to the `release` Homebrew channel. When `VCPE_HOMEBREW_VERSION` is not set, the release channel SHALL auto-detect the version from the latest git tag in the source repository. When `VCPE_HOMEBREW_SHA256` is not set, it SHALL be computed by downloading the tagged archive from GitHub. The Homebrew formula SHALL embed the version string into the binary via `-ldflags` at install time.

#### Scenario: sync with no env vars uses latest tag
- **WHEN** a developer runs `scripts/sync-homebrew-vcpe` with no environment overrides and a git tag v0.1.0 exists in the repo
- **THEN** the formula is rendered with `version "0.1.0"`, pointing at the v0.1.0 archive, with the correct sha256

#### Scenario: formula embeds version in binary
- **WHEN** a user runs `brew install vcpe` from a formula with `version "0.1.0"`
- **THEN** the installed binary reports `0.1.0` when `vcpe version` is run
