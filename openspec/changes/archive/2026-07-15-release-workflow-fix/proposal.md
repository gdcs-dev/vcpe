## Why

`vcpe release` currently auto-detects the version from `git describe --tags --abbrev=0`, which requires the git tag to exist before the command runs. This means the manifest stamp (and the release commit) lands *after* the tag — so the tag points to the un-stamped manifest. The Homebrew tarball and `git checkout v0.2.0` both see `tag: dev` instead of the pinned version.

## What Changes

- **BREAKING** `vcpe release` now requires an explicit `--version <vX.Y.Z>` flag; version auto-detection from `git describe` is removed.
- `vcpe release` performs the git release sequence itself: stages the manifest, commits it, creates a lightweight tag, and pushes both the commit and the tag to `origin` before building and pushing images.
- The correct ordering is enforced: stamp → commit → tag → push git → build → push images.
- `DetectGitVersion()` (`internal/app/release.go`) is removed since it is no longer used.
- Help text and CLI validation updated for `--version`.

## Capabilities

### New Capabilities

_(none — this is a workflow correction, not a new capability)_

### Modified Capabilities

- `image-release-versioning`: The `vcpe release` command contract changes: `--version` is now required; the command performs git commit + tag + push as part of the release sequence, and image builds happen after the tag is pushed.
- `local-control-plane-cli`: `release` command now requires `--version`; updated help text and usage example.

## Impact

- `controlplane/internal/app/release.go`: Remove `DetectGitVersion()`; add `runGitRelease(manifestPath, version string) error` that performs `git add`, `git commit`, `git tag`, `git push`, `git push origin <version>`.
- `controlplane/internal/app/commands.go`: `runRelease` uses `opts.Version` (required); calls `runGitRelease` after stamping, before building images.
- `controlplane/internal/app/cli.go`: `--version` added as a parsed flag on the `release` command; validation requires it to be non-empty for `release`.
- `controlplane/internal/app/help.go`: Updated release help text, flags, and examples.
- `controlplane/internal/app/testdata/help/*.golden`: Regenerated to reflect new flags.
