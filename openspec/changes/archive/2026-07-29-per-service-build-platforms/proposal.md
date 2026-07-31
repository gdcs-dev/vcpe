## Why

The `vcpe release` command builds all first-party images for the global platform set (`linux/amd64,linux/arm64` by default), but some images have base-image constraints that make certain platforms impossible — e.g., `xb10` is built from `localhost/xb10-dev:latest` which is arm64-only, so attempting an amd64 build fails. There is currently no way to declare a per-service platform constraint in the manifest; the only workaround is restricting the entire release to a single platform, which penalizes all other services.

## What Changes

- Adds an optional `platforms` field to `manifest.Image` (`[]string`), allowing a service to declare the set of platforms its image can be built for.
- When `image.platforms` is set, it overrides the global `--platform` flag for that service during `vcpe build` and `vcpe release`; when unset, the service inherits the global platform set.
- The release deduplication key is extended to include the effective platform set so that two services sharing the same `(repository, buildContext)` but different effective platforms are treated as distinct build targets.
- The `xb10` service entry in `manifests/xb10.yaml` is updated to declare `platforms: [linux/arm64]`.

## Capabilities

### New Capabilities

- `per-service-build-platforms`: Per-service platform override field (`image.platforms`) in the manifest schema, with propagation through the image manager and release command.

### Modified Capabilities

- `image-release-versioning`: The release build loop now resolves per-service effective platforms (service override || global default) and incorporates them into the deduplication key.
- `desired-state-manifests`: The `Image` schema gains an optional `platforms` field.

## Impact

- `controlplane/internal/manifest/model.go` — `Image` struct: add `Platforms []string`
- `controlplane/internal/image/manager.go` — `BuildWithOptions`: resolve effective platforms per-service
- `controlplane/internal/app/developer_commands.go` — `runRelease`: extend `buildKey` and `buildTarget` with per-service platforms
- `manifests/xb10.yaml` — xb10 service image block: add `platforms: [linux/arm64]`
- `openspec/specs/desired-state-manifests/spec.md` — document new `image.platforms` field
- `openspec/specs/image-release-versioning/spec.md` — document per-service platform resolution in release
