## MODIFIED Requirements

### Requirement: Top-level README must provide an end-to-end developer workflow
The project SHALL provide a top-level README that documents project overview, prerequisites, build steps, run steps, and test/verification steps with executable command examples for the primary Go `vcpe` command driven by a `vcpe.dev/v1` `Deployment` manifest.

#### Scenario: New developer follows quick start
- **WHEN** a developer reads the top-level README quick start section
- **THEN** the developer can author a `vcpe.dev/v1` manifest, apply a representative deployment, and view its status by `--name` using documented commands only

#### Scenario: Developer sees Go primary command path
- **WHEN** a developer reads build and run examples
- **THEN** the examples identify `vcpe` as the primary command and use manifest-driven deployment selected by `--name` rather than profile or `--customer` selection
