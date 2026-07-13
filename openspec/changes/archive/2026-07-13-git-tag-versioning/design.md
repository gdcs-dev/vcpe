## Context

`ExecuteCLI(prog, args)` is the sole CLI entry point called by `cmd/vcpe/main.go`. The version string must flow from `main.go` (where `-ldflags` inject it) through to the command dispatch in `local.go`. Adding a `version` parameter to `ExecuteCLI` is a one-callsite change since `cmd/vcpectl` is already deleted.

The Homebrew formula builds from source, so `-X main.version=#{version}` in the formula's ldflags is the natural injection point — the formula's `version` Ruby attribute (e.g., `"0.1.0"`) is available in the `def install` block.

The `homebrew-tap` release channel already supports all three inputs (`version`, `sha256`, `url`) but currently requires both to be set via env vars. The branch channels (`main`, `development`) already auto-compute sha256 by downloading the tarball; the release channel just needs the same treatment, plus auto-detection of the version tag.

## Goals / Non-Goals

**Goals:**
- `vcpe version` prints the embedded version string (`0.1.0` for tagged builds, `dev` for local dev builds).
- `make build` embeds the latest git tag; falls back to `dev` with no tags.
- `sync-homebrew-vcpe` with no env vars uses the latest git tag from `REPO_ROOT`, downloads the archive, computes sha256, and syncs the formula.
- Homebrew formula embeds the version into the binary via ldflags.

**Non-Goals:**
- CI/CD release pipeline — this remains a manual `scripts/sync-homebrew-vcpe` invocation.
- Commit-sha or dirty-tree versioning for local dev — `dev` is sufficient.
- `vcpe --version` flag — `vcpe version` subcommand is consistent with the existing command surface.

## Decisions

**`ExecuteCLI` gains a `version string` parameter**
One callsite, clean propagation. The version is threaded through `Options` (or passed directly to `runVersion`) without touching the planner, manifest, or any domain logic. Tests that call `executeLocal` directly are unaffected since `runVersion` is handled before `executeLocal`.

**Version detection in `Makefile`: `git describe --tags --abbrev=0`**
Returns the nearest tag reachable from HEAD (e.g., `v0.1.0`). Strips the leading `v` with `sed`. Falls back to `dev` when no tags exist. Does not include commit distance or dirty suffix — the formula tag and binary version should match exactly.

**Auto-detection in `homebrew-tap` release channel: `git -C "$REPO_ROOT" describe --tags --abbrev=0`**
`REPO_ROOT` is always set by `common.sh` so this is safe. The tag must be pushed to GitHub before `sync-homebrew-vcpe` runs (sha256 is computed from the live tarball). If the tag is not yet pushed, `curl` fails with an actionable error.

**`sync-homebrew-vcpe` default changes from `development` to `release`**
Keeps the existing override path: `VCPE_HOMEBREW_CHANNEL=development sync-homebrew-vcpe` still works for branch-tracking syncs.

## Risks / Trade-offs

**Risk: tag not yet pushed when sync runs**
→ `curl` fails with a 404-style error. Message is clear enough; no special handling needed.

**Risk: annotated vs. lightweight tags**
→ `git describe --tags` works with both. `--abbrev=0` strips the suffix so `v0.1.0-3-gabcdef` → `v0.1.0`. Desired behavior for release syncs.
