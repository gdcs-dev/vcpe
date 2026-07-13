# Decisions: cli-tests-and-help

## BREAKING CHANGES

None. All changes are additive (new help system, new test files, new env var) or
strictly non-breaking modifications (appending help hints to existing error
messages, expanding existing test files).

---

## Decisions

### D1 — Top-level help format

**Choice**: Command-table format (Option A).

```
Usage: vcpe <command> [flags]

Commands:
  init     Initialize the vCPE state root
  build    Build service images from a manifest
  up       Bring up a deployment  (alias: apply)
  ...

Global flags:
  --state-root <path>   Override state root
  --config <path>       Config file path
  --socket <path>       Daemon socket path

Run `vcpe <command> --help` for command-specific help.
```

**Rationale**: Matches the convention of `kubectl`, `gh`, `docker`. Machine-scannable. Prose descriptions belong in per-command help.

---

### D2 — Per-command help format

**Choice**: Required/optional flag split, 1–2 sentence description, no defaults shown.

```
Usage: vcpe up --manifest <path> [flags]

Bring up a deployment from a v1 Deployment manifest. Reconciles networks,
images, IPAM allocation, and compose lifecycle in a single journaled operation.
Alias: apply

Required flags:
  --manifest <path>       Path to deployment manifest YAML

Optional flags:
  --allow-disruptive      Permit CIDR changes and scale-to-zero operations
  --state-root <path>     Override state root directory
  --socket <path>         Override daemon socket path
  --json                  Emit structured JSON output

Examples:
  vcpe up --manifest ./manifest-bng-7.yaml
  vcpe up --manifest ./manifest.yaml --allow-disruptive
```

**Rationale**: Required/optional split makes it immediately clear what is mandatory. Defaults are non-trivial XDG-derived paths — showing them adds noise without clarity.

---

### D3 — `CommandHelp` data structure

**Choice**: `map[string]CommandHelp` in `internal/app/help.go`.

```go
type CommandHelp struct {
    Synopsis     string      // one-line description for GlobalHelp table
    Description  string      // 1-2 sentence body for per-command help
    Positionals  []string    // e.g. ["<service>", "<subcommand>"]
    RequiredFlags []FlagHelp
    OptionalFlags []FlagHelp
    Examples     []string
}

type FlagHelp struct {
    Name        string // e.g. "--manifest"
    Arg         string // e.g. "<path>", empty for booleans
    Description string
}
```

Entries: `init`, `build`, `up`, `plan`, `down`, `status`, `logs`, `service`, `config`, `state`.

**Rationale**: Machine-readable — `TestHelpCoverage` can assert every key in `topLevelCommands` has an entry, catching drift when a new command is added without help.

---

### D4 — Alias handling

**Choice**: Aliases (`apply`→`up`, `destroy`→`down`) produce a one-line redirect, not a duplicate full entry.

`HelpFor("apply")` returns: `"apply is an alias for up — run \`vcpe up --help\` for usage"`

**Rationale**: Duplicate content creates a maintenance hazard. Redirect is consistent with `git`, `kubectl` alias behaviour. 10 golden files (one per primary command) rather than 14.

---

### D5 — `daemon` command visibility

**Choice**: `daemon` is hidden — no `commandHelp` entry, not listed in `GlobalHelp()`.

**Rationale**: `daemon` is an advanced deployment mechanism, not an operator workflow command. It would add noise to the primary help listing for the 99% case.

---

### D6 — `-h`/`--help` interception point

**Choice**: Scan **all** args as the very first operation in `parseArgs`, before any tokenisation.

Algorithm:
1. Scan args for `-h` or `--help` anywhere.
2. If found: extract command token (first non-flag, non-value arg, skipping values of `--state-root`/`--config`/`--socket`).
3. Resolve aliases to primary command name.
4. Return `(Options{Command: command}, flag.ErrHelp)`.

`ExecuteCLI` checks `errors.Is(err, flag.ErrHelp)`, calls `HelpFor(opts.Command)` or `GlobalHelp()`, prints, returns `nil` (exit 0).

**Rationale**: Per-position interception is fragile — `vcpe --state-root /x up --help` would miss `--help` if only checked in the sub-flag position. Upfront scan handles every position without per-command duplication.

---

### D7 — Help sentinel: `flag.ErrHelp`

**Choice**: Return `flag.ErrHelp` from the standard library `"flag"` package.

**Rationale**: Idiomatic Go sentinel for "user asked for help, not an error." No new types. `errors.Is(err, flag.ErrHelp)` requires no type assertion. Imported for the constant only — no `FlagSet` usage required.

---

### D8 — Error message help hints

