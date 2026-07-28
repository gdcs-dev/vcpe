## ADDED Requirements

### Requirement: vcpe stamp pins image tags in one or more manifests
The system SHALL provide a `vcpe stamp` command that accepts `--version <vX.Y.Z>` (required) and one or more `--manifest` flags. Each `--manifest` value SHALL be resolved as a file path or a glob pattern (using `filepath.Glob`). The command SHALL stamp every first-party service (`image.buildContext` non-empty) in each resolved manifest to the provided version using the `gopkg.in/yaml.v3` Node API, preserving YAML comments and formatting. The command SHALL NOT perform any git operations. `vcpe stamp` SHALL be a developer-only command excluded from Homebrew installs via the existing stub mechanism.

#### Scenario: Single manifest stamped
- **WHEN** an operator runs `vcpe stamp --version v0.3.0 --manifest manifests/example.yaml`
- **THEN** every first-party service in `manifests/example.yaml` has its `image.tag` set to `v0.3.0`; third-party images (no `buildContext`) are unchanged; no git operations occur

#### Scenario: Multiple manifests stamped via repeated flag
- **WHEN** an operator runs `vcpe stamp --version v0.3.0 --manifest manifests/example.yaml --manifest manifests/example-macvlan.yaml`
- **THEN** both manifest files are stamped to `v0.3.0` and the command reports the count of stamped manifests

#### Scenario: Glob pattern expands to multiple manifests
- **WHEN** an operator runs `vcpe stamp --version v0.3.0 --manifest "manifests/*.yaml"`
- **THEN** all YAML files in `manifests/` are stamped to `v0.3.0`

#### Scenario: Mixed explicit and glob
- **WHEN** an operator runs `vcpe stamp --version v0.3.0 --manifest "manifests/*.yaml" --manifest extra/special.yaml`
- **THEN** all files from both the glob and the explicit path are stamped

#### Scenario: Glob matches nothing
- **WHEN** an operator runs `vcpe stamp --manifest "manifests/nonexistent-*.yaml" --version v0.3.0`
- **THEN** the command fails with an error indicating no manifests matched the pattern

#### Scenario: --version omitted fails immediately
- **WHEN** an operator runs `vcpe stamp --manifest manifests/example.yaml`
- **THEN** the command fails with a clear error explaining that `--version` is required

#### Scenario: stamp is unavailable in Homebrew builds
- **WHEN** an operator running a Homebrew-installed `vcpe` binary runs `vcpe stamp`
- **THEN** the command exits non-zero with a message indicating that `stamp` is not available in distribution builds
