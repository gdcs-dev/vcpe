## MODIFIED Requirements

### Requirement: Dynamic network planning
The system SHALL compute Podman network mappings from logical interface roles (`mgmt`, `wan`, `cm`, `lan-p*`) without hardcoded customer ID tables. Service catalog and host-network planning MUST consume logical topology inputs and MUST NOT reintroduce fixed customer ID network tables as the source of truth.

#### Scenario: New customer onboarding without code changes
- **WHEN** an operator applies a new customer deployment with valid network intents
- **THEN** the planner generates concrete attachments and names without requiring source edits for the customer ID

#### Scenario: Service catalog preserves dynamic topology
- **WHEN** a service catalog entry requires network attachments for a customer deployment
- **THEN** the planner resolves attachments from logical roles and allocation state rather than hardcoded customer ID lists

### Requirement: Deterministic IP allocation and conflict prevention
The system MUST allocate IPv4/IPv6 addresses from configured pools and reject plans that would create overlapping allocations.

#### Scenario: Overlapping allocation is prevented
- **WHEN** requested network allocation overlaps an existing active lease
- **THEN** planning fails with a conflict error and no new lease is committed

## ADDED Requirements

### Requirement: Host-network intent participates in topology planning
The system SHALL represent bridge, NAT, firewall, and attachment needs as host-network intents that can be preflighted and planned before mutation.

#### Scenario: Host-network intent is visible in plan
- **WHEN** a deployment requires host bridge or NAT changes
- **THEN** the plan includes host-network intent entries before apply performs privileged host operations
