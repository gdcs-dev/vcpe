## Purpose
Define the top-level developer workflow and documentation contract for building, running, and verifying the project during the Go control-plane migration.

## Requirements

### Requirement: Top-level README must provide an end-to-end developer workflow
The project SHALL provide a top-level README that documents project overview, prerequisites, build steps, run steps, and test/verification steps with executable command examples for the primary Go `vcpe` command driven by a `vcpe.dev/v1` `Deployment` manifest.

#### Scenario: New developer follows quick start
- **WHEN** a developer reads the top-level README quick start section
- **THEN** the developer can author a `vcpe.dev/v1` manifest, apply a representative deployment, and view its status by `--name` using documented commands only

#### Scenario: Developer sees Go primary command path
- **WHEN** a developer reads build and run examples
- **THEN** the examples identify `vcpe` as the primary command and use manifest-driven deployment selected by `--name` rather than profile or `--customer` selection

### Requirement: Optional Makefile wrappers must be convenience helpers over vcpe
If a top-level Makefile is provided, it SHALL provide optional convenience wrappers over `vcpe` commands. It SHALL NOT embed divergent orchestration behavior. Targets that accept a deployment name SHALL use a `NAME` variable (defaulting to the quick-start example deployment name) mapped to the `--name` flag.

#### Scenario: Make target invokes vcpe with correct flags
- **WHEN** a developer runs `make status NAME=bng-7`
- **THEN** the Makefile executes `vcpe status --name bng-7` and returns its exit code

#### Scenario: Make target uses default name when NAME is unset
- **WHEN** a developer runs `make status` without setting NAME
- **THEN** the Makefile executes `vcpe status --name bng-7` using the built-in default

### Requirement: README examples must reference stable deeper documentation
The top-level README SHALL link to runbook or detailed docs for advanced operational procedures.

#### Scenario: Advanced operator flow lookup
- WHEN a developer needs deeper operational guidance than quick start
- THEN the README provides direct links to detailed documentation sections
