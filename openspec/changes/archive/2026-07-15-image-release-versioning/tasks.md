## 1. Image Backend — Multi-Tag Support

- [x] 1.1 In `internal/backend/docker/adapter.go`: rename `BuildRequest.Tag string` → `BuildRequest.Tags []string`; update `buildImageArgs` to emit `--tag` for each entry (both multi-arch and single-arch paths)
- [x] 1.2 In `internal/backend/podman/adapter.go`: apply the same `Tags []string` change to Podman's `BuildRequest` for consistency
- [x] 1.3 Update all callers of `BuildImage` (in `image/manager.go`) to pass `Tags: []string{imageRef}` instead of `Tag: imageRef`
- [x] 1.4 Update `BuildRequest` golden tests in both backend packages to reflect `Tags` field

## 2. Git Version Detection

- [x] 2.1 In `internal/app/` (or a new `internal/release/` package): add `DetectGitVersion() (string, error)` that runs `git describe --tags --abbrev=0` and returns the tag as-is (e.g. `v0.1.0`); returns a clear error if no tag exists

## 3. Manifest Stamper

- [x] 3.1 In `internal/manifest/` (or `internal/release/`): implement `StampManifestFile(path, version string) error` that uses the `gopkg.in/yaml.v3` Node API to walk the manifest, find `image.tag` scalar nodes for services where `image.buildContext != ""`, replace their value with `version`, and write the file back in place preserving all comments and formatting
- [x] 3.2 Add unit test: given a manifest YAML with two services (one first-party with buildContext, one third-party without), stamping with `v0.1.0` only changes the first-party tag and leaves the third-party tag unchanged; comments in the YAML are preserved

## 4. Release Command

- [x] 4.1 In `internal/app/commands.go`: implement `runRelease(opts Options)` that: (1) calls `DetectGitVersion()`, (2) loads the manifest, (3) for each first-party service builds with `Tags: [repo:version, repo:latest]` using `BuildOptions{Platforms: defaultPlatforms, ForceBuild: true}`, (4) calls `StampManifestFile` only after all builds succeed, (5) returns a summary message
- [x] 4.2 In `internal/app/cli.go`: add `"release"` to `topLevelCommands`; wire `release` in the command dispatch to `runRelease`
- [x] 4.3 Ensure `vcpe release --help` prints a useful description of the command

## 5. Spec Sync

- [x] 5.1 Apply MODIFIED `local-control-plane-cli` spec to `openspec/specs/local-control-plane-cli/spec.md`
- [x] 5.2 Apply ADDED `image-release-versioning` spec to `openspec/specs/image-release-versioning/spec.md`

## 6. Verification

- [x] 6.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 6.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 6.3 Smoke test: create a test git tag, run `vcpe release --manifest manifests/example.yaml`, verify both `:vX` and `:latest` are pushed and `manifests/example.yaml` has the pinned tag for first-party services only
