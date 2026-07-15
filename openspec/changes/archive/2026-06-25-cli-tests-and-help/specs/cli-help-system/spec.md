## ADDED Requirements

### Requirement: Per-command and global help text
The system SHALL provide structured help text for every top-level command and for the global `vcpe` invocation, accessible via `-h` or `--help` at any argument position, and SHALL exit with code 0 when help is requested.

#### Scenario: Global help lists all commands
- **WHEN** an operator runs `vcpe --help` or `vcpe -h`
- **THEN** the system prints a command table listing every top-level command with a one-line synopsis and the global flags, then exits 0

#### Scenario: Per-command help shows flags and examples
- **WHEN** an operator runs `vcpe <command> --help` or `vcpe <command> -h`
- **THEN** the system prints the command's usage line, description, required flags, optional flags, and at least one example invocation, then exits 0

#### Scenario: Help flag works after global flags
- **WHEN** an operator runs `vcpe --state-root /some/path up --help`
- **THEN** the system prints help for the `up` command and exits 0 (global flag values do not interfere with help detection)

#### Scenario: Alias help redirects to primary
- **WHEN** an operator runs `vcpe apply --help` or `vcpe destroy --help`
- **THEN** the system prints a one-line message identifying the alias and its primary command, then exits 0

#### Scenario: Service command help shows full grammar
- **WHEN** an operator runs `vcpe service --help`
- **THEN** the system prints a table of supported services, supported subcommands, and the flags each subcommand requires, then exits 0

### Requirement: Help coverage completeness
The system SHALL maintain a help entry for every command listed in the top-level command registry, and the test suite SHALL enforce this invariant automatically.

#### Scenario: Every command has a help entry
- **WHEN** the test suite runs `TestHelpCoverage`
- **THEN** the test fails if any command in `topLevelCommands` lacks a corresponding entry in the `commandHelp` registry

#### Scenario: Help output is stable across versions
- **WHEN** the test suite runs the golden-file help tests
- **THEN** the tests fail if any command's help output differs from its committed golden file in `testdata/help/`

### Requirement: Error messages include help pointer
The system SHALL append a help pointer (`; run \`vcpe <command> --help\` for usage`) to error messages that report a missing required flag, so operators are directed to structured help rather than having to rediscover the interface by trial and error.

#### Scenario: Missing required flag error includes help hint
- **WHEN** an operator runs a command without a required flag (e.g., `vcpe up` without `--manifest`)
- **THEN** the error message includes the specific missing flag AND a pointer to `vcpe <command> --help`
