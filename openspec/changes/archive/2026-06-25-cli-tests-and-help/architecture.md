## Overview

This change adds two tightly coupled capabilities to the `vcpe` control plane CLI:

1. **Extensive `--help` / `-h` system** — structured, command-specific help text accessible at every level of the command grammar. No operator should have to provoke an error to discover the interface.
2. **Full command-layer test coverage** — unit tests for every command path (parsing, routing, state mutations, JSON output) at the `internal/app` boundary, plus golden-file coverage for all help output.

The change is entirely within `controlplane/internal/app/`. No new dependencies. No protocol changes. No changes to the manifest schema or persist layer.

---

## Components

- **`internal/app/help.go`** (new): Structured `CommandHelp` registry. Owns `GlobalHelp()`, `HelpFor(command)`, and the `commandHelp` map. This is the single source of truth for what every command does, what flags it takes, what positional arguments it expects, and example invocations.

- **`internal/app/cli.go`** (modified): `parseArgs` gains `-h` / `--help` interception before `validateCommandShape` runs. Returns `flag.ErrHelp` on match. `ExecuteCLI` in `execute.go` checks for this sentinel and prints help, exiting 0.

- **`internal/app/execute.go`** (modified): `ExecuteCLI` wraps the call to `parseArgs` with an `errors.Is(err, flag.ErrHelp)` check, prints help text, and returns `nil` (exit 0).

- **`internal/app/help_test.go`** (new): Tests that every command in `topLevelCommands` has a `commandHelp` entry; golden-file tests for `HelpFor()` and `GlobalHelp()` output; tests that `-h` and `--help` produce exit 0.

- **`internal/app/testdata/help/*.golden`** (new): Committed golden files for each command's expected help text. Updated via `go test -run TestHelp -update`.

- **Expanded `commands_test.go`**: `runBuild`, `runPlan`, `runDown` (with and without snapshot), `runLogs` with `--name`, JSON output schema conformance.

- **Expanded `apply_test.go`**: `--allow-disruptive` gate (should reject when disruptive but flag absent), JSON output key validation.

---

## Key Architectural Decisions

### Help sentinel: `flag.ErrHelp` (stdlib) vs. custom error type
**Choice**: `flag.ErrHelp` from the standard library (`"flag"` package).  
**Rationale**: It is already the canonical Go sentinel for "user asked for help, not an error." `ExecuteCLI` performs `errors.Is(err, flag.ErrHelp)` — no type assertion, no new type, no import beyond stdlib. Zero added complexity.  
**Alternatives considered**: Custom `type HelpRequested struct{command string}` — adds a type that callers must know about; rejected because `flag.ErrHelp` is already the established Go convention. Returning help text directly in the response struct — rejected because it conflates parsing errors with successful command resolution.

### Help data shape: structured `CommandHelp` map vs. embedded text files vs. inline strings
**Choice**: Structured `map[string]CommandHelp` in `help.go` where each entry owns `Synopsis`, `Description`, `Positionals []string`, `RequiredFlags []FlagHelp`, `OptionalFlags []FlagHelp`, and `Examples []string`.  
**Rationale**: The map is machine-readable — `help_test.go` can assert that every command in `topLevelCommands` has an entry (detects drift when a new command is added but no help is written). Embedded text files require manual formatting with no drift detection. Inline strings are not testable at the component level.  
**Alternatives considered**: `//go:embed help/*.txt` per command — rejected because text files can't be cross-referenced against `topLevelCommands`; drift is silent. Cobra/urfave-cli framework — rejected; it requires restructuring all 11 command handlers and adds a large external dependency for no user-visible improvement over the custom approach.

### `-h` / `--help` interception point: before vs. after `validateCommandShape`
**Choice**: Intercept in `parseArgs` **before** `validateCommandShape` runs.  
**Rationale**: `vcpe up --help` should show help even though `--manifest` is missing. Help is not a command invocation; it must not be blocked by required-flag validation. Any other interception point produces confusing `"up requires --manifest"` errors when the user is just asking for help.  
**Alternatives considered**: Intercept in `ExecuteCLI` after full parse — rejected; `validateCommandShape` would fire first, producing error output before the help check.

