## ADDED Requirements

### Requirement: vcpe service types CLI command
The system SHALL provide a `vcpe service types` command that lists all registered service types. With `--json`, it SHALL emit a JSON array of type descriptor objects to stdout. Without `--json`, it SHALL print a human-readable table with columns NAME, DESCRIPTION, and PULL_POLICY. The command SHALL be registered under a new top-level `service` command group in the CLI, consistent with the existing `manifest` command group pattern.

#### Scenario: vcpe service types prints table
- **WHEN** an operator runs `vcpe service types`
- **THEN** the system prints a table with one row per registered service type showing name, description, and default pull policy

#### Scenario: vcpe service types --json emits structured output
- **WHEN** an operator runs `vcpe service types --json`
- **THEN** the system prints a JSON object `{"types": [...]}` where each element contains `name`, `description`, `defaultPullPolicy`, `defaultImage`, and `expectedRoles` fields, then exits 0

#### Scenario: vcpe service --help exits zero
- **WHEN** an operator runs `vcpe service --help` or `vcpe service types --help`
- **THEN** the system prints structured help text and exits 0

### Requirement: ServiceType interface metadata methods
The `ServiceType` interface SHALL define two additional methods: `Description() string` returning a human-readable one-line description of the service type, and `DefaultImage() string` returning the default OCI image repository (empty string if the type has no canonical default image). All registered service type implementations SHALL implement both methods.

#### Scenario: All registered types implement Description and DefaultImage
- **WHEN** the type registry is initialized
- **THEN** every registered `ServiceType` provides non-empty `Description()` and either a non-empty or empty `DefaultImage()` without panicking

#### Scenario: JSON output includes description and default image
- **WHEN** `vcpe service types --json` is run with the bng type registered
- **THEN** the output includes `"name": "bng"`, `"description": "..."`, and `"defaultImage": "ghcr.io/gdcs-dev/bng"`

### Requirement: vcpe binary discovery in VS Code extension
The VS Code extension SHALL locate the `vcpe` binary by first checking the `vcpe.binaryPath` VS Code configuration setting, then falling back to `PATH` lookup. The extension SHALL cache the `vcpe service types --json` result in extension memory after the first successful invocation. The cache SHALL be invalidated on extension reload. If the binary cannot be found via either method, the extension SHALL surface an actionable error in the type palette referencing the `vcpe.binaryPath` setting.

#### Scenario: Binary found via configuration setting
- **WHEN** `vcpe.binaryPath` is set in VS Code settings to a valid `vcpe` binary path
- **THEN** the extension uses that path to invoke `vcpe service types --json`

#### Scenario: Binary found via PATH fallback
- **WHEN** `vcpe.binaryPath` is not set and `vcpe` is on the system PATH
- **THEN** the extension successfully invokes `vcpe service types --json` using the PATH-resolved binary

#### Scenario: Types cached after first fetch
- **WHEN** `vcpe service types --json` succeeds on first call
- **THEN** subsequent calls within the same session use the cached result without re-invoking the binary
