## Why

The Homebrew-installed `vcpe` binary targets end users running a vCPE lab, not developers building or releasing the vCPE project itself. Commands like `build`, `push`, and `release` require Docker buildx, registry credentials, and git push access — none of which make sense in the context of a Homebrew install. Including them adds noise, potential for user confusion, and binary bloat.

## What Changes

- Add a `homebrew` Go build tag that excludes developer-only commands from the compiled binary.
- Commands excluded under `//go:build homebrew`: `build`, `push`, `release`.
- When a user on a Homebrew install runs one of these commands, the response is the standard "unknown command" error — the commands are genuinely absent from the binary.
- The Homebrew formula passes `-tags homebrew` to `go build`; the formula has no knowledge of which specific commands are excluded (that knowledge stays in the Go source).
- A stub/full file pair (`developer_commands.go` / `developer_commands_stub.go`) wraps the dispatch, help registration, and `topLevelCommands` entries for the excluded commands.

## Capabilities

### New Capabilities

_(none — this is a build-configuration change, not a new user-visible capability)_

### Modified Capabilities

- `local-control-plane-cli`: The `build`, `push`, and `release` commands are conditionally compiled. Under the `homebrew` build tag they are absent; under the default (non-tagged) build they remain fully available.
- `developer-readme-and-build-workflow`: The Homebrew formula section notes that `-tags homebrew` is passed and that developer-only commands are excluded from the Homebrew binary.

## Impact

- `controlplane/internal/app/cli.go`: Remove `"build"`, `"push"`, `"release"` from the `topLevelCommands` literal; move them to a new `developer_commands.go` via `init()`. Remove corresponding flag-guard references that are only reachable when these commands exist.
- `controlplane/internal/app/local.go`: Remove `case "build":`, `case "push":`, `case "release":` from the dispatch switch; route developer commands through a new `dispatchDeveloperCommand(opts)` function.
- `controlplane/internal/app/developer_commands.go` (new, `//go:build !homebrew`): full implementations — `init()` registrations for `topLevelCommands` and `commandHelp`, `dispatchDeveloperCommand`, `runBuild`, `runPush`, `runRelease`, git release helpers.
- `controlplane/internal/app/developer_commands_stub.go` (new, `//go:build homebrew`): stub — `dispatchDeveloperCommand` always returns error "unknown command".
- `controlplane/internal/app/help.go`: Remove `"build"`, `"push"`, `"release"` map entries and order slice entries from the base file; move them to `developer_commands.go`.
- `scripts/homebrew-tap`: Add `-tags homebrew` to the `go build` invocation in the formula template.
- No behavior changes for non-Homebrew builds.
