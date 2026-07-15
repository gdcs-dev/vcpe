## MODIFIED Requirements

### Requirement: Dynamic network planning
The system SHALL compute Podman network mappings from the manifest's declared `networks[]` and interface roles without hardcoded customer ID tables. Network and bridge names SHALL derive from the manifest (explicit `bridge`, or the `<metadata.name>-<role>` default), and planning MUST NOT reintroduce fixed customer ID network tables as the source of truth.

#### Scenario: New deployment onboarding without code changes
- **WHEN** an operator applies a new deployment with valid `networks[]` and interfaces
- **THEN** the planner generates concrete attachments and names from the manifest without requiring source edits

#### Scenario: Planner resolves attachments from declared networks
- **WHEN** a service requires network attachments
- **THEN** the planner resolves attachments from interface roles and declared networks rather than hardcoded customer ID lists

### Requirement: Deterministic IP allocation and conflict prevention
The system MUST treat IPAM as the sole authority for IP address assignment. Explicit interface addresses MUST be validated as within their network CIDR and reserved in IPAM; all other addresses MUST be allocated from the network's configured pool. The system MUST reject plans that would create overlapping allocations, and no other component SHALL assign IP addresses.

#### Scenario: Overlapping allocation is prevented
- **WHEN** a requested or explicit allocation overlaps an existing active lease
- **THEN** planning fails with a conflict error and no new lease is committed

#### Scenario: Explicit address is validated and reserved
- **WHEN** an interface declares an explicit address within its network CIDR
- **THEN** IPAM reserves that address and rejects any conflicting allocation

### Requirement: Host-network intent participates in topology planning
The system SHALL represent bridge, NAT, firewall, and attachment needs as host-network intents derived from explicit `networks[]` fields (bridge name, `nat`, `firewall`) that can be preflighted and planned before mutation.

#### Scenario: Host-network intent is visible in plan
- **WHEN** a deployment requires host bridge or NAT changes
- **THEN** the plan includes host-network intent entries from the declared networks before apply performs privileged host operations

## ADDED Requirements

### Requirement: MAC-only deterministic identity fallback
The system SHALL assign a deterministic MAC address to any interface without an explicit `mac`, derived from the canonical key `metadata.name + "/" + service.name + "/" + interface.role` (with a trailing `/index` for replicas greater than 1). The planner and runtime-init contract generation MUST use the identical key. Deterministic hashing SHALL apply to MACs only and MUST NOT assign IP addresses.

#### Scenario: Deterministic MAC is stable across components
- **WHEN** an interface has no explicit MAC
- **THEN** the planner and runtime-init contract derive the same MAC from the canonical key

#### Scenario: Replica MACs are distinct
- **WHEN** a service has multiple replicas
- **THEN** each replica's interface MAC includes the replica index and is distinct

### Requirement: Interface and bridge name length limit
The system SHALL ensure generated interface and bridge names do not exceed the 15-character kernel limit, truncating derived names with a deterministic hash suffix when `<metadata.name>-<role>` would overflow, and SHALL warn when an explicit name exceeds 15 characters.

#### Scenario: Overflowing derived name is shortened deterministically
- **WHEN** `<metadata.name>-<role>` exceeds 15 characters
- **THEN** the system produces a truncated name with a deterministic hash suffix within the limit

#### Scenario: Explicit overlong name warns
- **WHEN** an explicit bridge name exceeds 15 characters
- **THEN** the system emits a warning identifying the network and name
