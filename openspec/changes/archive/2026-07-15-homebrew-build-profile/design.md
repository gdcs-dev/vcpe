## Context

The `vcpe` binary exposes all commands to all users regardless of how it was installed. Developer-only commands (`build`, `push`, `release`) are meaningful only to vCPE project contributors who have Docker buildx, registry credentials, and a git checkout — not to end users who install via Homebrew to run a vCPE lab.

The Go toolchain's build tag system provides a clean, binary-level mechanism for this split. The Homebrew formula already compiles from source, so passing `-tags homebrew` is a minimal formula change.

## Goals / Non-Goals

**Goals:**
- `build`, `push`, and `release` are absent from Homebrew-installed binaries — not just disabled, but genuinely not present
- All non-Homebrew builds (source checkout, CI) are unaffected
- The Homebrew formula has no per-command knowledge; it only passes `-tags homebrew`
- The excluded commands' flag flags (`--no-cache`, `--platform`, `--backend`, `--version`) are also absent from the Homebrew binary (cleaned up from validation guards)

**Non-Goals:**
- Runtime disabling or "not available in this distribution" messages
- Excluding any command other than `build`, `push`, `release`
- Changing behavior for non-Homebrew builds

## Decisions

### Decision: Stub/full file pair for the developer command surface

All developer command code is consolidated in a file pair:

| File | Build tag | Content |
|------|-----------|---------|
| `developer_commands.go` | `//go:build !homebrew` | `init()` registrations, `dispatchDeveloperCommand`, `runBuild`, `runPush`, plus moves `runRelease`/git helpers from their current files |
| `developer_commands_stub.go` | `//go:build homebrew` | `dispatchDeveloperCommand` that returns `fmt.Errorf("command %q is not executable", opts.Command)` |

The dispatch switch in `local.go` gets a single new `default` fallthrough to `dispatchDeveloperCommand(opts)`. The three `case "build":`, `case "push":`, `case "release":` entries are removed from `local.go`.

**Why not separate tagged files per command?** Three files (build_command.go, push_command.go, release_command.go) plus three stubs would be six files. One pair is simpler to maintain and makes the "developer surface" conceptually cohesive.

**Why move `runRelease` here?** `release.go` currently holds `runRelease`, `runGitRelease`, `gitReleasePreflight`. These are all developer-only. Moving them into `developer_commands.go` keeps all excluded code in one place and eliminates `release.go` entirely.

---

### Decision: `init()` for `topLevelCommands` and `commandHelp` registration

`topLevelCommands` and `commandHelp` are currently package-level map literals. They need to be mutable at `init()` time so the developer file pair can add/omit its entries.

```go
// cli.go — base map without developer commands
var topLevelCommands = map[string]struct{}{
    "init": {}, "up": {}, "apply": {}, "plan": {}, "down": {}, "destroy": {},
    "list": {}, "manifest": {}, "status": {}, "logs": {}, "config": {},
    "state": {}, "version": {},
}

// developer_commands.go (//go:build !homebrew)
func init() {
    topLevelCommands["build"] = struct{}{}
    topLevelCommands["push"] = struct{}{}
    topLevelCommands["release"] = struct{}{}
}
```

Same pattern for `commandHelp` in `help.go` and the `order` slice in `GlobalHelp()`.

For `GlobalHelp()`'s `order` slice, introduce a `developerCommandOrder []string` variable (empty by default, populated by `init()` in the developer file) that is inserted into the order slice.

---

### Decision: Flag guards cleaned up, not tag-guarded

The flag validation guards in `cli.go` that reference `build`, `push`, `release` (e.g., `--no-cache only supported for build`) become dead code on Homebrew builds since those commands aren't in `topLevelCommands` and can't be reached. Rather than tag-guarding them, simply remove the references:
- `--no-cache` guard: becomes `command != "build"` — this is a no-op on Homebrew since "build" is unknown
- Actually cleaner: remove the guards' references to excluded commands since they can't be reached

The validation path `validateCommandShape` and `isManifestCommand` similarly lose their references to the excluded commands.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Test coverage: existing tests may import `runBuild`/`runPush`/`runRelease` directly | Move functions to `developer_commands.go`; tests run under `!homebrew` (default) so they compile normally |
| `TestHelpCoverage` asserts every `topLevelCommands` key has a `commandHelp` entry | Both maps are populated by the same `init()`, so coverage remains 1:1 |
| `release.go` is deleted — tests that reference it must be updated | `release_test.go` tests `gitReleasePreflight`; these stay in a `developer_commands_test.go` also tagged `//go:build !homebrew` |

## Open Questions

None — all decisions are made.
