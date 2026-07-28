## Why

`vcpe release` stamps image tags and commits a git tag in a single atomic step, which prevents operators from stamping multiple manifests and testing them before the tag is cut. Decoupling stamp-from-git allows a review window between "pin all manifests to this version" and "create the git tag and push images."

## What Changes

- **New command**: `vcpe stamp` — stamps one or more manifest files to a target version; developer-only (not built for Homebrew installs); accepts multiple `--manifest` flags with glob expansion; no git operations.
- **Updated command**: `vcpe release` — no longer performs the stamp step itself; instead discovers already-stamped manifests (via `git diff` auto-detect or explicit `--manifest` flags); verifies coherence (every first-party tag == `--version`) before committing; deduplicates image builds across manifests.
- **CLI parser**: `--manifest` becomes repeatable and glob-aware for `stamp` and `release`; adds `ManifestPaths []string` to `Options` alongside the existing single-path field.

## Capabilities

### New Capabilities

- `multi-manifest-stamp`: The `vcpe stamp` command stamps image tags in one or more manifest files to a target version, with glob expansion and validation, without touching git.

### Modified Capabilities

- `image-release-versioning`: The release workflow now separates stamp from git commit/tag/push; `vcpe release` verifies pre-stamped manifests rather than stamping them inline.
- `local-control-plane-cli`: A new `stamp` top-level command is added to the developer command surface (homebrew-excluded).

## Impact

- `controlplane/internal/app/developer_commands.go` — add `runStamp`; refactor `runRelease` to remove inline stamp, add coherence check, add multi-manifest discovery.
- `controlplane/internal/app/developer_commands_stub.go` — add `stamp` to the stub list.
- `controlplane/internal/app/cli.go` — `--manifest` becomes repeatable; add `ManifestPaths []string` to `Options`; CLI parsing accumulates and glob-expands.
- `controlplane/internal/app/help.go` — add help entry for `stamp`.
- `controlplane/internal/manifest/stamp.go` — `StampManifestFile` unchanged; add `StampManifestFiles(paths []string, version string)` batch helper.
- Golden test files for new help output.
