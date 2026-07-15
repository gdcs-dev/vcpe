## Why

`vcpe build` invokes `podman build -t <tag>` which produces a single-architecture image for the host platform. There is no way to produce container images for both `linux/amd64` and `linux/arm64` in one step, and there is no `vcpe push` command to push built images to a registry. Both gaps require the same `--platform` thread-through: CLI → image manager → podman adapter.

## What Changes

- **BREAKING** `vcpe build` defaults to `linux/amd64,linux/arm64`: invokes `podman build --platform linux/amd64,linux/arm64 --manifest <tag>` producing an OCI manifest list. Use `--platform linux/amd64` (or any single value) to restrict to one arch.
- `BuildOptions` and `BuildRequest` gain `Platforms []string` with a default of `["linux/amd64", "linux/arm64"]` applied in `runBuild()`.
- When platforms are specified, `podman build` uses `--platform <csv>` and `--manifest <tag>` instead of `-t <tag>`.
- Add `vcpe push --manifest <path>` command that pushes all service images from the manifest to their registries (registry is already embedded in each image reference).
- Help text and golden test files updated for both changes.
- `vcpe up`/`apply` are unaffected — `EnsureForApply` builds/pulls for the host arch at runtime and queries manifest lists correctly via `podman image exists`.

## Capabilities

### New Capabilities
- `multi-arch-image-build`: `vcpe build` SHALL default to building `linux/amd64` and `linux/arm64` as an OCI manifest list. An optional `--platform` flag overrides the target list.
- `vcpe-push`: `vcpe push --manifest <path>` SHALL push all service images referenced in the manifest to their registries.

### Modified Capabilities
- `local-control-plane-cli`: **BREAKING** — `vcpe build` default behavior changes to multi-arch manifest list. `build` gains `--platform` override flag. New `push` command added.

## Impact

- **`internal/backend/podman/adapter.go`** — `ImageBuildRequest` + `buildImageArgs()`: add `Platforms []string`; switch to `--manifest`/`--platform` when set
- **`internal/image/manager.go`** — `BuildOptions` + `BuildRequest`: add `Platforms []string`; pass through
- **`internal/app/cli.go`** — `Options`: add `Platforms []string` + `--platform` flag for `build`; add `push` to command table
- **`internal/app/commands.go`** — `runBuild()`: set default platforms; add `runPush()`
- **`internal/app/help.go`** — update `build` help, add `push` help
- **Tests**: `adapter_test.go`, `help_test.go` + `build.golden` + new `push.golden`
