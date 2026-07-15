## Why

The `vcpe` CLI has no `--help` system — operators discover commands and flags only by provoking errors. Additionally, large sections of the command layer (`build`, `plan`, `down`, compose lifecycle) have no unit tests, making regressions invisible. Both gaps are blocking production readiness.

## What Changes

- **NEW**: `internal/app/help.go` — structured `CommandHelp` registry with `HelpFor(command)` and `GlobalHelp()` functions
- **NEW**: `-h` / `--help` flag support at every command level (global and per-command), returning exit 0
- **NEW**: `VCPE_SKIP_IMAGE=1` environment variable — swaps in a no-op image backend for unit testing (consistent with `VCPE_SKIP_HOSTNET_PREFLIGHT`)
- **NEW**: `internal/app/help_test.go` — coverage and golden-file tests for all help output
- **NEW**: `internal/app/testdata/help/*.golden` — committed golden files for all 10 commands plus global help
- **MODIFIED**: `parseArgs` in `cli.go` — upfront full-args scan for `-h`/`--help` before validation
- **MODIFIED**: `ExecuteCLI` in `execute.go` — `flag.ErrHelp` branch (print help, return nil)
- **MODIFIED**: `newImageBackend()` in `imagebackend.go` — env-var-aware backend selection
- **MODIFIED**: `validateCommandShape` in `cli.go` — error messages append `; run \`vcpe <command> --help\` for usage`
- **EXPANDED**: `commands_test.go` — `runBuild`, `runPlan`, `runDown`, `runLogs` tests
- **EXPANDED**: `apply_test.go` — `--allow-disruptive` gate, JSON output key validation

## Capabilities

### New Capabilities

- `cli-help-system`: Structured per-command and global help text accessible via `-h`/`--help` at any position in the argument list; alias redirect; committed golden files

### Modified Capabilities

- `local-control-plane-cli`: CLI now supports `-h`/`--help` flag (was error-driven only); error messages now include help pointers; `VCPE_SKIP_IMAGE` test environment variable added to the documented test surface

## Impact

- **Files modified**: `internal/app/cli.go`, `internal/app/execute.go`, `internal/app/imagebackend.go`, `internal/app/commands_test.go`, `internal/app/apply_test.go`
- **Files created**: `internal/app/help.go`, `internal/app/help_test.go`, `internal/app/testdata/help/*.golden` (11 files)
- **No new external dependencies**: `flag.ErrHelp` is stdlib
- **No protocol changes**: daemon `CommandRequest`/`CommandResponse` unchanged
- **No manifest schema changes**
- **No persist layer changes**
