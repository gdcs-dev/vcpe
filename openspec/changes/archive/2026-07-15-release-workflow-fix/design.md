## Context

`vcpe release` was introduced in the `image-release-versioning` change. Its current flow:

1. `git describe --tags --abbrev=0` → detect version
2. Stamp manifest: `tag: dev` → `tag: vX.Y.Z`
3. Build + push images (multi-arch buildx)
4. Print "commit the manifest yourself"

The flaw: the operator must create the git tag before running the command, but the manifest stamp that should be *in* that tag commit hasn't happened yet. The tag ends up pointing to the un-stamped commit.

## Goals / Non-Goals

**Goals:**
- `--version` is a required flag on `vcpe release`; no git-detect fallback
- vcpe performs all git operations: `git add`, `git commit`, `git tag`, `git push`, `git push origin <version>`
- Correct order: stamp → commit → tag → push git → build → push images
- Images are only pushed after the git tag is publicly available

**Non-Goals:**
- Annotated tags (lightweight only, as decided)
- Signing commits or tags
- Detecting or configuring git remotes (hard-code `origin`)
- Amending or force-pushing existing tags
- A `--from-tag` fallback for manual-tagging workflows (can be added later if needed)

## Decisions

### Decision: --version is required, auto-detect removed

`DetectGitVersion()` is deleted. `runRelease` reads `opts.Version` and returns an error if empty. This eliminates the ordering ambiguity entirely — the version is always provided explicitly before any side effects occur.

---

### Decision: git operations via `os/exec`, hard-coded to `origin`

vcpe already shells out to `git` for nothing today, but it shells out to `podman` and `docker`. The same `exec.Command("git", ...)` pattern is consistent and keeps the implementation minimal. Remote is hard-coded to `origin` — the common convention. A `--remote` flag can be added later if needed.

**Sequence of git calls:**
```
git tag -l <version>         → fail if tag already exists (pre-flight)
git add <manifest-path>
git commit -m "release: pin images to <version>"
git tag <version>            → lightweight tag
git push origin HEAD         → push the release commit
git push origin <version>    → push the tag
```

---

### Decision: Build/push happens after git push, not before

Images are published after the git tag is in the remote. This means:
- The source code tag and the registry tag are in sync
- If git push fails, no images are published and the operator can retry cleanly (local commit + tag exist but are not yet pushed)
- If the image build fails after git push, the tag is public but images are absent — operator re-runs with `--skip-git` to retry just the build+push phase (see Open Questions)

---

### Decision: Fail fast if tag already exists

`git tag -l <version>` before any mutation. If the tag already exists locally or remotely, fail with a clear error before touching the manifest or git history.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Dirty working tree causes unintended files in release commit | `git add <manifest-path>` stages only the manifest file; other changes are not staged |
| `git push origin HEAD` pushes the wrong branch if HEAD is detached | Acceptable for now; operators running a release are on a branch |
| Image build fails after git push — tag is public but registry is empty | Operator can manually retry the build+push; a `--skip-git` flag is deferred |
| `origin` is not the right remote | Deferred; `--remote` flag can be added later |

## Open Questions

- Should a `--skip-git` flag be added now to support "retry build after git already succeeded"? Decision: defer — the common path is a clean first run; retry instructions in the error message are sufficient for now.
