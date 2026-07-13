## 1. Podman Adapter

- [x] 1.1 Add `Platforms []string` to `ImageBuildRequest` in `internal/backend/podman/adapter.go`
- [x] 1.2 Update `buildImageArgs()`: when `len(req.Platforms) > 0`, use `--platform <strings.Join(req.Platforms, ",")>` and `--manifest <tag>` instead of `-t <tag>`
- [x] 1.3 Add unit tests in `adapter_test.go` covering: no platforms (legacy `-t` path), single platform (`--manifest`), multi-platform (`--manifest`)

## 2. Image Manager

- [x] 2.1 Add `Platforms []string` to `BuildOptions` in `internal/image/manager.go`
- [x] 2.2 Add `Platforms []string` to `BuildRequest` in `internal/image/manager.go`
- [x] 2.3 Thread `opts.Platforms` into `BuildRequest` inside `BuildWithOptions()`

## 3. CLI — build command

- [x] 3.1 Add `Platforms []string` to `Options` in `internal/app/cli.go`
- [x] 3.2 Parse `--platform <csv>` flag for `build` in `parseArgs()`: split on comma, store in `opts.Platforms`
- [x] 3.3 In `runBuild()` (`commands.go`): if `opts.Platforms` is empty, default to `["linux/amd64", "linux/arm64"]` before passing to `BuildOptions`

## 4. CLI — push command

- [x] 4.1 Add `"push": {}` to the known-commands map in `internal/app/cli.go`
- [x] 4.2 Add `push` to the set of commands that require `--manifest` in `parseArgs()`
- [x] 4.3 Implement `runPush(opts Options)` in `commands.go`: load manifest, preflight, iterate services calling `backend.PushImage()`, return summary
- [x] 4.4 Wire `runPush` into the command dispatch in `local.go` (or wherever `build`/`plan` are dispatched)

## 5. Help Text

- [x] 5.1 Update `build` entry in `internal/app/help.go`: document `--platform` flag, its default (`linux/amd64,linux/arm64`), and QEMU note
- [x] 5.2 Add `push` entry in `internal/app/help.go` with description and `--manifest` flag
- [x] 5.3 Update `internal/app/testdata/help/build.golden` to match new build help
- [x] 5.4 Create `internal/app/testdata/help/push.golden` with the push help output
- [x] 5.5 Run `go test ./internal/app/... -run TestHelp` — golden tests must pass

## 6. Sync Spec

- [x] 6.1 Apply the two ADDED requirements from the delta spec to `openspec/specs/local-control-plane-cli/spec.md`

## 7. Verification

- [x] 7.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 7.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 7.3 Run `VCPE_SKIP_IMAGE=1 controlplane/bin/vcpe build --manifest manifests/example.yaml` — must show default platforms `linux/amd64,linux/arm64` in output
- [x] 7.4 Run `VCPE_SKIP_IMAGE=1 controlplane/bin/vcpe build --manifest manifests/example.yaml --platform linux/amd64` — must override to single arch
- [x] 7.5 Run `controlplane/bin/vcpe push --help` — output includes `--manifest` flag documentation