### Image backend for `runBuild` tests: dependency injection vs. env-var skip
**Choice**: `VCPE_SKIP_IMAGE=1` environment variable causes `runBuild` to skip actual image lifecycle, consistent with the existing `VCPE_SKIP_HOSTNET_PREFLIGHT=1` pattern.  
**Rationale**: Dependency injection (making `newImageBackend` injectable via `Options`) is over-engineered for a unit test boundary that already has an established env-var convention. Adding a field to `Options` solely for test-time substitution couples the production type to test scaffolding.  
**Alternatives considered**: `image.Backend` field on `Options` — rejected; the `Options` struct is a parsed invocation, not a dependency container. Build tag — rejected; it creates a separate compilation path that can mask real build errors.

### Golden files: location and update mechanism
**Choice**: `internal/app/testdata/help/<command>.golden`, created on first run, updated via `go test ./internal/app/ -run TestHelp -update` (standard `flag.Bool("update", ...)` pattern).  
**Rationale**: Per-Go convention, `testdata/` directories are excluded from compilation and included in test binary working directory. Golden files are committed — PRs carry diffs of help text changes, preventing silent regressions.  
**Alternatives considered**: Inline string literals in tests — rejected; a 40-line help string as a Go string literal is a maintenance burden and produces unreadable diffs. Snapshot testing frameworks — rejected; no existing dependency; the pattern is trivially implemented in ~30 LOC.

### Test file organisation: new files vs. expand existing
**Choice**: One new file (`help_test.go`); expand existing `commands_test.go` and `apply_test.go`.  
**Rationale**: `help_test.go` is conceptually distinct (tests a new component). The remaining gaps — `runBuild`, `runPlan`, `runDown`, `runLogs --name`, JSON schema — are command-layer tests that belong with the existing command tests. Proliferating files fragments coverage and makes it harder to see what is and isn't tested.  
**Alternatives considered**: Separate `build_test.go`, `plan_test.go`, etc. — rejected; the tests are short, the existing files are not oversized, and co-location by concern is cleaner.

---

## Data Flow

### `--help` request

```
User invokes: vcpe up --help
                │
                ▼
         ExecuteCLI(prog, args)
                │
                ▼
         parseArgs(prog, args)
         ┌──────────────────────────────────────┐
         │  scan for -h / --help anywhere       │
         │  in global or sub-command args       │
         │  → return flag.ErrHelp               │
         └──────────────────────────────────────┘
                │ flag.ErrHelp
                ▼
         ExecuteCLI:
           errors.Is(err, flag.ErrHelp)?
           YES → HelpFor(command)    GlobalHelp() (if no command)
                 fmt.Print(text)
                 return nil  (exit 0)
           NO  → return err (exit 1)
```

### Command test path

```
Test calls: executeLocal(Options{Command:"build", StateRoot:tmpDir, ...})
                    │
                    ▼
            types.Register()  (idempotent)
                    │
                    ▼
            runBuild(opts)
              │
              ├─ VCPE_SKIP_IMAGE=1 → skip image.Manager.BuildWithOptions
              │                       return stub summary
              └─ else              → newImageBackend() → real podman
                    │
                    ▼
            daemon.CommandResponse{Message: "build complete..."}
```

---

## Integration Points

- **`topLevelCommands` map (`cli.go`)**: `commandHelp` map in `help.go` must have an entry for every key. `help_test.go` enforces this with `TestHelpCoverage`.

- **`parseArgs` (`cli.go`)**: Gains `-h` / `--help` interception. The check runs after the command token is extracted (so `HelpFor(command)` can be called) but before `validateCommandShape` (so missing required flags do not block help).

- **`ExecuteCLI` (`execute.go`)**: Adds `errors.Is(err, flag.ErrHelp)` branch between `parseArgs` and the daemon/local dispatch.

