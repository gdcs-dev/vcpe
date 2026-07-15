## Why

Container images are currently tagged with a hardcoded `dev` tag in every manifest, making it impossible to pin a deployment to a specific release. The vcpe binary already uses `git describe --tags` for versioning; images should follow the same scheme so a single git tag produces a fully-versioned, reproducible release artifact.

## What Changes

- Add `vcpe release` command that: detects the current git tag, builds all first-party images (those with a `buildContext`) with two tags simultaneously (`v<N>` and `latest`) via multi-arch `buildx`, then stamps the manifest's `image.tag` fields on success.
- `BuildRequest` in the image backend gains a `Tags []string` field (replaces single `Tag string`) so `buildx build` can emit multiple `--tag` flags in one invocation.
- Manifest stamping: after a successful release push, `vcpe release` rewrites the manifest file using the `yaml.Node` API to change `tag: dev` → `tag: v<N>` for first-party images, preserving all YAML comments and formatting.
- `vcpe release` always uses the Docker backend (multi-arch push via `buildx` requires it); no `--backend` flag is needed.
- First-party detection: services with a non-empty `image.buildContext` are versioned; third-party images (e.g., `alpine:3.19`) are left unchanged.

## Capabilities

### New Capabilities

- `image-release-versioning`: The `vcpe release` command: git-tag detection, dual-tag multi-arch build-and-push, and comment-preserving YAML manifest stamping.

### Modified Capabilities

- `local-control-plane-cli`: New `release` subcommand added to the vcpe CLI surface.

## Impact

- `controlplane/internal/backend/docker/adapter.go`: `BuildImage` request gains `Tags []string`; `buildImageArgs` emits multiple `--tag` flags for multi-arch buildx builds.
- `controlplane/internal/image/manager.go`: `BuildRequest.Tag` replaced by `Tags []string`; new `ReleaseWithOptions` or similar orchestration in `Manager`.
- `controlplane/internal/app/commands.go`: new `runRelease` function wired to the `release` top-level command.
- `controlplane/internal/app/cli.go`: `release` added to `topLevelCommands`; no new flags (version auto-detected from git).
- `manifests/example.yaml` and `manifests/example-macvlan.yaml`: stamped with pinned tag at release time (not a code change; happens at operator runtime).
- No breaking changes to existing `vcpe build` or `vcpe push` commands.
