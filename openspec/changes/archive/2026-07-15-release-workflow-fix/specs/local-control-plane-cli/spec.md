## MODIFIED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, `config`, `state`, and `release` commands as Go-owned operator commands rather than bash-owned behavior. Every command SHALL support `-h`/`--help` to display structured help text and exit 0. The `down` command SHALL remove the Podman networks created for the deployment after stopping all compose services. Network removal failures SHALL be treated as warnings and SHALL NOT prevent state cleanup. The `manifest` command group SHALL expose `list` and `build` subcommands; `build` SHALL run the interactive manifest builder wizard. The `release` command SHALL require an explicit `--version <vX.Y.Z>` flag, stamp first-party image tags in the manifest, commit and tag the manifest change in git, push the commit and tag to origin, then build and push container images.

#### Scenario: Plan reports intended changes
- **WHEN** an operator runs `plan` for a valid deployment manifest
- **THEN** the system outputs a deterministic diff of desired versus actual state without mutating runtime resources

#### Scenario: Go operator owns public command behavior
- **WHEN** an operator runs `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `state`, or `release`
- **THEN** the command is handled by the Go operator command surface and uses control-plane validation, state, and output contracts

#### Scenario: Help flag exits zero on any command
- **WHEN** an operator runs `vcpe <command> --help` or `vcpe --help`
- **THEN** the system prints structured help text and exits with code 0 without executing the command

#### Scenario: down removes networks
- **WHEN** an operator runs `vcpe down --name <deployment>`
- **THEN** after stopping all compose services the system removes the Podman networks that were created for the deployment

#### Scenario: down completes even if network removal fails
- **WHEN** an operator runs `vcpe down --name <deployment>` and a network cannot be removed
- **THEN** the system logs a warning for the failed network but continues, clears IPAM leases, and removes the deployment snapshot

#### Scenario: manifest build launches wizard
- **WHEN** an operator runs `vcpe manifest build`
- **THEN** the system launches the interactive manifest builder wizard

#### Scenario: release requires --version
- **WHEN** an operator runs `vcpe release` without `--version`
- **THEN** the command fails immediately with a clear error before any side effects

#### Scenario: release requires main branch
- **WHEN** an operator runs `vcpe release` from a branch other than `main`
- **THEN** the command fails immediately with a clear error identifying the current branch, before any side effects
