## ADDED Requirements

### Requirement: vcpe version command
The system SHALL provide a `vcpe version` command that prints the embedded version string and exits 0. When built without `-ldflags` override, the version SHALL default to `dev`.

#### Scenario: Tagged build reports version
- **WHEN** `vcpe` is built with `-ldflags "-X main.version=0.1.0"` and an operator runs `vcpe version`
- **THEN** the command prints `0.1.0` to stdout and exits 0

#### Scenario: Untagged build reports dev
- **WHEN** `vcpe` is built without version ldflags and an operator runs `vcpe version`
- **THEN** the command prints `dev` to stdout and exits 0

## MODIFIED Requirements

### Requirement: Optional Makefile wrappers must be convenience helpers over vcpe
If a top-level Makefile is provided, it SHALL provide optional convenience wrappers over `vcpe` commands and build steps. It SHALL NOT embed divergent orchestration behavior. Targets that accept a deployment name SHALL use a `NAME` variable (defaulting to the quick-start example deployment name) mapped to the `--name` flag. The Makefile SHALL provide a `build-all` target that invokes `scripts/build-vcpe` to produce binaries for all supported architectures. The `build` target SHALL embed the version string from the nearest git tag via `-ldflags`, defaulting to `dev` when no tags exist.

#### Scenario: Make target invokes vcpe with correct flags
- **WHEN** a developer runs `make status NAME=bng-7`
- **THEN** the Makefile executes `vcpe status --name bng-7` and returns its exit code

#### Scenario: Make target uses default name when NAME is unset
- **WHEN** a developer runs `make status` without setting NAME
- **THEN** the Makefile executes `vcpe status --name bng-7` using the built-in default

#### Scenario: build-all produces multi-arch binaries
- **WHEN** a developer runs `make build-all`
- **THEN** the Makefile invokes `scripts/build-vcpe` and all supported architecture binaries are present under `controlplane/bin/`

#### Scenario: build embeds version from git tag
- **WHEN** a developer runs `make build` in a repo with tag v0.1.0 at HEAD
- **THEN** the resulting binary reports `0.1.0` when `vcpe version` is run

### Requirement: sync-homebrew-vcpe defaults to release channel with auto-detected version
`sync-homebrew-vcpe` SHALL default to the `release` Homebrew channel. When `VCPE_HOMEBREW_VERSION` is not set, the release channel SHALL auto-detect the version from the latest git tag in the source repository. When `VCPE_HOMEBREW_SHA256` is not set, it SHALL be computed by downloading the tagged archive from GitHub. The Homebrew formula SHALL embed the version string into the binary via `-ldflags` at install time.

#### Scenario: sync with no env vars uses latest tag
- **WHEN** a developer runs `scripts/sync-homebrew-vcpe` with no environment overrides and a git tag v0.1.0 exists in the repo
- **THEN** the formula is rendered with `version "0.1.0"`, pointing at the v0.1.0 archive, with the correct sha256

#### Scenario: formula embeds version in binary
- **WHEN** a user runs `brew install vcpe` from a formula with `version "0.1.0"`
- **THEN** the installed binary reports `0.1.0` when `vcpe version` is run
