## ADDED Requirements

### Requirement: Top-level README must provide an end-to-end developer workflow
The project SHALL provide a top-level README that documents project overview, prerequisites, build steps, run steps, and test/verification steps with executable command examples.

#### Scenario: New developer follows quick start
- WHEN a developer reads the top-level README quick start section
- THEN the developer can initialize configuration, start a representative deployment, and view status using documented commands only

### Requirement: README must document both supported local run modes
The project SHALL document both legacy script orchestration and control-plane compatibility mode, including how to switch modes.

#### Scenario: Developer switches run mode
- WHEN a developer needs to run in control-plane compatibility mode
- THEN README includes mode toggle guidance and command examples that use the same script entrypoints

### Requirement: Optional Makefile wrappers must mirror script workflows
If a top-level Makefile is provided, it MUST wrap existing scripts and SHALL NOT replace script semantics or embed divergent orchestration behavior.

#### Scenario: Make target and script parity
- WHEN a developer runs a Make target for a common workflow step
- THEN the resulting behavior maps directly to the corresponding script command path

### Requirement: README examples must reference stable deeper documentation
The top-level README SHALL link to runbook or detailed docs for advanced operational procedures.

#### Scenario: Advanced operator flow lookup
- WHEN a developer needs deeper operational guidance than quick start
- THEN the README provides direct links to detailed documentation sections
