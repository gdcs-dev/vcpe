## ADDED Requirements

### Requirement: Shared direct health publication
The shared renderer infrastructure SHALL provide one reusable direct-publication helper that attaches an intended workload service to the deployment's external health network and adds its allocated loopback-only health port mapping. The helper MUST mutate only the selected workload service and MUST NOT add a separate transport proxy service.

#### Scenario: Direct helper renders the managed health path
- **WHEN** a renderer invokes the direct-publication helper for an instance
- **THEN** the workload service receives the external `aa-health` attachment and `127.0.0.1:<allocated-port>:9878` mapping without a proxy service

#### Scenario: Zero health port is a no-op
- **WHEN** a renderer invokes the direct-publication helper with no allocated health port
- **THEN** the helper adds no health network, port mapping, dependency, or service

#### Scenario: Namespace-sharing probe inherits workload transport
- **WHEN** a generic-container probe helper shares a directly published workload's network namespace
- **THEN** the probe helper receives no separate `networks` entry or host port mapping

## MODIFIED Requirements

### Requirement: Consolidation boundaries
The change MUST NOT consolidate Docker and Podman CLI argument builders, persistence query scanning, manifest and plan models, runtime-init binary entrypoints, wizard workflows with distinct domain behavior, service-specific auxiliary configuration generation (e.g. DHCP/RADVD/DNS/firewall content, generated entrypoints), or any health mechanism beyond the two established shared responsibilities (direct endpoint publication and namespace-sharing probe execution). A new shared hook or attachment variant SHALL be introduced only when at least two production consumers already share the identical behavioral contract; the shared Compose escape hatch MUST remain a single mutator hook and MUST NOT grow a dedicated flag per Compose field.

#### Scenario: Similar code with different policy remains separate
- **WHEN** two implementations have similar control flow but different error, network, lifecycle, or security semantics
- **THEN** the implementations remain in their current owning packages

#### Scenario: Runtime-init entrypoints remain explicit
- **WHEN** service runtime-init binaries are built
- **THEN** each binary retains its explicit `main` package while delegating shared execution to the existing service command package

#### Scenario: No single-consumer abstraction is introduced
- **WHEN** duplicated-looking code has only one production consumer after dead code removal
- **THEN** it remains local unless it establishes an already-specified ownership boundary

#### Scenario: A third health mechanism stays local
- **WHEN** a service type needs a health-delivery mechanism that is neither direct endpoint publication nor namespace-sharing probe execution
- **THEN** that type constructs its own local Compose fragment rather than extending the shared health helpers

#### Scenario: Escape hatch does not grow per-field flags
- **WHEN** a service type needs a Compose field the shared builder does not already produce
- **THEN** it sets that field through the single mutator-hook pattern rather than a new dedicated hook parameter

## REMOVED Requirements

### Requirement: Shared health-sidecar mechanisms
**Reason**: The proxy-sidecar mechanism duplicates Podman's forwarding after the workload receives a managed health attachment. Direct publication replaces proxy transport, while namespace-sharing probe execution remains covered by existing probe-helper behavior and the new direct-publication contract.

**Migration**: None. The health-check implementation is unreleased; delete the proxy builder and its call sites rather than providing compatibility. Use the direct-publication helper in the replacement implementation, and use the namespace-sharing probe helper only where the new design requires probe execution in the workload namespace.