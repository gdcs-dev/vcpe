## 1. CLI — Add --version Flag

- [x] 1.1 In `internal/app/cli.go`: parse `--version` as a string flag into `opts.Version` for the `release` command; add `release` to the set of commands that require `--version` non-empty (alongside the existing `--manifest` requirement)
- [x] 1.2 In `internal/app/cli.go`: remove `release` from the `--backend` / `--platform` allowed-commands guard if it is blocking `--version`; ensure `--version` is accepted only on `release`
- [x] 1.3 In `internal/app/help.go`: update the `release` entry — replace `--platform` / `--backend` optional flags with `--version` as a required flag; update synopsis, description, and examples to reflect the new workflow (`git tag` is no longer a prerequisite)
- [x] 1.4 Regenerate golden help files: `go test ./internal/app/ -run TestHelp -update`

## 2. Git Release Helper

- [x] 2.1 In `internal/app/release.go`: remove `DetectGitVersion()`; add `runGitRelease(manifestPath, version string) error` that executes in sequence: (a) `git rev-parse --abbrev-ref HEAD` — fail if result is not `main`, (b) `git tag -l <version>` — fail if output is non-empty, (c) `git add <manifestPath>`, (d) `git commit -m "release: pin images to <version>"`, (e) `git tag <version>`, (f) `git push origin HEAD`, (g) `git push origin <version>`; each step must succeed before the next runs
- [x] 2.2 Add unit-level test for `runGitRelease` using a real temp git repo (`git init`, `git remote add origin`): verify the correct sequence of commands is invoked and that an existing tag causes an early failure; verify that running from a non-`main` branch returns an error before any other git mutations

## 3. Release Command — Wire It Together

- [x] 3.1 In `internal/app/commands.go`: update `runRelease` to: (1) require `opts.Version` non-empty (return error if missing), (2) call `manifest.StampManifestFile(opts.ManifestPath, opts.Version)`, (3) call `runGitRelease(opts.ManifestPath, opts.Version)`, (4) build+push images using the existing multi-arch logic with `Tags: [repo:version, repo:latest]`
- [x] 3.2 Remove any remaining call to `DetectGitVersion()` from `runRelease`

## 4. Spec Sync

- [x] 4.1 Apply MODIFIED `image-release-versioning` spec to `openspec/specs/image-release-versioning/spec.md`
- [x] 4.2 Apply MODIFIED `local-control-plane-cli` spec to `openspec/specs/local-control-plane-cli/spec.md`

## 5. Verification

- [x] 5.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 5.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 5.3 Smoke test: run `vcpe release --help` and verify `--version` is shown as required; run `vcpe release --manifest manifests/example.yaml` (no `--version`) and verify it fails with a clear error
