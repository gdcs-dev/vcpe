## REMOVED Requirements

### Requirement: Primary Go operator binary
**Reason**: The `vcpectl` alias/debug binary was a compatibility-window artefact. The compatibility window has closed; `vcpe` is the sole operator command. `controlplane/cmd/vcpectl/` is deleted.
**Migration**: Use `vcpe` for all operations. Any scripts or aliases pointing to `vcpectl` should be updated to `vcpe`.

#### Scenario: User invokes primary operator
- **WHEN** a user runs `vcpe status`
- **THEN** the command executes the Go operator implementation

### Requirement: Script compatibility shims
**Reason**: The one-release compatibility window for `./scripts/vcpe` and per-service script shims has closed. Top-level script shims are removed; the platform `scripts/lib/common.sh` no longer references `vcpectl`. All operator paths go through the `vcpe` binary.
**Migration**: Use `vcpe <command>` directly or the Makefile convenience targets (`make status NAME=<name>`, `make down NAME=<name>`, etc.).

#### Scenario: Script shim delegates to Go
- **WHEN** a user runs a documented `./scripts/vcpe` or service script command
- **THEN** the script invokes the Go operator command, propagates its exit code, and does not source profiles or mutate runtime resources directly
