## MODIFIED Requirements

### Requirement: Release command builds, tags, and stamps a versioned release
The system SHALL provide a `vcpe release` command that requires an explicit `--version <vX.Y.Z>` flag. The command SHALL execute the following sequence: (1) validate git state (must be on `main`, tag must not exist); (2) collect the manifest set — from explicit `--manifest` flags (with glob expansion) if provided, or by running `git diff --name-only -- manifests/` to auto-detect modified YAML files when `--manifest` is omitted; (3) verify coherence: for each manifest in the set, every first-party service (`image.buildContext` non-empty) MUST have `image.tag` already equal to `--version` — fail with a clear error naming the offending manifest and service if not; (4) stage all manifest files in the set (`git add`), commit (`git commit -m "release: pin images to <version>"`), create a lightweight git tag, push commit and tag to `origin`; (5) build and push all first-party images deduplicated by `(repository, buildContext, effectivePlatforms)` across all manifests, where `effectivePlatforms` is `image.platforms` if non-empty, otherwise the global `--platform` default. Each deduplicated build target is built as an OCI manifest tagged with both `:<version>` and `:latest` for its effective platform set. Third-party images SHALL be left unchanged. The command SHALL always use the Docker backend by default.

**Stamp is no longer performed by `release`**: operators MUST run `vcpe stamp` before `vcpe release`. This provides a review window between pinning image tags and publishing the release.

#### Scenario: Release with pre-stamped manifests succeeds
- **WHEN** an operator runs `vcpe stamp --version v0.3.0 --manifest "manifests/*.yaml"` followed by `vcpe release --version v0.3.0`
- **THEN** release auto-detects the stamped manifests via git diff, verifies all first-party tags equal `v0.3.0`, commits, tags, pushes, and builds+pushes deduplicated images

#### Scenario: Release with explicit manifest set
- **WHEN** an operator runs `vcpe release --version v0.3.0 --manifest manifests/example.yaml --manifest manifests/example-macvlan.yaml`
- **THEN** release uses those two files, verifies coherence, and proceeds with git and image operations

#### Scenario: Coherence check fails on un-stamped manifest
- **WHEN** an operator runs `vcpe release --version v0.3.0` but one manifest was not stamped (its first-party services still have `tag: dev`)
- **THEN** release fails before any git operations with an error identifying the manifest file and service that has the wrong tag, and hints to run `vcpe stamp` first

#### Scenario: Auto-detect finds no modified manifests
- **WHEN** an operator runs `vcpe release --version v0.3.0` with no `--manifest` flags and `git diff` shows no modified files in `manifests/`
- **THEN** release fails with an error explaining that no stamped manifests were detected and instructs the operator to run `vcpe stamp` first

#### Scenario: Image deduplication across manifests respects effective platforms
- **WHEN** two manifests both declare a `bng` service with the same `repository` and `buildContext` and neither has `image.platforms` set
- **THEN** the image is built and pushed exactly once using the global platform default

#### Scenario: Services with different effective platforms are not deduplicated together
- **WHEN** manifest A declares service X with `image.platforms: [linux/arm64]` and manifest B declares service X with the same `repository` and `buildContext` but no `image.platforms` (effective: `linux/amd64,linux/arm64`)
- **THEN** two separate builds are issued: one for `linux/arm64` and one for `linux/amd64,linux/arm64`

#### Scenario: Per-service platform override honored in release build
- **WHEN** a manifest service declares `image.platforms: [linux/arm64]` and `vcpe release` is run with default platforms (`linux/amd64,linux/arm64`)
- **THEN** that service's image is built and pushed for `linux/arm64` only; other services without `image.platforms` are built for `linux/amd64,linux/arm64`

#### Scenario: Third-party images are untouched
- **WHEN** a manifest contains a service with no `buildContext` (e.g. `docker.io/library/alpine`)
- **THEN** `vcpe release` does not build, push, or retag that image

#### Scenario: Existing tag fails before side effects
- **WHEN** an operator runs `vcpe release --version v0.3.0` and the tag `v0.3.0` already exists in git
- **THEN** the command fails with a clear error before any manifest reads, git operations, or image builds

#### Scenario: Non-main branch fails before side effects
- **WHEN** an operator runs `vcpe release` from a branch other than `main`
- **THEN** the command fails with a clear error identifying the current branch name, before any operations

#### Scenario: --version omitted fails immediately
- **WHEN** an operator runs `vcpe release` without `--version`
- **THEN** the command fails with a clear error explaining that `--version` is required
