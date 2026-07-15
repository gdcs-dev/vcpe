## MODIFIED Requirements

### Requirement: Release command builds, tags, and stamps a versioned release
The system SHALL provide a `vcpe release` command that requires an explicit `--version <vX.Y.Z>` flag. The command SHALL execute the following sequence: (1) validate that the provided version tag does not already exist in git; (2) stamp all first-party service images (those with a non-empty `image.buildContext`) in the `--manifest` file using the `gopkg.in/yaml.v3` Node API, changing `image.tag` values to the provided version and preserving all YAML comments and formatting; (3) stage the manifest file (`git add`), commit it (`git commit -m "release: pin images to <version>"`), create a lightweight git tag (`git tag <version>`), push the commit (`git push origin HEAD`), and push the tag (`git push origin <version>`); (4) build all first-party service images as a multi-arch OCI manifest list (`linux/amd64` and `linux/arm64`) in a single `buildx build --push` invocation with both the versioned tag and `:latest`; (5) report completion. Third-party images (no `buildContext`) SHALL be left unchanged in the manifest and not built or pushed. The command SHALL always use the Docker backend. Version auto-detection from `git describe` is removed.

#### Scenario: Release stamps, commits, tags, and pushes
- **WHEN** an operator runs `vcpe release --manifest manifests/example.yaml --version v0.2.0`
- **THEN** the manifest is stamped with `v0.2.0`, a commit and lightweight tag `v0.2.0` are created and pushed to `origin`, and each first-party service image is built and pushed as both `<repo>:v0.2.0` and `<repo>:latest`

#### Scenario: Third-party images are untouched
- **WHEN** a manifest contains a service with no `buildContext` (e.g. `docker.io/library/alpine`)
- **THEN** `vcpe release` does not build, push, or retag that image, and its `tag` field in the manifest is not modified

#### Scenario: Existing tag fails before side effects
- **WHEN** an operator runs `vcpe release --version v0.2.0` and the tag `v0.2.0` already exists in git
- **THEN** the command fails with a clear error before touching the manifest, git history, or any images

#### Scenario: Non-main branch fails before side effects
- **WHEN** an operator runs `vcpe release` from a branch other than `main`
- **THEN** the command fails with a clear error identifying the current branch name, before touching the manifest, git history, or any images

#### Scenario: --version omitted fails immediately
- **WHEN** an operator runs `vcpe release` without `--version`
- **THEN** the command fails with a clear error explaining that `--version` is required, before any side effects

#### Scenario: Push failure after git tag — images not yet published
- **WHEN** the git push succeeds but the image build or push fails
- **THEN** the manifest stamp and git tag are intact; the operator can retry the image build+push step

### Requirement: BuildRequest supports multiple tags
The image backend's `BuildRequest` SHALL accept a `Tags []string` field. For multi-arch `buildx build --push`, each entry in `Tags` SHALL be emitted as a separate `--tag` flag so the registry receives all tags in one build operation. Single-arch builds SHALL also respect `Tags`, emitting each as `--tag`.

#### Scenario: Multi-arch release emits both versioned and latest tags
- **WHEN** `vcpe release` issues a `BuildRequest` with `Tags: ["repo:v0.2.0", "repo:latest"]` and `Platforms: ["linux/amd64", "linux/arm64"]`
- **THEN** the Docker adapter runs `docker buildx build --push --platform linux/amd64,linux/arm64 --tag repo:v0.2.0 --tag repo:latest ...` in a single invocation
