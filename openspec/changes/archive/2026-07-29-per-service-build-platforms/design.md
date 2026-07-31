## Context

`vcpe release` and `vcpe build` accept a `--platform` flag (default `linux/amd64,linux/arm64`) that is applied uniformly to every first-party service in the manifest set. The `BuildRequest` struct already carries `Platforms []string`, and the Docker adapter already handles multi-arch via `docker buildx`. However, the `manifest.Image` schema has no platform field, so there is no way to declare a per-service constraint — the only workaround is restricting the whole release to a single platform.

The `xb10` service uses `FROM localhost/xb10-dev:latest` as its base image, which exists only as `linux/arm64`. Attempting a multi-arch build fails at the amd64 layer. This blocks automated release of the full manifest set.

## Goals / Non-Goals

**Goals:**
- Allow any service in a manifest to declare `image.platforms` to constrain which platforms its image is built for.
- During `vcpe build` and `vcpe release`, a service with `image.platforms` set uses those platforms; services without the field inherit the global `--platform` default.
- The release deduplication key includes the effective platform set so identical `(repo, buildContext)` entries with different effective platforms are treated as distinct build targets.
- The `xb10` manifest entry is updated to declare `platforms: [linux/arm64]`.

**Non-Goals:**
- Changing how `EnsureForApply` (used by `vcpe up`) resolves images — it always uses native platform, and that behavior is correct and unchanged.
- Validation that `image.platforms` entries are a subset of any known global set.
- Runtime enforcement of platform constraints in Podman (the container runtime doesn't need to know).

## Decisions

### `image.platforms` lives in the manifest, not the type registry

**Decision**: The `platforms` override is a field on `manifest.Image`, not on the service type descriptor in the type registry.

**Rationale**: Platform capability is a property of the *image* (specifically its base image), not the service topology. The type registry describes how a service connects to networks and containers; it has no business knowing what hardware the base image supports. Putting it in the manifest also makes the constraint explicit and reviewable in the same place the operator declares the image reference.

**Alternative considered**: Type registry `supportedPlatforms` annotation on the `xb10` type. Rejected — it hides the constraint from the manifest operator and couples the registry to image build concerns.

### The field is authoritative everywhere, not release-only

**Decision**: When `image.platforms` is set, it overrides the global platforms for both `vcpe build` and `vcpe release`. It is not a release-only hint.

**Rationale**: The base image constraint is real and applies regardless of command. A developer running `vcpe build --manifest xb10.yaml` on an amd64 machine would fail at `FROM localhost/xb10-dev:latest` anyway — the field is documenting an existing constraint, not introducing a new one. Silently ignoring the field in some paths would be surprising and error-prone.

**Alternative considered**: A separate `image.releasePlatforms` field applied only during `vcpe release`. Rejected — two fields with overlapping semantics is confusing and the difference would be non-obvious.

### Deduplication key extended to include effective platforms

**Decision**: The release dedup key changes from `(repository, buildContext)` to `(repository, buildContext, sorted(effectivePlatforms))`.

**Rationale**: Without this, two manifests declaring the same service — one with `image.platforms: [linux/arm64]` and one without — would collide on the dedup key, causing one build to be silently skipped. With platforms in the key, they're treated as distinct build targets with distinct platform sets.

**Implementation**: The key is a struct `buildKey{repo, context, platforms string}` where `platforms` is the sorted, comma-joined effective platform list (e.g., `"linux/arm64"` or `"linux/amd64,linux/arm64"`).

## Risks / Trade-offs

- **Cross-arch local builds require QEMU** → Mitigation: this was already true for the multi-arch path; documenting it in the spec is sufficient. `EnsureForApply` is unaffected.
- **Field is silently optional** — a manifest without `image.platforms` inherits the global default, so existing manifests are fully backward compatible with no changes required.
- **Dedup key change is a behavior change** → Mitigation: the new behavior is strictly more correct; the old key could produce silent build omissions.
