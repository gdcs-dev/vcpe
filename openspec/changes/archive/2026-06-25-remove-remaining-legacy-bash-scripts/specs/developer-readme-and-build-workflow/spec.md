## MODIFIED Requirements

### Requirement: Top-level README must provide an end-to-end developer workflow
The project SHALL provide a top-level README that documents project overview, prerequisites, build steps, run steps, and test/verification steps with executable command examples for the primary Go `vcpe` command and `vcpe service <name> ...` workflows.

#### Scenario: New developer follows quick start
- **WHEN** a developer reads the top-level README quick start section
- **THEN** the developer can initialize configuration, start a representative deployment, and view status using documented commands only

#### Scenario: Developer sees canonical command paths
- **WHEN** a developer reads build and run examples after wrapper removal
- **THEN** the examples identify `vcpe` and `vcpe service <name> ...` as canonical command paths and do not require legacy script wrappers

### Requirement: README must document both supported local run modes
The project SHALL document the supported local Podman run model and host prerequisites for Linux and macOS Podman-machine delegation.

#### Scenario: Developer configures supported host mode
- **WHEN** a developer prepares a host for local execution
- **THEN** README includes host-model guidance and command examples for supported Linux and macOS delegation paths

### Requirement: Optional Makefile wrappers must mirror script workflows
If a top-level Makefile is provided, it MUST wrap canonical `vcpe` command workflows and SHALL NOT embed divergent orchestration behavior.

#### Scenario: Make target and command parity
- **WHEN** a developer runs a Make target for a common workflow step
- **THEN** the resulting behavior maps directly to the corresponding `vcpe` command path