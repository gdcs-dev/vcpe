## ADDED Requirements

### Requirement: Release command builds, tags, and stamps a versioned release
The system SHALL provide a `vcpe release` command that: (1) detects the current release version from `git describe --tags --abbrev=0`, (2) builds all first-party service images (those with a non-empty `image.buildContext`) as a multi-arch OCI manifest list (`linux/amd64` and `linux/arm64`) in a single `buildx build --push` invocation with both the versioned tag and `:latest`, (3) after all images are successfully pushed, rewrites each `--manifest` file in place using the `gopkg.in/yaml.v3` Node API to change first-party service `image.tag` values from their current value to the detected version, preserving all YAML comments and formatting. Third-party images (no `buildContext`) SHALL be left unchanged. The command SHALL always use the Docker backend and SHALL fail with a clear error if no git tag is found before touching any image or file.

#### Scenario: Release stamps versioned and latest tags
- **WHEN** an operator runs `vcpe release --manifest manifests/example.yaml` on a commit tagged `v0.1.0`
- **THEN** each first-party service image is built and pushed as both `<repo>:v0.1.0` and `<repo>:latest`, and `manifests/example.yaml` is updated so every first-party `image.tag` reads `v0.1.0`

#### Scenario: Third-party images are untouched
- **WHEN** a manifest contains a service with no `buildContext` (e.g. `docker.io/library/alpine`)
- **THEN** `vcpe release` does not build, push, or retag that image, and its `tag` field in the manifest is not modified

#### Scenario: No git tag fails before side effects
- **WHEN** an operator runs `vcpe release` and no git tag exists on the current commit or its ancestors
- **THEN** the command fails with an error identifying the missing tag and makes no changes to images or manifest files

#### Scenario: Push failure prevents manifest stamp
- **WHEN** one or more image pushes fail during `vcpe release`
- **THEN** the manifest file is NOT modified; the operator can re-run `vcpe release` after resolving the push failure

### Requirement: BuildRequest supports multiple tags
The image backend's `BuildRequest` SHALL accept a `Tags []string` field (replacing the single `Tag string`). For multi-arch `buildx build --push`, each entry in `Tags` SHALL be emitted as a separate `--tag` flag so the registry receives all tags in one build operation. Single-arch builds SHALL also respect `Tags`, emitting each as `--tag`.

#### Scenario: Multi-arch release emits both versioned and latest tags
- **WHEN** `vcpe release` issues a `BuildRequest` with `Tags: ["repo:v0.1.0", "repo:latest"]` and `Platforms: ["linux/amd64", "linux/arm64"]`
- **THEN** the Docker adapter runs `docker buildx build --push --platform linux/amd64,linux/arm64 --tag repo:v0.1.0 --tag repo:latest ...` in a single invocation
