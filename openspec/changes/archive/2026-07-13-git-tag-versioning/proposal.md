## Why

`vcpe` has no version command, no version embedded in its binary, and `brew upgrade vcpe` never fires because the formula always reports `version "main"`. There is also no automated path from a git tag to an updated Homebrew formula. Developers must manually compute sha256 checksums and set environment variables to publish a release. This change adds end-to-end versioning: git tags drive the formula version, `sync-homebrew-vcpe` defaults to the latest tag automatically, and the binary itself can report its version at runtime.

## What Changes

- Add `var version = "dev"` to `cmd/vcpe/main.go`; pass it into `app.ExecuteCLI` so the CLI can expose it.
- Add `vcpe version` command that prints the embedded version string.
- Update `ExecuteCLI(prog, args, version string)` signature in `internal/app/execute.go`.
- Update the `Makefile` `build` target to embed version via `-ldflags "-X main.version=<git-tag>"`, falling back to `dev` when no tags exist.
- Update the `homebrew-tap` formula template to pass `-X 'main.version=#{version}'` in ldflags.
- Update `homebrew-tap` `release` channel: auto-detect version from `git describe --tags --abbrev=0` when `VCPE_HOMEBREW_VERSION` is unset; auto-compute sha256 from the tagged archive when `VCPE_HOMEBREW_SHA256` is unset (same curl+shasum pattern as `main`/`development` channels).
- Update `sync-homebrew-vcpe` default channel from `development` → `release`.

## Capabilities

### New Capabilities
- `vcpe-version-command`: `vcpe version` SHALL print the embedded version string. The binary SHALL default to `dev` when not built with `-ldflags`.

### Modified Capabilities
- `developer-readme-and-build-workflow`: The `make build` target and Homebrew formula SHALL embed the version string via `-ldflags`. `sync-homebrew-vcpe` SHALL default to the `release` channel and auto-detect the latest git tag and sha256.
- `local-control-plane-cli`: Add `version` as a top-level command.

## Impact

- **`controlplane/cmd/vcpe/main.go`** — add `version` var, update `ExecuteCLI` call
- **`controlplane/internal/app/execute.go`** — `ExecuteCLI` signature gains `version string`
- **`controlplane/internal/app/cli.go`** — add `"version"` to `topLevelCommands`
- **`controlplane/internal/app/local.go`** — add `case "version"` dispatch
- **`controlplane/internal/app/commands.go`** — add `runVersion(version string)`
- **`controlplane/internal/app/help.go`** — add `version` command entry; golden file update
- **`Makefile`** — embed `$(GIT_VERSION)` in `go build` ldflags; add `GIT_VERSION` computation
- **`scripts/homebrew-tap`** — release channel: auto-detect tag + sha256; ldflags in formula template
- **`scripts/sync-homebrew-vcpe`** — default channel `release`
- No manifest schema changes; no state format changes
