## Context

`vcpe build` calls `podman build -t <tag> ...`. Podman supports multi-arch OCI manifest lists via `podman build --platform linux/amd64,linux/arm64 --manifest <tag> ...`. The option threads through four layers:

```
cli.go (Options.Platforms, default ["linux/amd64","linux/arm64"])
  → commands.go (BuildOptions.Platforms)
  → image/manager.go (BuildRequest.Platforms)
  → backend/podman/adapter.go (ImageBuildRequest.Platforms → podman args)
```

`vcpe push` reuses the same manifest-loading path as `build` and calls `backend.PushImage()` (already implemented) for each service image.

## Goals / Non-Goals

**Goals:**
- `vcpe build` defaults to `linux/amd64,linux/arm64` manifest list; `--platform <csv>` overrides.
- `vcpe push --manifest <path>` pushes all service images to their registries.
- All existing tests remain green.

**Non-Goals:**
- `--platform` on `vcpe up`/`apply` — `EnsureForApply` builds host-arch at runtime; a separate concern.
- Registry authentication — handled by `podman login` outside vcpe.
- QEMU setup — prerequisite on the Podman machine; out of scope.

## Decisions

**Default platforms in `runBuild()`, not in the manager**
The image manager is policy-neutral; it executes whatever `BuildOptions.Platforms` it receives. The default `["linux/amd64", "linux/arm64"]` is applied in `runBuild()` when `opts.Platforms` is empty, keeping the manager reusable without opinionated defaults.

**`--manifest <tag>` always used when `len(Platforms) > 0`**
Podman requires `--manifest` (not `-t`) when `--platform` is present. Since the default always sets platforms, single-arch builds via `--platform linux/amd64` also use `--manifest`, which is fine — a single-arch manifest list is valid and `podman run` selects the correct arch from it.

**`runPush()` mirrors `runBuild()` structure**
Loads the manifest, runs preflight, iterates services calling `backend.PushImage(ctx, PushRequest{Reference: imageRef})` for each. Returns a summary of pushed images. Re-uses the same `newImageBackend()` seam so `VCPE_SKIP_IMAGE=1` suppresses pushes in tests.

**`push` is a manifest-driven command (requires `--manifest`)**
Consistent with `build`, `plan`, `up`, `apply`. Pushing by deployment name (`--name`) without a manifest is a separate use case not addressed here.

## Risks / Trade-offs

**BREAKING: `vcpe build` default changes**
`EnsureForApply` calls `podman image exists <tag>`. Podman returns true for manifest-list tags in the local store, so `vcpe up` after a multi-arch `vcpe build` works correctly. No data migration needed.

**Risk: QEMU not available on the Podman machine for cross-arch builds**
→ Documented in help text. `podman build` will fail with a clear error from Podman if QEMU is missing; vcpe does not need to pre-check.
