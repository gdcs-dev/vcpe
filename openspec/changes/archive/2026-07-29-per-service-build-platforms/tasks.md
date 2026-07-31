## 1. Manifest Schema

- [x] 1.1 Add `Platforms []string` field to `manifest.Image` struct in `controlplane/internal/manifest/model.go` with `json:"platforms,omitempty"` and `yaml:"platforms,omitempty"` tags

## 2. Image Manager

- [x] 2.1 In `image.Manager.BuildWithOptions` (`controlplane/internal/image/manager.go`), compute `effectivePlatforms` per service: use `service.Image.Platforms` if non-empty, else `opts.Platforms`
- [x] 2.2 Pass `effectivePlatforms` (not `opts.Platforms`) to `BuildRequest.Platforms` in both the `ForceBuild` path and the default `build` switch case

## 3. Release Command

- [x] 3.1 In `runRelease` (`controlplane/internal/app/developer_commands.go`), extend `buildKey` struct to include a `platforms string` field (sorted, comma-joined effective platforms)
- [x] 3.2 Extend `buildTarget` struct to carry `platforms []string` (the effective platform set for this target)
- [x] 3.3 When collecting builds, compute each service's effective platforms (`service.Image.Platforms` if non-empty, else global `platforms`) and use them in both the dedup key and the stored `buildTarget`
- [x] 3.4 In the build loop, pass `buildTarget.platforms` to `BuildRequest.Platforms` instead of the global `platforms`

## 4. Manifest Update

- [x] 4.1 Add `platforms: [linux/arm64]` to the `xb10` service's `image` block in `manifests/xb10.yaml`

## 5. Tests

- [x] 5.1 Add a unit test in `controlplane/internal/image/manager_test.go` verifying that a service with `Image.Platforms` set uses those platforms in the `BuildRequest`, overriding `opts.Platforms`
- [x] 5.2 Add a unit test verifying that a service without `Image.Platforms` uses `opts.Platforms` in the `BuildRequest`

## 6. Spec Sync

- [x] 6.1 Run `openspec sync-specs --change per-service-build-platforms` to merge delta specs into main specs
