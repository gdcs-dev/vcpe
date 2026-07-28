## Context

The current `vcpe release` command stamps image tags, commits, tags, and pushes in one atomic step using a single manifest file. Operators who maintain multiple manifests (`example.yaml`, `example-macvlan.yaml`, `xb10.yaml`) cannot stamp all of them before the git tag is created, meaning there is no window to test the pinned versions across deployments before the release is published.

The existing `manifest.StampManifestFile(path, version)` function is already decoupled from git; the release command simply calls it inline before the git operations. Decoupling at the command level is therefore a surgical change.

## Goals / Non-Goals

**Goals:**
- Introduce `vcpe stamp` as a developer-only command that stamps one or more manifests without touching git
- Make `vcpe release` verify pre-stamped manifests instead of stamping them inline
- Support multiple `--manifest` flags and glob patterns in both `stamp` and `release`
- Preserve backward compatibility: `vcpe release --manifest x.yaml --version vX.Y.Z` continues to work (release auto-detects or user provides explicit paths)

**Non-Goals:**
- Changing the image build or push logic beyond deduplication across manifests
- Supporting non-developer (Homebrew) installs of `stamp`
- Auto-stamping on commit hooks or CI pipelines

## Decisions

### D1: `--manifest` becomes repeatable and glob-expanding for stamp/release only

The `Options` struct gains a `ManifestPaths []string` field. The CLI parser accumulates `--manifest` values into this slice for `stamp` and `release`. Each value is tested with `filepath.Glob`; if it expands to one or more paths, those are added; if it does not expand (no wildcard characters or exact file path), it is used as-is with `os.Stat` validation. All other commands keep using `ManifestPath string` unchanged.

**Why not change `ManifestPath` to a slice everywhere?** Too broad a change; breaks the type contract for all single-manifest commands. A parallel field is safer.

### D2: `vcpe release` discovers manifests via `git diff` when `--manifest` is omitted

When `release` receives no `--manifest` flags, it runs `git diff --name-only -- manifests/` to find modified YAML files. This avoids requiring the operator to repeat the same flags they typed for `stamp`. If the discovery set is empty, release errors with a clear message.

**Alternative considered: always require explicit `--manifest` on release.** Rejected — it would force operators to duplicate the list they already typed for `stamp`, defeating half the ergonomic benefit.

### D3: Release verifies coherence before git operations

After collecting the manifest set (explicit or auto-detected), `release` loads each manifest and verifies that every first-party service (non-empty `buildContext`) has `image.tag == --version`. This catches the case where stamp was run with a different version or a manifest was accidentally left un-stamped.

### D4: Image build deduplication across manifests

Release collects `(repository, buildContext)` pairs from all manifests and deduplicates before building. The first occurrence of each pair determines the build parameters. This prevents rebuilding the same image multiple times when it appears in several manifests.

### D5: `stamp` is homebrew-excluded via the existing stub mechanism

`developer_commands_stub.go` lists commands that are not compiled into Homebrew installs. `stamp` is added to this list alongside `build`, `push`, and `release`. The registration function call in `developer_commands.go` adds `stamp` to `topLevelCommands` at init time.

## Risks / Trade-offs

- **git diff discovery picks up accidental edits** → The coherence check (D3) guards against this; a file with a different version or un-stamped services will error before any git operations.
- **Glob expansion ordering is OS-dependent** → `filepath.Glob` is alphabetical and deterministic; image deduplication means order only matters for which manifest's build parameters are used when two services share the same repo+context with different flags (rare in practice).
- **Two-step workflow is forgettable** → `release` without pre-stamped files will fail the coherence check and print a clear hint to run `vcpe stamp` first.
