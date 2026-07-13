## Context

`newImageBackend()` in `internal/app/imagebackend.go` is hardcoded to return a `podmanImageBackend`. The `image.Backend` interface it satisfies already abstracts all five image operations cleanly. Adding a Docker backend is a new implementation of that interface — no interface changes needed.

The Docker CLI surface for image operations maps directly:

| Operation | Podman (current) | Docker (new, no platforms) | Docker (new, with platforms) |
|---|---|---|---|
| Build | `podman build -t <tag> ...` | `docker build --tag <tag> ...` | `docker buildx build --platform <csv> --tag <tag> --push ...` |
| Exists | `podman image exists <ref>` | `docker image inspect <ref>` (exit 0 = exists) | same |
| Pull | `podman pull <ref>` | `docker pull <ref>` | same |
| Push | `podman push <ref>` | `docker push <ref>` | same (re-push; idempotent) |
| Tag | `podman tag <src> <dst>` | `docker tag <src> <dst>` | same |

## Goals / Non-Goals

**Goals:**
- `--backend podman|docker` flag on `build` and `push`.
- Docker adapter for image operations only.
- No pre-cleanup step needed for Docker (overwriting an existing tag is safe; no manifest-list collision like Podman).

**Non-Goals:**
- Docker support for `vcpe up`/`apply`, networking, or compose. Podman owns those permanently in this change.
- `nerdctl` or other runtimes — reserved for future extension.
- Auto-detection of available runtimes.

## Decisions

**`--backend` is a `build`/`push`-only flag, validated in `parseArgs`**
If passed to any other command, `parseArgs` returns an error. This keeps the scope honest and avoids the confusion of "what does `--backend docker` mean for `vcpe up`?"

**Docker multi-arch: `buildx build --push` (no separate push step)**
With Docker, `--push` is embedded in the build when platforms are specified. `vcpe push --backend docker` still works (idempotent `docker push`) but is redundant after a multi-arch build. This is documented in help text.

**No platforms → `docker build` (not `buildx`)**
When `--platform` is not specified (or a single native arch is requested), use plain `docker build` for maximum compatibility. `buildx` is only needed for multi-arch.

**`newImageBackend` takes a `backend string` parameter**
All three call sites (`runBuild`, `runPush`, orchestrator's `EnsureForApply`) already call `newImageBackend()`. The orchestrator call is unaffected — it has no `opts.Backend` and always uses the default (`podman`). Only `runBuild` and `runPush` pass the flag value.

## Risks / Trade-offs

**`pullPolicy: build-if-missing` is incompatible with the Docker workflow**
After `vcpe build --backend docker`, images are in the registry, not in Podman's local store. `vcpe up` calls `EnsureForApply` with the Podman backend. If `pullPolicy` is `build-if-missing`, Podman will trigger a local rebuild. Mitigation: document this constraint clearly in help text and the `--backend docker` output message.

**`docker buildx` requires a buildx builder**
On OrbStack this is pre-configured. On standard Docker Desktop the default builder may not support multi-platform. No code-level check needed — `docker buildx build` will fail with a clear error if the builder is missing.
