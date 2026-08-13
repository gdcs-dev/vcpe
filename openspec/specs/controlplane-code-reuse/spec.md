# controlplane-code-reuse Specification

## Purpose
TBD - created by archiving change consolidate-controlplane-reuse. Update Purpose after archive.
## Requirements
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

### Requirement: Explicit replica rendering strategies
The shared renderer lifecycle SHALL provide an explicit per-instance strategy for services that consume resolved `plan.Instance` values and an explicit interpolated strategy for services that generate all replica definitions together. A renderer MUST select its strategy declaratively and the shared lifecycle MUST NOT infer the strategy from service names or artifact content.

#### Scenario: Literal renderer consumes resolved instances
- **WHEN** a curated service uses the per-instance strategy
- **THEN** each hook invocation receives exactly one resolved instance with its planned interface identities and addresses

#### Scenario: Generic container preserves interpolation
- **WHEN** a generic-container service renders multiple replicas
- **THEN** it retains its existing interpolated Compose and environment behavior while delegating only common lifecycle mechanics

#### Scenario: Unsupported strategy fails
- **WHEN** a renderer declares a replica strategy not supported by the shared lifecycle
- **THEN** rendering fails deterministically before any artifact is returned

### Requirement: Stable renderer artifact contract
Renderer consolidation MUST preserve renderer names, artifact keys, first-instance canonical artifacts, per-instance directories, auxiliary artifact mirroring, and semantically equivalent Compose output. YAML map ordering MAY differ, but parsed Compose services, networks, values, and references MUST remain equivalent.

#### Scenario: Canonical environment artifact is preserved
- **WHEN** a service with at least one resolved instance is rendered
- **THEN** the result contains root `compose.env` content corresponding to instance one and `instances/<n>/compose.env` for every resolved instance using one-based directory names

#### Scenario: Auxiliary artifacts retain mirror behavior
- **WHEN** a service hook emits an auxiliary artifact for each instance
- **THEN** every instance receives `instances/<n>/<key>` and instance one is additionally mirrored at root `<key>`

#### Scenario: Compose fragments aggregate
- **WHEN** per-instance hooks emit valid Compose fragments with distinct service and network keys
- **THEN** the shared lifecycle emits one `compose.yaml` containing all services and networks

#### Scenario: Invalid Compose fragment fails
- **WHEN** a per-instance hook emits missing or malformed `compose.yaml`
- **THEN** aggregation fails with renderer and instance context and returns no partial result

#### Scenario: Conflicting Compose key fails
- **WHEN** two instance fragments define the same service or network key with different values
- **THEN** aggregation fails explicitly rather than silently replacing one definition

#### Scenario: Equivalent shared network key is accepted
- **WHEN** multiple instance fragments define the same external network key with semantically identical values
- **THEN** aggregation retains one equivalent network definition without failing

### Requirement: Shared service-type defaults
The service type registry SHALL provide reusable defaults for curated health behavior on container port `9878`, build image policy, and no-op interface validation. Built-in service types SHALL delegate to these defaults unless they explicitly declare different behavior. Default delegation MUST preserve the complete `ServiceType` interface and compile-time registration model.

#### Scenario: Curated type inherits defaults
- **WHEN** a built-in curated service type does not override shared metadata
- **THEN** registry consumers observe curated health on port `9878`, build policy, and successful no-op interface validation

#### Scenario: Optional health type overrides default
- **WHEN** generic-container declares optional health behavior
- **THEN** its explicit optional health declaration overrides the shared curated default

#### Scenario: Registry completeness remains enforced
- **WHEN** built-in types are enumerated in tests
- **THEN** every type exposes valid health metadata, image policy, interface validation, description, default image metadata, expected roles, config validation, and a renderer

### Requirement: Registry-authoritative service defaults
All control-plane user interfaces and command paths that need service-type default metadata SHALL resolve it through the service type registry and MUST NOT maintain an independent type-to-default mapping. Unknown types and types with no default image SHALL produce an empty image default without panic.

#### Scenario: Wizard uses registered image default
- **WHEN** an operator selects any registered service type in the manifest wizard without entering an existing repository
- **THEN** the image repository prompt defaults to that type's `DefaultImage()` value

#### Scenario: Registered type with no image default
- **WHEN** an operator selects a registered type whose `DefaultImage()` is empty
- **THEN** the wizard presents an empty repository default and permits explicit operator input

#### Scenario: Unknown type is handled safely
- **WHEN** a metadata consumer requests defaults for an unregistered type
- **THEN** it receives no default rather than panicking or consulting a parallel hardcoded catalog

### Requirement: Deterministic environment reuse
The rendering package SHALL provide one deterministic conversion from environment maps to `KEY=value` entries and one conventional helper for root and per-instance environment artifacts. Environment map keys MUST be sorted lexicographically. Service renderers MUST append computed environment entries deliberately and MUST NOT rely on Go map iteration order.