**Choice**: Append `; run \`vcpe <command> --help\` for usage` to required-flag error messages in `validateCommandShape`.

**Rationale**: Keeps the specific error signal (what was wrong) while directing the user to structured help. The two are complementary. Replacing the message with only a pointer loses the specific error detail.

---

### D9 — `service` command help

**Choice**: Single flat help page (`service.golden`). No per-service or per-subcommand pages.

The page shows the full grammar table: services × subcommands × required flags.

**Rationale**: The grammar has only one level of nesting and a fixed set of services and subcommands. A recursive subcommand registry would be over-engineering for a fixed vocabulary.

---

### D10 — JSON output validation in tests

**Choice**: Assert required top-level key presence only (unmarshal to `map[string]any`, check keys exist).

`status --json` required keys: `"metrics"`, `"timeline"`, `"desired"`, `"planned"`, `"observed"`, `"runtimeInitDiagnostics"`.
`logs --json` required keys: `"timeline"`, `"runtimeInitDiagnostics"`.

**Rationale**: JSON output is an operator-facing diagnostic surface, not an API contract. Key-presence checks catch the important regressions without over-specifying shape. Typed struct binding breaks on any field rename.

---

### D11 — Golden files

**Choice**: `internal/app/testdata/help/<command>.golden`, one per primary command plus `global.golden`.

Update mechanism: `var update = flag.Bool("update", false, "update golden files")` in `help_test.go`. Invocation: `go test ./internal/app/ -run TestHelp -update`.

Missing golden file (no `-update`): fail with actionable message: `"golden file testdata/help/<command>.golden not found; run: go test ./internal/app/ -run TestHelp -update"`.

Files are committed to the repository. PRs that change help text carry explicit golden file diffs.

---

### D12 — Test naming convention

**Choice**: PascalCase sentence style, consistent with existing tests.

New test names:
- `TestHelpGlobal`
- `TestHelpForUp`, `TestHelpForDown`, `TestHelpForBuild`, `TestHelpForPlan`, `TestHelpForStatus`, `TestHelpForLogs`, `TestHelpForService`, `TestHelpForConfig`, `TestHelpForState`, `TestHelpForInit`
- `TestHelpAliasRedirects`
- `TestHelpCoverage`
- `TestHelpFlagExitsZero`
- `TestBuildReportsSummary`
- `TestPlanShowsNetworksAndServices`
- `TestPlanDisruptiveGate`
- `TestDownClearsLeases`
- `TestLogsWithNameShowsDeployment`
- `TestApplyAllowDisruptiveGate`
- `TestApplyStatusJSONKeys`

---

### D13 — `VCPE_SKIP_IMAGE` behaviour

**Choice**: When `VCPE_SKIP_IMAGE=1`, `newImageBackend()` returns a `noopImageBackend{}` instead of the real podman backend.

```go
type noopImageBackend struct{}
func (noopImageBackend) ImageExists(_ context.Context, _ string) (bool, error) { return true, nil }
func (noopImageBackend) BuildImage(_ context.Context, _ image.BuildRequest) error { return nil }
func (noopImageBackend) PullImage(_ context.Context, _ image.PullRequest) error  { return nil }
func (noopImageBackend) PushImage(_ context.Context, _ image.PushRequest) error  { return nil }
func (noopImageBackend) TagImage(_ context.Context, _ image.TagRequest) error    { return nil }
```

`image.Manager.BuildWithOptions` still runs and returns a real `Summary` with `action:"noop"`. Tests can assert the full output format.

**Rationale**: Preserves the real code path through `image.Manager` so tests exercise output formatting, not just command routing.

---

### D14 — `TestHelpFlagExitsZero` assertion

**Choice**: Assert `ExecuteCLI("vcpe", []string{"--help"}) == nil` only. No stdout capture.

**Rationale**: Content correctness is covered by golden file tests. Stdout redirect in parallel tests is unsafe without isolation primitives. `nil` return correctly captures "exits 0."

---

### D15 — `TestApplyAllowDisruptiveGate` manifest variant

**Choice**: Use in-test file mutation (call `writeV1Manifest`, read file, `strings.Replace` the WAN CIDR, write back). Consistent with `TestClassifyDisruptiveCIDRChange` in `commands_test.go`.

**Rationale**: No new helper needed. The mutation pattern is already established in the test suite. New helpers are only warranted when the same setup appears in 3+ tests.

---

### D16 — `writeV1Manifest` helper location

**Note** (not a decision, but a cross-file dependency to document): `writeV1Manifest(t *testing.T, name string) string` is defined in `apply_test.go`. It is accessible to `commands_test.go` and `help_test.go` because all three files are in `package app`. No export or move required.
