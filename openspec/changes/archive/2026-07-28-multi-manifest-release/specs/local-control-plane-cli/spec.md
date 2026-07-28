## ADDED Requirements

### Requirement: stamp command available in developer builds
The system SHALL expose a `stamp` top-level command in developer builds of the `vcpe` CLI (excluded from Homebrew installs via the existing developer-commands stub mechanism). The `stamp` command SHALL support `-h`/`--help` to display structured help text and exit 0.

#### Scenario: vcpe stamp --help exits zero
- **WHEN** an operator runs `vcpe stamp --help`
- **THEN** the system prints the `stamp` command help text (synopsis, required/optional flags, examples) and exits 0

#### Scenario: vcpe stamp unavailable in Homebrew build
- **WHEN** an operator running a Homebrew-installed `vcpe` binary runs `vcpe stamp`
- **THEN** the command exits non-zero with a message indicating the command is not available in distribution builds

#### Scenario: --manifest accepts repeated flags with glob expansion
- **WHEN** an operator runs `vcpe stamp --version v0.3.0 --manifest "manifests/*.yaml" --manifest extra/special.yaml`
- **THEN** the CLI resolves the glob and the explicit path into a combined file list and stamps all of them
