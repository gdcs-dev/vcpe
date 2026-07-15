## 1. Binary Version Variable

- [x] 1.1 Add `var version = "dev"` to `controlplane/cmd/vcpe/main.go`
- [x] 1.2 Update `app.ExecuteCLI(os.Args[0], os.Args[1:])` call to `app.ExecuteCLI(os.Args[0], os.Args[1:], version)`

## 2. ExecuteCLI and Dispatch

- [x] 2.1 Update `ExecuteCLI(prog string, args []string, version string) error` signature in `internal/app/execute.go`; store version for dispatch
- [x] 2.2 Add `"version"` to `topLevelCommands` map in `internal/app/cli.go`
- [x] 2.3 Add `case "version"` to `executeLocal` dispatch in `internal/app/local.go` calling `runVersion(version)`
- [x] 2.4 Implement `runVersion(version string) (daemon.CommandResponse, error)` in `internal/app/commands.go` — returns `CommandResponse{Message: version}`

## 3. Help Text

- [x] 3.1 Add `"version"` entry to `commandHelp` in `internal/app/help.go` with synopsis `"Print the vcpe version"` and no required flags
- [x] 3.2 Add `TestHelpForVersion` test in `help_test.go`
- [x] 3.3 Update `internal/app/testdata/help/global.golden` to include `version` in the commands table
- [x] 3.4 Create `internal/app/testdata/help/version.golden`
- [x] 3.5 Run `go test ./internal/app/... -run TestHelp` — golden tests must pass

## 4. Makefile Version Embedding

- [x] 4.1 Add `GIT_VERSION` computation to `Makefile`: `$(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')`, defaulting to `dev`
- [x] 4.2 Update `build` target to pass `-ldflags "-s -w -X main.version=$(GIT_VERSION)"` to `go build`

## 5. Homebrew Formula Template

- [x] 5.1 Update `system "go", "build"` in `scripts/homebrew-tap` to pass `-ldflags "-s -w -X 'main.version=#{version}'"` in the formula template

## 6. Homebrew Release Channel Auto-detection

- [x] 6.1 Update `release)` case in `scripts/homebrew-tap`: when `VCPE_HOMEBREW_VERSION` is unset, run `git -C "$REPO_ROOT" describe --tags --abbrev=0` and strip leading `v`; fail with a clear error if no tags exist
- [x] 6.2 Update `release)` case: when `VCPE_HOMEBREW_SHA256` is unset, download the tagged archive and compute sha256 (same curl+shasum+mktemp pattern as the `main` and `development` cases)

## 7. sync-homebrew-vcpe Default Channel

- [x] 7.1 Change `export VCPE_HOMEBREW_CHANNEL=${VCPE_HOMEBREW_CHANNEL:-development}` to `release` in `scripts/sync-homebrew-vcpe`

## 8. Sync Spec

- [x] 8.1 Apply ADDED `vcpe-version-command` requirement to `openspec/specs/local-control-plane-cli/spec.md`
- [x] 8.2 Apply MODIFIED `developer-readme-and-build-workflow` requirements to `openspec/specs/developer-readme-and-build-workflow/spec.md`

## 9. Verification

- [x] 9.1 Run `cd controlplane && go build ./...` and `go test ./...` — must pass
- [x] 9.2 Run `cd controlplane && go build -ldflags "-X main.version=0.1.0" -o bin/vcpe ./cmd/vcpe && bin/vcpe version` — must print `0.1.0`
- [x] 9.3 Run `make build && controlplane/bin/vcpe version` — must print the latest git tag version (or `dev` if no tags)
- [x] 9.4 Verify `scripts/homebrew-tap formula` output contains `-X 'main.version=` in the ldflags
