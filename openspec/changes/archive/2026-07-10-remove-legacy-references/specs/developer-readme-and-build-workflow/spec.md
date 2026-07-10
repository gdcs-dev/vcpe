## REMOVED Requirements

### Requirement: README must document both supported local run modes
**Reason**: Legacy bash script orchestration is no longer a supported run mode. The Go `vcpe` control plane is the only supported operator path. Documenting a removed mode as "supported" is misleading.
**Migration**: The README documents only the `vcpe`-driven manifest workflow. Developers who need legacy bash script context can refer to the archived change history in `openspec/changes/archive/`.

#### Scenario: Developer switches run mode
- **WHEN** a developer needs to run in control-plane compatibility mode
- **THEN** README includes mode toggle guidance and command examples that use the same script entrypoints

## MODIFIED Requirements

### Requirement: Optional Makefile wrappers must mirror script workflows
The top-level Makefile SHALL provide optional convenience wrappers over `vcpe` commands. It SHALL NOT embed divergent orchestration behavior. Targets that accept a deployment name SHALL use a `NAME` variable (defaulting to the quick-start example name) mapped to the `--name` flag.

#### Scenario: Make target invokes vcpe with correct flags
- **WHEN** a developer runs `make status NAME=bng-7`
- **THEN** the Makefile executes `vcpe status --name bng-7` and returns its exit code

#### Scenario: Make target uses default name when NAME is unset
- **WHEN** a developer runs `make status` without setting NAME
- **THEN** the Makefile executes `vcpe status --name bng-7` using the built-in default
