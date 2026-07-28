## 1. CLI: ManifestPaths and repeatable --manifest

- [x] 1.1 Add `ManifestPaths []string` field to `Options` in `cli.go`
- [x] 1.2 Update the CLI parser in `parseArgs` to accumulate repeated `--manifest` values into `ManifestPaths` for `stamp` and `release` commands; apply `filepath.Glob` expansion to each value; error if a non-glob path does not exist on disk
- [x] 1.3 Verify that all existing single-manifest commands (`up`, `down`, `plan`, etc.) continue to populate only `ManifestPath` (unchanged)
- [x] 1.4 Update CLI tests in `cli_test.go` to cover repeated `--manifest` and glob expansion

## 2. vcpe stamp command

- [x] 2.1 Add `runStamp` function in `developer_commands.go`: validate `--version` present; collect `ManifestPaths`; call `manifest.StampManifestFile` for each; report count of stamped manifests and files
- [x] 2.2 Register `stamp` in `topLevelCommands` and `developerCommandOrder` in `developer_commands.go`
- [x] 2.3 Add `stamp` to the stub list in `developer_commands_stub.go`
- [x] 2.4 Add help entry for `stamp` (synopsis, required/optional flags, examples) in `help.go`
- [x] 2.5 Add golden help file `testdata/help/stamp.golden`
- [x] 2.6 Add `TestHelpForStamp` in `developer_commands_test.go`
- [x] 2.7 Add unit tests for `runStamp` (single path, multi-path, glob expansion, empty glob, missing `--version`)

## 3. vcpe release: remove inline stamp, add coherence check

- [x] 3.1 Remove the `manifest.StampManifestFile` call from `runRelease`
- [x] 3.2 Add manifest collection to `runRelease`: use `ManifestPaths` if provided; otherwise run `git diff --name-only -- manifests/` and filter to `*.yaml` files
- [x] 3.3 Add coherence check: for each collected manifest, load it and verify every first-party service has `image.tag == version`; fail with a clear error (naming manifest + service) if not
- [x] 3.4 Update `runGitRelease` to accept `[]string` manifest paths and `git add` all of them in one commit
- [x] 3.5 Deduplicate image builds: collect `(repository, buildContext)` pairs across all manifests; build each unique pair once
- [x] 3.6 Update the release help entry and golden file to reflect updated behavior and flags

## 4. manifest.StampManifestFiles batch helper

- [x] 4.1 Add `StampManifestFiles(paths []string, version string) error` in `manifest/stamp.go` that iterates and calls `StampManifestFile` for each path
- [x] 4.2 Add unit tests for `StampManifestFiles` in `manifest/stamp_test.go` (empty list, single, multiple, one failing)
- [x] 4.3 `stampServices` also sets `image.pullPolicy: always-pull` for first-party services; test asserts this

## 5. Tests and validation

- [x] 5.1 Add integration-style test for `runStamp` → `runRelease` flow with two manifests and `VCPE_SKIP_RUNTIME=1`
- [x] 5.2 Verify coherence check test: mismatched tag on one manifest causes release to fail before git operations
- [x] 5.3 Verify auto-detect test: no `--manifest` on release discovers files from `git diff` output
- [x] 5.4 Run `go test ./...` and confirm all tests pass

## 6. Documentation

- [x] 6.1 Update `README.md` developer commands table: add `vcpe stamp` row; update `vcpe release` description to reflect the two-step workflow
- [x] 6.2 Update `docs/runbook.md` release workflow section: replace the single-command release instructions with the two-step `stamp` → `release` workflow, including examples with multiple `--manifest` flags and glob syntax
