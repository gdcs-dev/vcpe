## 1. Help data and rendering (`help.go`)

- [x] 1.1 Create `internal/app/help.go` with `type FlagHelp struct { Name, Arg, Description string }` and `type CommandHelp struct { Synopsis, Description string; Positionals []string; RequiredFlags, OptionalFlags []FlagHelp; Examples []string }`
- [x] 1.2 Add `var commandHelp = map[string]CommandHelp` with entries for all 10 primary commands: `init`, `build`, `up`, `plan`, `down`, `status`, `logs`, `service`, `config`, `state`
- [x] 1.3 Implement `func GlobalHelp() string` — command table format with synopsis column and global flags section
- [x] 1.4 Implement `func HelpFor(command string) string` — per-command format with required/optional split and examples; aliases (`apply`, `destroy`) return a one-line redirect message

## 2. `-h`/`--help` flag interception (`cli.go`, `execute.go`)

- [x] 2.1 Add `func extractHelpCommand(args []string) (string, bool)` in `cli.go` — upfront scan for `-h`/`--help` with one-step lookahead to skip values of `--state-root`, `--config`, `--socket`; resolve aliases to primary command
- [x] 2.2 Insert `extractHelpCommand` call as the **first statement** in `parseArgs`, returning `(Options{Command: cmd}, flag.ErrHelp)` when matched
- [x] 2.3 Add `errors.Is(err, flag.ErrHelp)` branch in `ExecuteCLI` (`execute.go`): call `GlobalHelp()` or `HelpFor(opts.Command)`, `fmt.Print`, return `nil`

## 3. Error message help pointers (`cli.go`)

- [x] 3.1 Append `; run \`vcpe <command> --help\` for usage` to the four required-flag error messages in `validateCommandShape`: missing `--manifest`, `service down` missing `--name`, `service down` missing `--force`, `destroy` missing `--force`

## 4. No-op image backend (`imagebackend.go`)

- [x] 4.1 Add `type noopImageBackend struct{}` in `imagebackend.go` implementing all 5 methods of `image.Backend` (`ImageExists→true`, mutating methods→`nil`)
- [x] 4.2 Update `newImageBackend()` to return `noopImageBackend{}` when `VCPE_SKIP_IMAGE=1`

## 5. Help tests and golden files (`help_test.go`, `testdata/`)

- [x] 5.1 Create `internal/app/help_test.go` with `var update = flag.Bool("update", false, "update golden files")` and `func checkGolden(t, name, got)` helper
- [x] 5.2 Add `TestHelpCoverage` — iterates `topLevelCommands`, asserts each key has a `commandHelp` entry
- [x] 5.3 Add `TestHelpGlobal` — calls `GlobalHelp()`, runs `checkGolden(t, "global", got)`
- [x] 5.4 Add per-command golden tests
- [x] 5.5 Add `TestHelpAliasRedirects` — `HelpFor("apply")` contains `"alias for up"`, `HelpFor("destroy")` contains `"alias for down"`
- [x] 5.6 Add `TestHelpFlagExitsZero` — `ExecuteCLI("vcpe", []string{"--help"})` returns `nil`
- [x] 5.7 Run `go test ./internal/app/ -run TestHelp -update` to generate all golden files

## 6. Command tests — expand `commands_test.go`

- [x] 6.1 Add `TestBuildReportsSummary` — sets `VCPE_SKIP_IMAGE=1` and `VCPE_SKIP_HOSTNET_PREFLIGHT=1`, calls `executeLocal` with `build` + manifest path, asserts output starts with `"build complete for deployment"`
- [x] 6.2 Add `TestPlanShowsNetworksAndServices` — calls `executeLocal` with `plan` + manifest path, asserts output contains `"networks:"` and `"services:"`
- [x] 6.3 Add `TestPlanDisruptiveGate` — seeds a WAN CIDR lease for `"edge"`, writes manifest with different WAN CIDR via in-test mutation, calls `plan`, asserts output contains `"disruptive: yes"`
- [x] 6.4 Add `TestDownClearsLeases` — seeds lease + desired snapshot for `"edge"`, sets `VCPE_SKIP_HOSTNET_PREFLIGHT=1`, calls `down` with `Name:"edge"`, asserts `ListIPAMLeases` returns empty
- [x] 6.5 Add `TestLogsWithNameShowsDeployment` — calls `executeLocal` with `logs` + `Name:"edge"`, asserts output contains `"deployment=edge"`

## 7. Apply tests — expand `apply_test.go`

- [x] 7.1 Add `TestApplyAllowDisruptiveGate` — seeds WAN CIDR lease for `"edge"`, writes manifest with different WAN CIDR via in-test mutation, calls `apply` without `AllowDisruptive`, asserts error contains `"disruptive"`
- [x] 7.2 Add `TestApplyStatusJSONKeys` — calls `executeLocal` with `status` + `OutputJSON:true`, unmarshals response to `map[string]any`, asserts keys `"metrics"`, `"timeline"`, `"desired"`, `"planned"`, `"observed"`, `"runtimeInitDiagnostics"` are all present

## 8. Validation

- [x] 8.1 Run `go build ./...` — confirm clean compile
- [x] 8.2 Run `go test ./internal/app/...` — confirm all tests pass including new ones
- [x] 8.3 Manually run `./controlplane/bin/vcpe --help` and `./controlplane/bin/vcpe up --help`
- [x] 8.4 Manually run `./controlplane/bin/vcpe up` (no flags) and verify error message includes help pointer