- **`VCPE_SKIP_IMAGE`**: Read in `runBuild` (and potentially `runApply`'s image phase). Consistent with the existing `VCPE_SKIP_HOSTNET_PREFLIGHT` and `VCPE_FAIL_PHASE` conventions.

- **`testdata/help/`**: Go's test runner makes the package directory the working directory, so `os.ReadFile("testdata/help/up.golden")` works without path manipulation.

---

## Security Model

This change touches only UX and test infrastructure. No trust boundaries change. No secrets, credentials, or network calls are introduced.

- Help text is static strings — no dynamic content, no user input reflected.
- `VCPE_SKIP_IMAGE` is a test-only env var with no privilege implications (it only suppresses a build/pull call; it does not bypass auth or write state).
- Golden test files are read-only at test time; the `-update` path writes to `testdata/` under the source tree, not to runtime state.

---

## Error Handling Strategy

### Help vs. parse error distinction
`flag.ErrHelp` is the sentinel for "user asked for help." All other errors from `parseArgs` are genuine parse failures and propagate normally (non-zero exit).

### Missing help entry (drift)
`TestHelpCoverage` fails the build if `commandHelp` is missing an entry for any command in `topLevelCommands`. This is a compile-time-equivalent check — the failure is caught in CI before merge, not at runtime.

### Golden file missing
If a `.golden` file doesn't exist and `-update` is not set, the test fails with a clear message: `"golden file testdata/help/<command>.golden not found; run with -update to create"`. First-time setup requires one `go test -update` run.

### `VCPE_SKIP_IMAGE` in apply pipeline
If `VCPE_SKIP_IMAGE=1` is set and the apply pipeline reaches the image phase, the phase records "succeeded (skipped)" to the journal and continues. This is intentionally visible in the operation log so test failures aren't silently masked.

---

## Observability Strategy

Help requests are not logged — they are stateless UX interactions with no side effects. No observability needed.

For new tests:
- Failed tests produce `t.Fatalf` messages with the full `opts` value and actual vs. expected output — no additional instrumentation needed.
- `apply_test.go` tests that use `VCPE_FAIL_PHASE` already assert on the operation journal (`ps.OperationPhases`), confirming the phase was recorded with the correct status. New tests follow this pattern.

---

## Constraints

- **No new external dependencies.** `flag.ErrHelp` is stdlib. All test helpers are in `internal/app/` or `testdata/`.
- **`Options` struct is not a DI container.** Backend substitution is done via env vars, not struct fields.
- **Help text is English prose only.** No i18n, no templating, no runtime-variable substitution in help strings.
- **`commandHelp` map is the only source of help text.** Do not duplicate usage strings in `validateCommandShape` error messages — point users to `vcpe <command> --help` instead.
- **Test boundary at Podman.** Unit tests never invoke `podman` or `podman-compose`. `VCPE_SKIP_HOSTNET_PREFLIGHT=1` and `VCPE_SKIP_IMAGE=1` mark the boundary. Integration tests in `tests/smoke/` cover the rest.
- **Golden files are committed.** They are not generated artifacts; a PR that changes help text must carry a golden file diff.

---

## Diagrams

### Component layout

```
internal/app/
  ├── cli.go              ← parseArgs (modified: -h/-h interception)
  ├── execute.go          ← ExecuteCLI (modified: flag.ErrHelp branch)
  ├── help.go             ← NEW: CommandHelp registry, HelpFor(), GlobalHelp()
  ├── commands.go         ← runBuild, runPlan, runDown (VCPE_SKIP_IMAGE added to runBuild)
  ├── local.go            ← runStatus, runLogs, runConfig, runState
  ├── orchestrator.go     ← runApply pipeline
  │
  ├── help_test.go        ← NEW: coverage + golden tests
  ├── commands_test.go    ← EXPANDED: runBuild, runPlan, runDown, runLogs
  ├── apply_test.go       ← EXPANDED: --allow-disruptive, JSON output
  ├── cli_test.go         ← existing (no changes needed)
  └── testdata/
        └── help/
              ├── up.golden
              ├── down.golden
              ├── plan.golden
              ├── build.golden
              ├── status.golden
              ├── logs.golden
              ├── service.golden
              ├── config.golden
              ├── state.golden
              └── init.golden
```

### `--help` sequence

```
vcpe up --help
   │
   ├─ parseArgs detects --help in sub-args
   │    returns (Options{Command:"up"}, flag.ErrHelp)
   │
   └─ ExecuteCLI
         errors.Is(flag.ErrHelp) → true
         fmt.Print(HelpFor("up"))
         return nil
         ──────────────────────
         process exits 0
```

### Help data → output rendering

```
commandHelp["up"]  ──────────────────────────────────────────┐
  Synopsis:  "Bring up a deployment from a v1 manifest"       │
  Positionals: []                                              │
  Flags:                                                       │
    --manifest <path>  Required  "Path to manifest YAML"       │   HelpFor("up")
    --allow-disruptive  Bool     "Allow CIDR/scale changes"    ├──────────────────▶
    --state-root <path>  Optional "Override state root"        │   Formatted string
    --json  Bool  "Emit structured JSON"                       │   printed to stdout
  Examples:                                                    │
    vcpe up --manifest ./manifest-bng-7.yaml                   │
    vcpe up --manifest ./manifest.yaml --allow-disruptive      │
                                                              ─┘
```
