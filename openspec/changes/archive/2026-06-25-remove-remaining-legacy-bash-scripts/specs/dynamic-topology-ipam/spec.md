## MODIFIED Requirements

### Requirement: Dynamic network planning
The system SHALL compute Podman network mappings from logical interface roles (`mgmt`, `wan`, `cm`, `lan-p*`) without hardcoded customer ID tables. Service catalog and host-network planning MUST consume logical topology inputs and persisted allocation state, and MUST NOT reintroduce fixed customer ID network tables as the source of truth.

#### Scenario: New customer onboarding without code changes
- **WHEN** an operator applies a new customer deployment with valid network intents
- **THEN** the planner generates concrete attachments and names without requiring source edits for the customer ID

#### Scenario: Service catalog preserves dynamic topology
- **WHEN** a service catalog entry requires network attachments for a customer deployment
- **THEN** the planner resolves attachments from logical roles and allocation state rather than hardcoded customer ID lists

### Requirement: Host-network intent participates in topology planning
The system SHALL represent bridge, NAT, firewall, and attachment needs as host-network intents that can be preflighted and planned before mutation, and SHALL emit host-network intents consumable by the Go Host Network Controller.

#### Scenario: Host-network intent is visible in plan
- **WHEN** a deployment requires host bridge or NAT changes
- **THEN** the plan includes host-network intent entries before apply performs privileged host operations

## ADDED Requirements

### Requirement: Deterministic interface identity contracts
The system SHALL derive deterministic per-service interface identity contracts from typed manifest, catalog, and allocation state inputs and SHALL persist derived MAC/interface role assignments as deployment state.

#### Scenario: Interface role mapping remains stable across apply
- **WHEN** the same deployment is planned and applied repeatedly without topology changes
- **THEN** derived interface role and MAC assignments remain unchanged for each service instance

#### Scenario: Allocation artifacts support recovery
- **WHEN** a deployment operation requires rollback or status inspection
- **THEN** persisted MAC/interface allocation artifacts are available to reconstruct intended runtime interface identity