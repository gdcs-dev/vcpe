## Why

`vcpe build` and `vcpe push` are hardcoded to Podman. On Apple Silicon Macs with OrbStack, Docker's buildx uses Rosetta 2 for amd64 emulation instead of QEMU, making multi-arch builds significantly faster. There is no way to direct image build and push operations to Docker without changing source code.

## What Changes

- Add `--backend <podman|docker>` flag to `vcpe build` and `vcpe push`. Default is `podman`; existing behavior is unchanged when the flag is omitted.
- Add `internal/backend/docker/adapter.go` implementing the image operations (`ImageExists`, `BuildImage`, `PullImage`, `PushImage`, `TagImage`) using `docker` CLI commands.
- When `--backend docker` is used with platforms, `BuildImage` invokes `docker buildx build --platform <csv> --tag <tag> --push ...` — the push is embedded in the build step.
- When `--backend docker` is used without platforms, `BuildImage` invokes `docker build --tag <tag> ...` (local build only, no push).
- `vcpe push --backend docker` runs `docker push <ref>` — idempotent re-push after a `vcpe build --backend docker` with platforms.
- Document the `pullPolicy` constraint: manifests using `build-if-missing` with a Docker-built workflow must change to `always-pull` or `missing` so `vcpe up` pulls from the registry rather than triggering a Podman rebuild.

## Capabilities

### New Capabilities
- `docker-image-backend`: `vcpe build` and `vcpe push` SHALL accept a `--backend <podman|docker>` flag that selects the container runtime for image operations. When `docker` is selected, multi-arch builds use `docker buildx build --push`; single-arch builds use `docker build`.

### Modified Capabilities
- `local-control-plane-cli`: The `build` and `push` commands gain an optional `--backend` flag. Default is `podman`; existing behavior is unchanged.

## Impact

- **`internal/backend/docker/adapter.go`** — new Docker image adapter (no network/compose operations)
- **`internal/app/imagebackend.go`** — `newImageBackend(backend string)` selects docker vs podman
- **`internal/app/cli.go`** — `Options.Backend string`; `--backend` flag parsed for `build` and `push`
- **`internal/app/commands.go`** — `runBuild` and `runPush` pass `opts.Backend` to `newImageBackend`
- **`internal/app/help.go`** + golden files — document `--backend` flag on `build` and `push`
- **No changes** to networking, compose, hostnet, or `vcpe up`/`apply` behavior
- **Workflow note**: `pullPolicy: build-if-missing` is incompatible with the Docker backend workflow; manifests should use `always-pull` or `missing` when images are built and pushed with Docker
