## MODIFIED Requirements

### Requirement: Declarative local control-plane commands
The system SHALL provide a local CLI contract with `plan`, `apply`, `status`, and `destroy` commands for deployments, and SHALL expose `init`, `build`, `up`, `down`, `logs`, `config`, `state`, and `release` commands as Go-owned operator commands rather than bash-owned behavior. Every command SHALL support `-h`/`--help` to display structured help text and exit 0. The `down` command SHALL remove the Podman networks created for the deployment after stopping all compose services. Network removal failures SHALL be treated as warnings and SHALL NOT prevent state cleanup. The `manifest` command group SHALL expose `list` and `build` subcommands; `build` SHALL run the interactive manifest builder wizard. The `release` command SHALL require an explicit `--version <vX.Y.Z>` flag, stamp first-party image tags in the manifest, commit and tag the manifest change in git, push the commit and tag to origin, then build and push container images. The `build`, `push`, and `release` commands SHALL be conditionally compiled: under the `homebrew` Go build tag they SHALL be absent from the binary; under the default (no build tag) build they SHALL be fully available.

#### Scenario: Developer commands absent from Homebrew binary
- **WHEN** the vcpe binary is built with `-tags homebrew`
- **THEN** `vcpe build`, `vcpe push`, and `vcpe release` return "unknown command" because they are not compiled into the binary

#### Scenario: Developer commands present in non-Homebrew binary
- **WHEN** the vcpe binary is built without any build tags (default)
- **THEN** `vcpe build`, `vcpe push`, and `vcpe release` are fully functional
