## Context

The vcpe binary is already versioned via `git describe --tags` embedded at compile time. Container images are not: every manifest hard-codes `tag: dev`. Releasing currently requires manually editing manifests and separately pushing images with no canonical workflow.

The relevant image machinery already exists:
- `internal/backend/docker/adapter.go` — `BuildImage`, `PushImage`, `TagImage`; `buildImageArgs` already uses `buildx build --push` for multi-platform builds, which means the image goes directly to the registry (no local image artifact)
- `internal/image/manager.go` — `Manager`, `BuildRequest`, `BuildOptions`, `ImageReference()`
- `internal/app/commands.go` — `runBuild`, `runPush` as the existing build/push command handlers

## Goals / Non-Goals

**Goals:**
- Single `vcpe release` command: detect git tag, build multi-arch images with versioned + `:latest` tags, push, then stamp manifest files
- Comment-preserving manifest stamp using `gopkg.in/yaml.v3` Node API
- First-party-only versioning: only services with a non-empty `image.buildContext` get the version tag; third-party images (e.g. `alpine:3.19`) are untouched
- Multi-arch by default (same as `vcpe build`); always uses Docker backend

**Non-Goals:**
- No `--backend podman` support for release (Podman cannot produce multi-arch OCI manifest lists)
- No automatic `git commit` of the stamped manifest (operator does this manually)
- No changes to `vcpe build` or `vcpe push` behavior
- No support for pre-release tag formats (e.g. `v0.1.0-rc1`) — treated as any other tag

## Decisions

### Decision: Dual-tag in a single `buildx build` invocation

**Chosen:** Pass multiple `--tag` flags to a single `docker buildx build --push` call.

**Why:** Multi-arch `buildx build --push` produces no local image. `docker tag` cannot operate on a manifest list that exists only in the registry. Two alternatives exist:

| Option | Mechanism | Trade-off |
|--------|-----------|-----------|
| A — dual-tag in build | `--tag repo:vX --tag repo:latest` in same buildx call | One build, no extra round-trip, clean |
| B — imagetools after push | `docker buildx imagetools create --tag :latest :vX` | Extra step, but lets you separate "push versioned" from "promote latest" |

Option A is simpler and requires no new Docker CLI surface. Option B is better if we ever want gated promotion; not needed now.

**Implementation:** `BuildRequest.Tag string` → `BuildRequest.Tags []string`; `buildImageArgs` emits `--tag` for each entry.

---

### Decision: Stamp manifest on success only, using `yaml.Node`

**Chosen:** Perform manifest file rewriting _after_ all images have been successfully pushed. Use `gopkg.in/yaml.v3`'s `Node` API for in-place mutation.

**Why:** If the stamp happened before pushing and a push failed partway through, the manifest would claim a version that is only partially in the registry. Stamping on success means the manifest is a reliable record of what was actually published.

`yaml.Node`-based rewriting (decode to `*yaml.Node`, walk to find `tag` scalar siblings of first-party `repository` fields, mutate in place, encode back) preserves all comments and whitespace. A naive unmarshal→mutate→marshal cycle strips the 28-line ASCII topology comment block in `manifests/example.yaml`, which is worth preserving.

---

### Decision: First-party detection via `buildContext`

**Chosen:** A service is considered first-party (and gets the version tag applied) if and only if `image.buildContext != ""`.

**Why:** `buildContext` is already the indicator that vcpe owns building this image. Third-party images (pulled from Docker Hub, GHCR forks, etc.) have no `buildContext` and should never have their tag overridden by a vcpe release.

---

### Decision: Version always from `git describe --tags --abbrev=0`

**Chosen:** `vcpe release` always auto-detects the version; there is no `--tag` override flag.

**Why:** The operator creates the git tag manually before running `vcpe release`. Requiring them to also pass `--tag v0.1.0` is redundant and creates a mismatch risk. Auto-detection matches the Makefile and Homebrew tap pattern already in use. A `--tag` override can be added later if needed.

**Failure mode:** If no tag exists or `git` is unavailable, `vcpe release` fails with a clear error before touching any images or files.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Push fails midway (e.g. bng pushed, gateway fails) | Manifest stamp only happens after ALL images succeed. Registry has a partial set of images for this version, but the manifest doesn't claim otherwise. Operator re-runs `vcpe release` — buildx is idempotent. |
| `yaml.Node` round-trip changes formatting despite comment preservation | Acceptable; YAML semantics are identical. We don't format-check manifests in CI. |
| Dirty working tree causes misleading version | No guard implemented initially. If `git describe` returns a tag, we trust it. A `--require-clean` flag can be added if this becomes a problem. |
| `buildx` builder not set up | Same failure mode as existing `vcpe build --backend docker`. No special handling needed. |

## Open Questions

- Should `vcpe release` update **all** manifest files in the default search path, or only the one passed via `--manifest`? Currently scoped to the single `--manifest` file; expanding to all discovered manifests can be added later.
- Should `:latest` promotion be skippable via a flag (e.g. `--no-latest`)? Deferred — no use case identified yet.
