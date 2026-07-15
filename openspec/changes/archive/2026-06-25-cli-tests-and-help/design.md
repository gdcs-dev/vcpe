## Context

Architecture and all key decisions are documented in `architecture.md` and `decisions.md`. This document covers implementation-level details those artifacts do not address.

The `internal/app/` package is split across `cli.go` (parsing), `execute.go` (entrypoint), `local.go` (command dispatch), `commands.go` (build/plan/down), `orchestrator.go` (apply pipeline), `imagebackend.go` (backend adapter), and `preflight.go`/`disruptive.go` (validation). All test files are in `package app` (not `package app_test`), so unexported symbols are accessible.

## Goals / Non-Goals

**Goals:**
- Ship `help.go` with `CommandHelp` registry, `HelpFor()`, `GlobalHelp()`, and `flag.ErrHelp` integration
- Ship `-h`/`--help` support at every argument position
- Ship `noopImageBackend` behind `VCPE_SKIP_IMAGE=1`
- Ship `help_test.go` with golden files and `TestHelpCoverage`
- Expand `commands_test.go` and `apply_test.go` to cover all untested command paths

**Non-Goals:**
- Shell completion
- Localisation / i18n
- Man page generation
- Test coverage for Podman-facing phases (those belong in smoke tests)
- Daemon path unit tests

## Decisions

### `parseArgs` change structure

The upfront scan is inserted as the **first statement** in `parseArgs`, before the `len(args) == 0` check:

```go
func parseArgs(_ string, args []string) (Options, error) {
    // Upfront help scan — must run before any validation.
    if cmd, ok := extractHelpCommand(args); ok {
        return Options{Command: cmd}, flag.ErrHelp
    }
    // existing: len(args) == 0 check ...
}

// extractHelpCommand scans args for -h/--help. Returns the resolved primary
// command name and true if help was requested; empty string and false otherwise.
func extractHelpCommand(args []string) (string, bool) { ... }
```

`extractHelpCommand` skips values of the three value-taking global flags (`--state-root`, `--config`, `--socket`) using a simple one-step lookahead.

### `ExecuteCLI` change structure

```go
opts, err := parseArgs(prog, args)
if errors.Is(err, flag.ErrHelp) {
    if opts.Command == "" {
        fmt.Print(GlobalHelp())
    } else {
        fmt.Print(HelpFor(opts.Command))
    }
    return nil
}
if err != nil {
    return err
}
```

`fmt.Print` (not `fmt.Println`) — `HelpFor` and `GlobalHelp` include the trailing newline.

### `noopImageBackend` placement

Lives in `imagebackend.go` alongside `podmanImageBackend`. `newImageBackend()` becomes:

```go
func newImageBackend() image.Backend {
    if os.Getenv("VCPE_SKIP_IMAGE") == "1" {
        return noopImageBackend{}
    }
    return podmanImageBackend{adapter: podman.New()}
}
```

`noopImageBackend.ImageExists` returns `true` (image "exists") so the default `build-if-missing` policy produces `action:"noop"` rather than triggering a build.

### Error message update scope

Only `validateCommandShape` errors that mention a specific missing required flag get the hint appended. Generic unknown-command errors do not (they already carry migration hints). Affected messages:
- `"<cmd> requires --manifest <path>"` → append `"; run \`vcpe <cmd> --help\` for usage"`
- `"service down requires --name"` → append hint
- `"service down requires --force"` → append hint
- `"destroy requires --force"` → append hint

### Golden test boilerplate

```go
var update = flag.Bool("update", false, "update golden files")

func checkGolden(t *testing.T, name, got string) {
    t.Helper()
    path := filepath.Join("testdata", "help", name+".golden")
    if *update {
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
            t.Fatalf("mkdir: %v", err)
        }
        if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
            t.Fatalf("write golden: %v", err)
        }
        return
    }
    want, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        t.Fatalf("golden file %s not found; run: go test ./internal/app/ -run TestHelp -update", path)
    }
    if err != nil {
        t.Fatalf("read golden: %v", err)
    }
    if got != string(want) {
        t.Errorf("output mismatch for %s:\ngot:\n%s\nwant:\n%s", name, got, want)
    }
}
```

## Risks / Trade-offs

- **Golden file churn**: Any change to help text requires a golden update pass. Mitigated by the explicit `-update` flag and the fact that help text changes should be intentional.
- **`writeV1Manifest` coupling**: `TestApplyAllowDisruptiveGate` depends on `writeV1Manifest` defined in `apply_test.go`. If that helper changes signature, new tests break. Mitigated by the cross-file dependency note in `decisions.md` D16.
- **`VCPE_SKIP_IMAGE` in production binary**: The env var is present in the production binary, not just tests. This is consistent with `VCPE_SKIP_HOSTNET_PREFLIGHT` and is acceptable for a local dev tool where operators control the environment.
