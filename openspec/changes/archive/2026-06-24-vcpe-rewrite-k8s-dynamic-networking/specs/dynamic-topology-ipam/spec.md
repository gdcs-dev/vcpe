## ADDED Requirements

### Requirement: Dynamic network planning
The system SHALL compute Podman network mappings from logical interface roles (`mgmt`, `wan`, `cm`, `lan-p*`) without hardcoded customer ID tables.

#### Scenario: New customer onboarding without code changes
- **WHEN** an operator applies a new customer deployment with valid network intents
- **THEN** the planner generates concrete attachments and names without requiring source edits for the customer ID

### Requirement: Deterministic IP allocation and conflict prevention
The system MUST allocate IPv4/IPv6 addresses from configured pools and reject plans that would create overlapping allocations.

#### Scenario: Overlapping allocation is prevented
- **WHEN** requested network allocation overlaps an existing active lease
- **THEN** planning fails with a conflict error and no new lease is committed
