## 1. Mutable Registration Maps

- [x] 1.1 In `internal/app/cli.go`: remove `"build"`, `"push"`, `"release"` from the `topLevelCommands` map literal; leave all other commands unchanged
- [x] 1.2 In `internal/app/cli.go`: remove `release` from `isManifestCommand()` (already handled in validate); remove `"build"`, `"push"`, `"release"` from `validateCommandShape`'s manifest-required case; remove `case "release":` from `validateCommandShape`; remove all flag-guard references to the three excluded commands (`--no-cache`, `--platform`, `--backend`, `--version` guards)
- [x] 1.3 In `internal/app/help.go`: remove the `"build"`, `"push"`, `"release"` entries from the `commandHelp` map literal; remove them from the `order` slice in `GlobalHelp()`; add a package-level `developerCommandOrder []string` variable (empty slice) that `GlobalHelp()` appends at the front of the order after `"init"`
- [x] 1.4 In `internal/app/local.go`: remove `case "build":`, `case "push":`, `case "release":` from the dispatch switch; add `default: return dispatchDeveloperCommand(opts)` in place of the existing default

## 2. Developer Command Files

- [x] 2.1 Create `internal/app/developer_commands.go` with `//go:build !homebrew`: contains `func init()` that adds `"build"`, `"push"`, `"release"` to `topLevelCommands`, adds their `CommandHelp` entries to `commandHelp`, and sets `developerCommandOrder = []string{"build", "push", "release"}`; contains `func dispatchDeveloperCommand(opts Options) (daemon.CommandResponse, error)` with cases for all three
- [x] 2.2 Move `runBuild`, `runPush` from `commands.go` into `developer_commands.go`
- [x] 2.3 Move `runRelease`, `runGitRelease`, `gitReleasePreflight` from `release.go` (and `commands.go`) into `developer_commands.go`; delete `release.go`
- [x] 2.4 Create `internal/app/developer_commands_stub.go` with `//go:build homebrew`: contains `func dispatchDeveloperCommand(opts Options) (daemon.CommandResponse, error)` that returns `fmt.Errorf("command %q is not executable", opts.Command)`

## 3. Tests

- [x] 3.1 Move `release_test.go` content into `developer_commands_test.go` with `//go:build !homebrew`; delete `release_test.go`
- [x] 3.2 Update any tests that reference `runBuild` or `runPush` directly to account for their new location
- [x] 3.3 Regenerate golden help files: `go test ./internal/app/ -run TestHelp -update`
- [x] 3.4 Run `go build -tags homebrew ./...` and verify it compiles; verify `vcpe build`, `vcpe push`, `vcpe release` are not in the help output

## 4. Homebrew Formula

- [x] 4.1 In `scripts/homebrew-tap`: add `-tags homebrew` to the `go build` invocation in `render_formula()`

## 5. Spec Sync

- [x] 5.1 Apply MODIFIED `local-control-plane-cli` spec to `openspec/specs/local-control-plane-cli/spec.md`
- [x] 5.2 Apply MODIFIED `developer-readme-and-build-workflow` spec to `openspec/specs/developer-readme-and-build-workflow/spec.md`

## 6. Verification

- [x] 6.1 Run `cd controlplane && go build ./...` — default build succeeds with all commands present
- [x] 6.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 6.3 Run `cd controlplane && go build -tags homebrew -o bin/vcpe-homebrew ./cmd/vcpe` — Homebrew build succeeds
- [x] 6.4 Verify `./bin/vcpe-homebrew build` returns "unknown command"; verify `./bin/vcpe build --help` still works in the default binary
