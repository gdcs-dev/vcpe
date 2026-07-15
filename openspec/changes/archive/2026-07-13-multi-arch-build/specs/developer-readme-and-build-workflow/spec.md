## ADDED Requirements

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
