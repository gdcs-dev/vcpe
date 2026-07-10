## Purpose
Discover deployment manifest files from a search path so users do not need to provide an explicit file path. Powers both manifest auto-selection and `vcpe manifest list`.

## Requirements

### Requirement: Manifest search-path discovery
The system SHALL discover manifest files from an ordered list of directories without requiring the user to provide an explicit path. The search order SHALL be:
1. `VCPE_MANIFEST_DIRS` environment variable (colon-separated; tilde-expanded; empty entries skipped)
2. `<os.Executable()>/../share/vcpe/manifests/` (Homebrew pkgshare; symlinks NOT resolved)
3. `~/.vcpe/manifests/`
4. `./manifests/` (current working directory)

Only files with `apiVersion: vcpe.dev/v1` and `kind: Deployment` SHALL be considered valid manifests. Invalid or unparseable files SHALL be silently skipped.

#### Scenario: Single manifest discovered
- **WHEN** exactly one valid manifest exists across all search directories
- **THEN** `FindAll` returns a slice of one `Entry` with `Name`, `Path`, and `Description` populated

#### Scenario: Multiple manifests discovered
- **WHEN** valid manifests exist in more than one search directory
- **THEN** `FindAll` returns all entries in discovery order (earlier directories first)

#### Scenario: No valid manifests
- **WHEN** no valid manifests exist in any search directory
- **THEN** `FindAll` returns an empty slice and nil error

#### Scenario: VCPE_MANIFEST_DIRS override
- **WHEN** `VCPE_MANIFEST_DIRS=/custom/path:/other/path` is set
- **THEN** those directories are searched before pkgshare and `~/.vcpe/manifests/`

#### Scenario: Non-existent directories silently skipped
- **WHEN** a search directory does not exist
- **THEN** it is skipped without error or warning

---

### Requirement: Bare-name manifest resolution
The system SHALL resolve a bare manifest name (no `/` or `.yaml` suffix) to a file path by searching discovery directories for `<name>.yaml`.

#### Scenario: Name found
- **WHEN** `Resolve("single-gateway", dirs)` is called and `single-gateway.yaml` exists in one of `dirs`
- **THEN** the absolute path to the first matching file is returned

#### Scenario: Name not found
- **WHEN** no `<name>.yaml` exists in any search directory
- **THEN** an error "no manifest named `<name>` found in search path" is returned

---

### Requirement: `vcpe manifest list` command
The system SHALL provide a `vcpe manifest list` subcommand that displays all discovered manifests.

Default output is a table with columns: `NAME`, `PATH`, `DESCRIPTION`. `--json` outputs a JSON array of `{name, path, description}` objects. Empty result for `--json` is `[]`. Empty result for table is the message "no manifests found in search path" (exit 0).

#### Scenario: List with manifests present
- **WHEN** `vcpe manifest list` is run with manifests in the search path
- **THEN** a table row is printed for each discovered manifest with name, path, and description

#### Scenario: List with --json flag
- **WHEN** `vcpe manifest list --json` is run
- **THEN** a JSON array is printed to stdout; exit code 0

#### Scenario: List with no manifests
- **WHEN** `vcpe manifest list` is run and no manifests exist in any search directory
- **THEN** "no manifests found in search path" is printed to stdout; exit code 0
