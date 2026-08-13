## MODIFIED Requirements

### Requirement: Shared renderer lifecycle
Built-in service renderers SHALL delegate common renderer lifecycle mechanics to a shared typed implementation. The shared implementation SHALL own required-hook validation, no-instance validation, typed config decoding, replica traversal, standard artifact placement, Compose fragment aggregation, renderer result identity, standard Compose service-block construction, and the two established health-sidecar mechanisms. Service packages SHALL retain ownership of service-specific environment values, network attachment triggers, health-sidecar trigger conditions, generated auxiliary configuration content, and any Compose field not covered by the shared service-block builder or the capped escape-hatch hook.

#### Scenario: Per-instance service uses shared lifecycle
- **WHEN** a built-in service type renders multiple resolved instances
- **THEN** the shared renderer lifecycle invokes its typed service hook once per instance and returns one aggregated service-level result

#### Scenario: Standard Compose fields are shared, divergent policy stays local
- **WHEN** a service type needs the standard `image`/`container_name`/`hostname`/`env_file`/`networks` fields
- **THEN** it obtains them from the shared Compose service-block builder rather than re-implementing them, and supplies only genuinely divergent fields (`privileged`, `cap_add`, `volumes`, `network_mode`, `command`) through the single capped escape-hatch hook

#### Scenario: Missing renderer hook fails clearly
- **WHEN** a shared renderer is constructed without a required name, decoder, or render hook
- **THEN** rendering fails with an error identifying the missing renderer contract before producing artifacts

#### Scenario: Service with no resolved instances fails
- **WHEN** a per-instance renderer receives a service with no resolved instances
- **THEN** rendering fails with an error identifying the renderer and service before producing artifacts

### Requirement: Consolidation boundaries
The change MUST NOT consolidate Docker and Podman CLI argument builders, persistence query scanning, manifest and plan models, runtime-init binary entrypoints, wizard workflows with distinct domain behavior, service-specific auxiliary configuration generation (e.g. DHCP/RADVD/DNS/firewall content, generated entrypoints), or any health mechanism beyond the two already-established shared shapes (proxy sidecar, probe sidecar). A new shared hook or attachment variant SHALL be introduced only when at least two production consumers already share the identical behavioral contract; the shared Compose escape hatch MUST remain a single mutator hook and MUST NOT grow a dedicated flag per Compose field.

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
- **WHEN** a service type needs a health-delivery mechanism that is neither the shared proxy-sidecar nor probe-sidecar shape
- **THEN** that type constructs its own local Compose fragment for it rather than extending the shared health-sidecar builders

#### Scenario: Escape hatch does not grow per-field flags
- **WHEN** a service type needs a Compose field the shared builder does not already produce
- **THEN** it sets that field through the single `ComposeExtras`-style mutator hook rather than a new dedicated hook parameter

## ADDED Requirements

### Requirement: Shared Compose service-block construction
The shared renderer lifecycle SHALL provide one builder that constructs a Compose service's `image`, `container_name`, `hostname`, `env_file`, and `networks` fields from `render.Input`, the resolved `plan.Instance`, and a network-attachment function. Every curated per-instance built-in type SHALL use this builder instead of independently constructing these fields.

#### Scenario: Standard fields match existing naming
- **WHEN** the shared builder constructs a service block for instance `n` of service `svc` in deployment `dep`
- **THEN** it produces `container_name` `<dep>-<svc>-<n>`, `hostname` `<svc>-<n>`, and `env_file` `instances/<n>/compose.env`, matching the values every migrated type already produced by hand

#### Scenario: Network attachment is pluggable
- **WHEN** a type supplies a network-attachment function
- **THEN** the shared builder populates each interface's network entry using that function instead of a hardcoded attachment rule

#### Scenario: Default attachment pins mac and conditionally pins ipv4
- **WHEN** a type does not supply a network-attachment function
- **THEN** the shared builder always pins `mac_address` and pins `ipv4_address` only when the interface's network is Podman-managed (`ipamDriver != "none"`)

#### Scenario: MAC-only attachment never pins ipv4
- **WHEN** a type supplies the shared MAC-only attachment function
- **THEN** the shared builder pins `mac_address` and never pins `ipv4_address`, regardless of network management

### Requirement: Shared health-sidecar mechanisms
The shared renderer lifecycle SHALL provide exactly two reusable health-sidecar builders: a proxy-sidecar builder for workloads reached by service name over the shared external health network, and a probe-sidecar builder for workloads whose sidecar shares the workload's network namespace and runs a caller-supplied probe command. A service type SHALL decide whether and when to invoke a builder using its own trigger condition; the shared lifecycle MUST NOT decide sidecar necessity on a type's behalf.

#### Scenario: Proxy sidecar reproduces existing shape
- **WHEN** a type invokes the shared proxy-sidecar builder for an instance
- **THEN** the produced sidecar service is semantically equivalent to the sidecar previously hand-built by that type, including its `entrypoint`, `--proxy-url`/`--timeout` command, external health network attachment, host port mapping, and `restart: unless-stopped` policy

#### Scenario: Probe sidecar reproduces existing shape
- **WHEN** a type invokes the shared probe-sidecar builder for an instance with a caller-supplied command
- **THEN** the produced sidecar service is semantically equivalent to the sidecar previously hand-built by that type, including sharing the workload's network namespace via `network_mode: service:<instance>` and depending on the workload instance

#### Scenario: Sidecar trigger stays service-owned
- **WHEN** a service type's own trigger condition (e.g. a manifest-declared `healthUpstream` interface, or a configured health probe) evaluates false for an instance
- **THEN** the shared lifecycle does not add a health-sidecar service for that instance