#### Scenario: Environment map output is stable
- **WHEN** equivalent environment maps are rendered repeatedly
- **THEN** the generated `KEY=value` entries are byte-for-byte identical and sorted by key

#### Scenario: Instance environment paths are stable
- **WHEN** the conventional environment artifact helper processes resolved instances
- **THEN** it emits the existing root and one-based per-instance artifact paths without changing content ordering

### Requirement: Canonical image reference formatting
The control plane SHALL use one canonical image-reference formatter across rendered environment, Compose definitions, and image lifecycle operations. For a non-empty repository, an omitted or whitespace-only tag SHALL resolve to `latest`. For an empty or whitespace-only repository, the formatter SHALL return an empty reference and MUST NOT return `:latest`.

#### Scenario: Missing tag defaults to latest
- **WHEN** an image has repository `example.test/workload` and no tag
- **THEN** the canonical reference is `example.test/workload:latest`

#### Scenario: Explicit tag is preserved
- **WHEN** an image has repository `example.test/workload` and tag `v2`
- **THEN** the canonical reference is `example.test/workload:v2`

#### Scenario: Empty repository stays empty
- **WHEN** an image has no repository
- **THEN** the canonical reference is empty in rendering and image lifecycle code

### Requirement: Direct image backend conformance
Docker and Podman image adapters SHALL implement the `image.Backend` interface directly using the request types owned by the image lifecycle package. The application SHALL select and return those concrete adapters without forwarding-only request translation wrappers. Backend-specific command construction, multi-platform behavior, standard stream handling, and error diagnostics SHALL remain owned by each adapter.

#### Scenario: Podman adapter satisfies image backend
- **WHEN** the Podman adapter is compiled against `image.Backend`
- **THEN** it accepts the canonical build, pull, push, and tag request types without application-layer translation

#### Scenario: Docker adapter satisfies image backend
- **WHEN** the Docker adapter is compiled against `image.Backend`
- **THEN** it accepts the canonical build, pull, push, and tag request types without application-layer translation

#### Scenario: Backend command semantics remain distinct
- **WHEN** equivalent multi-platform build requests are sent to Docker and Podman adapters
- **THEN** each adapter retains its runtime-specific CLI arguments and validation behavior

#### Scenario: Skip-image backend remains available
- **WHEN** image operations are disabled through the existing test environment contract
- **THEN** the application returns the no-op image backend with unchanged behavior

### Requirement: Conservative network helper ownership
Exact network helper duplication SHALL be consolidated only when the helper has one coherent domain owner. Address-allocation policy SHALL remain in IPAM or planning, renderer-only address formatting SHALL remain in rendering, and unused duplicate implementations SHALL be removed. The control plane MUST NOT introduce a miscellaneous network utility package containing unrelated policy.

#### Scenario: Address formatting is shared without moving policy
- **WHEN** multiple service renderers append a CIDR prefix to an assigned IP
- **THEN** they call one renderer-oriented formatter with unchanged invalid-input behavior

#### Scenario: Unused helper is removed
- **WHEN** a duplicate network helper has no production callers
- **THEN** it is deleted rather than exported solely to appear reusable

#### Scenario: Policy-specific helpers remain domain-owned
- **WHEN** two helpers use similar network parsing but make different allocation or reconciliation decisions
- **THEN** they remain separate in their owning domains

### Requirement: Compatibility-gated migration
Every consolidation slice SHALL be preceded or accompanied by tests that characterize the affected behavior. Migration SHALL proceed incrementally, and focused tests for the touched shared package and migrated consumer MUST pass before another consumer is migrated. Final verification SHALL cover all unit tests, build targets, and available non-destructive smoke tests.

#### Scenario: Renderer migration is behavior-checked
- **WHEN** a built-in renderer is migrated to shared infrastructure
- **THEN** tests compare artifact inventories, environment content, parsed Compose content, replica names, health ports, and type-specific exceptions before the next renderer is migrated

#### Scenario: XB10 migration has focused coverage
- **WHEN** XB10 is migrated to shared renderer mechanics
- **THEN** package-local tests first capture its artifact layout, ports, network attachments, and addressing exemption

#### Scenario: Shared infrastructure failure blocks further migration
- **WHEN** focused tests fail after a consumer adopts shared infrastructure
- **THEN** that consumer is repaired or reverted before another consumer is migrated

#### Scenario: Final repository verification
- **WHEN** all consolidation tasks are complete
- **THEN** the control plane builds and all environment-independent Go tests pass, with any environment-gated failures documented separately

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
- **THEN** it sets that field through the single mutator-hook pattern rather than a new dedicated hook parameter

