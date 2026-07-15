## 1. Docker Adapter

- [x] 1.1 Create `internal/backend/docker/adapter.go` with an `Adapter` struct and `New() *Adapter`
- [x] 1.2 Implement `ImageExists(ctx, reference)` — `docker image inspect <ref>` (exit 0 = exists, any error = not found)
- [x] 1.3 Implement `BuildImage(ctx, req)` — when `len(req.Platforms) > 1`: `docker buildx build --platform <csv> --tag <tag> --push [--no-cache] [-f file] <context>`; when `len(req.Platforms) <= 1`: `docker build --tag <tag> [--platform <p>] [--no-cache] [-f file] <context>`
- [x] 1.4 Implement `PullImage`, `PushImage`, `TagImage` — direct `docker pull/push/tag` equivalents
- [x] 1.5 Add `adapter_test.go` covering `buildImageArgs` for: no platforms, single platform, multi-platform (--push path)

## 2. Image Backend Selection

- [x] 2.1 Update `newImageBackend(backend string) image.Backend` in `internal/app/imagebackend.go` — add `docker` case returning `dockerImageBackend{adapter: docker.New()}`; default remains `podman`
- [x] 2.2 Add `dockerImageBackend` struct that wraps `docker.Adapter` and satisfies `image.Backend`

## 3. CLI

- [x] 3.1 Add `Backend string` to `Options` in `internal/app/cli.go`
- [x] 3.2 Parse `--backend <value>` flag for `build` and `push` in `parseArgs()` — reject unknown values (`!= "podman" && != "docker"`) with a clear error
- [x] 3.3 Add validation: `--backend` on any command other than `build` or `push` returns an error
- [x] 3.4 Update `runBuild()` and `runPush()` in `commands.go` to call `newImageBackend(opts.Backend)` instead of `newImageBackend()`

## 4. Help Text

- [x] 4.1 Add `--backend` to `build` optional flags in `help.go` with description noting `podman` default, Docker multi-arch `--push` behavior, and `pullPolicy` compatibility constraint
- [x] 4.2 Add `--backend` to `push` optional flags in `help.go`
- [x] 4.3 Update `internal/app/testdata/help/build.golden` and `push.golden` to match new output
- [x] 4.4 Run `go test ./internal/app/... -run TestHelp` — must pass

## 5. Spec Sync

- [x] 5.1 Apply the two ADDED requirements from the delta spec to `openspec/specs/local-control-plane-cli/spec.md`

## 6. Verification

- [x] 6.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 6.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 6.3 Run `VCPE_SKIP_IMAGE=1 controlplane/bin/vcpe build --manifest manifests/example.yaml --backend docker` — must parse and run without error
- [x] 6.4 Run `controlplane/bin/vcpe up --backend docker --manifest manifests/example.yaml` — must fail with `--backend is only supported for build and push`
- [x] 6.5 Run `controlplane/bin/vcpe build --backend invalid --manifest manifests/example.yaml` — must fail with unknown backend error
