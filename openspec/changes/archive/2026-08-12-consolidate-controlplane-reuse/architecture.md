## Overview

This change replaces repeated control-plane mechanics with small owning abstractions while preserving policy at its current domain boundary. The design follows the existing manifest-to-plan-to-render-to-reconcile flow: shared packages own invariant mechanics, service packages own workload semantics, and runtime adapters own backend-specific command construction.

The change is compatibility-first. It does not redesign manifests, plans, state, Compose projects, or service behavior. Each extraction must demonstrate equivalent outputs and errors before a duplicated implementation is removed.

## Components

- **`render/servicetemplate`**: Implements typed config decoding, no-instance validation, replica traversal, canonical artifact placement, and aggregation of Compose service/network fragments. It exposes an explicit per-instance mode and an explicit all-replicas/interpolated mode.
- **Service renderer hooks**: Remain in each `internal/types/<type>` package. They own service-specific environment values, Compose service fields, network attachment exceptions, health topology, and auxiliary files.
- **`typeregistry.BaseServiceType`**: Supplies zero-value defaults for curated health metadata, build image policy, and no-op interface validation. Concrete types override only behavior that differs.
- **Registry-backed wizard defaults**: Resolves the selected type through `typeregistry.Lookup` and reads `DefaultImage`, eliminating the wizard's second service catalog.
- **Canonical render helpers**: Produce sorted `KEY=value` entries, conventional root/per-instance `compose.env` artifacts, and a canonical image reference.
- **Image backend contract**: Defines image operation requests once in `internal/image`. Docker and Podman adapters implement `image.Backend` directly while retaining backend-specific command arguments and diagnostics.
- **Focused network helpers**: Live with the domain that owns their policy. IPAM owns network-address selection; rendering owns address formatting needed by multiple renderers. Dead duplicate functions are deleted.
- **Compatibility test matrix**: Exercises shared invariants across every registered built-in type and leaves service-specific content assertions in each service package.

## Key Architectural Decisions

### Consolidate mechanics, not service policy
**Choice**: Shared infrastructure owns lifecycle control flow and deterministic formatting. Service packages continue to own all workload-specific values and exceptions.
**Rationale**: The duplicated lifecycle is a stable invariant, while service privilege, network pinning, health topology, volumes, commands, and generated files vary materially.
**Alternatives considered**: A universal Compose builder with a large options structure was rejected because it would conceal runtime policy and grow with each exceptional service.

### Use typed renderer hooks
**Choice**: The shared renderer uses generic typed hooks for config decoding and rendering instead of untyped maps or reflection.
**Rationale**: Typed hooks preserve compile-time service config boundaries and the ratified requirement against freeform substitution.
**Alternatives considered**: Reflection and map-based callbacks were rejected because they weaken validation and error locality. Repeating each full renderer was rejected because it retains the current drift risk.

### Model replica strategies explicitly
**Choice**: The shared renderer distinguishes per-instance literal rendering from all-replicas interpolated rendering.
**Rationale**: Curated services consume resolved `plan.Instance` values, while generic-container intentionally generates interpolated Compose definitions. Naming the difference prevents either model from being forced into the other.
**Alternatives considered**: Excluding generic-container was rejected because artifact and replica mechanics would remain duplicated. Converting it to literal rendering was rejected as an unrelated behavior change.

### Preserve artifact compatibility
**Choice**: Keep `compose.yaml`, root `compose.env`, `instances/<n>/compose.env`, first-instance mirrored auxiliary artifacts, and existing renderer names unchanged.
**Rationale**: The orchestrator and persisted generated state consume these conventions. Structural cleanup must not trigger deployment churn or break inspection workflows.
**Alternatives considered**: A new artifact hierarchy was rejected because it would require state and orchestration changes outside this proposal.

### Keep one authoritative service catalog
**Choice**: Registry metadata supplies wizard image defaults and other service-type metadata.
**Rationale**: The registry already defines support and default images; a parallel switch has already omitted registered types.
**Alternatives considered**: Expanding the wizard switch was rejected because it preserves duplicate ownership and future drift.

### Let concrete image adapters implement the lifecycle interface
**Choice**: Docker and Podman adapter methods accept `internal/image` request types directly and satisfy `image.Backend` without forwarding wrappers.
**Rationale**: The request structures are identical and the current wrappers contain no policy. Backend-specific argument construction remains independent.
**Alternatives considered**: A second neutral request package was rejected as unnecessary layering. Sharing Docker/Podman CLI argument builders was rejected because their semantics differ.

### Use a leaf owner for image references
**Choice**: Move image reference formatting to a dependency-neutral leaf API used by both rendering and image lifecycle code. Empty repository returns an empty reference; an absent tag defaults to `latest` only for a non-empty repository.
**Rationale**: Current helpers disagree on empty repositories. A leaf owner avoids importing lifecycle policy into rendering or creating an import cycle.
**Alternatives considered**: Choosing either current package as the universal owner was rejected if it reverses dependency direction. Leaving both helpers was rejected because disagreement is already observable.

### Extract only proven network utility duplication
**Choice**: Move byte-equivalent, multi-package formatting helpers to a narrow owner; move policy helpers to IPAM; delete unused duplicate implementations.
**Rationale**: Network utilities often look similar while encoding different lifecycle assumptions. Conservative extraction keeps ownership visible.
**Alternatives considered**: A broad `netutil` package was rejected because it would become a catch-all without a coherent domain contract.

### Migrate incrementally behind compatibility tests
**Choice**: Add shared infrastructure and invariant tests first, then migrate one service or adapter at a time and run focused tests after every migration.
**Rationale**: Renderer changes have a high blast radius and some services lack package-local coverage. Incremental migration makes regressions attributable and reversible.
**Alternatives considered**: A single mechanical rewrite was rejected because failures would not identify which service exception was lost.

## Data Flow

    manifest.Document
           |
           v
      app.Preflight ---------> typeregistry.ServiceType
           |                           |
           v                           | typed config/default metadata
        planner                        v
           |                    service renderer hooks
           v                           |
      plan.Deployment -----------------+
           |
           v
    render/servicetemplate
       |           |
       |           +--> service-owned env/config/Compose fragments
       |
       +--> canonical artifact layout + merged compose.yaml
                           |
                           v
                   app orchestrator/state
                           |
                           v
                    compose + Podman

Image operations follow a separate policy/backend split:

    manifest.Image
          |
          v
     image.Manager ---- typed image requests ----> image.Backend
                                                   /          \
                                                  v            v
                                             Podman CLI     Docker CLI

## Integration Points

- `typeregistry.ServiceType` remains the dispatch and metadata interface used by preflight, planning, image policy, help output, and the wizard.
- `render.Renderer`, `render.Input`, `render.Result`, and `render.Artifact` remain the orchestration boundary.
- `plan.Deployment`, `plan.Service`, and `plan.Instance` remain immutable resolved inputs to rendering.
- `image.Manager` remains the owner of build/pull/push/tag policy; adapters perform commands only.
- Persisted state continues storing generated artifacts under existing versioned paths.
- Compose and Podman adapter command behavior remains backend-specific and unchanged except for request type ownership.
- No new external APIs, event streams, databases, or runtime daemons are introduced.

## Security Model

- Secret resolution remains apply-time only; shared renderer infrastructure receives the same already-resolved typed input and must not add logging or persistence of secret payloads.
- Generated environment and configuration artifacts retain current state-root permissions and redaction behavior.
- The consolidation introduces no new process execution. Docker, Podman, Compose, and host networking remain the only command boundaries.
- Typed config decoding and fail-before-mutation validation remain mandatory; shared hooks must not bypass preflight or accept unbounded text substitution.
- Backend errors may include command diagnostics but must continue using existing redaction before operator output or durable recording.

## Error Handling Strategy

- Shared renderer construction rejects missing names, decoders, hooks, or unsupported modes with deterministic errors.
- Rendering rejects services with no resolved instances before producing artifacts.
- Hook errors retain service type, service name, and one-based instance context when propagated.
- Invalid or missing per-instance `compose.yaml` fragments fail aggregation; partial results are not returned.
- Duplicate Compose service or network keys fail explicitly unless their definitions are semantically identical. Silent last-writer-wins merging is prohibited.
- YAML marshal/unmarshal failures are wrapped with renderer and instance context.
- Image adapter validation and command errors preserve current backend-specific wording and exit details.
- No retries are added by this refactor. Existing reconciliation, network, and rollback policies remain authoritative.

## Observability Strategy

- Preserve existing renderer names, operation journal phase names, and backend error identities so logs remain comparable.
- Do not add high-volume per-artifact logging or log rendered contents.
- Tests serve as the primary observability mechanism for this internal refactor: artifact inventories, normalized Compose structures, registry metadata, and adapter arguments are compared before and after migration.
- Existing apply summaries and operation journals remain unchanged; no new metrics are required.

## Constraints

- No manifest, CLI, daemon protocol, state schema, or generated artifact path changes.
- No semantic changes to interface addressing, IP pinning, health exposure, replica identity, image policy, startup ordering, or rollback.
- The canonical `IFACE_<ROLE>_*` environment contract remains the sole interface identity contract.
- XB10 addressing behavior remains exempt exactly as specified by `interface-addressing-mode`.
- Generic-container keeps its interpolated replica behavior and entrypoint artifact contract.
- BNG keeps all auxiliary configuration artifacts and first-instance mirror behavior.
- Service-specific Compose construction remains readable in the service package.
- No reflection, regex rendering, global mutable templates, or broad miscellaneous utility package.
- No new third-party dependency.
- Existing dirty or unrelated workspace changes are outside scope.

## Diagrams

Responsibility boundary after consolidation:

    +-------------------------- shared mechanics --------------------------+
    | config lifecycle | replica traversal | artifact paths | YAML merge  |
    +-------------------------------+--------------------------------------+
                                    |
              +---------------------+---------------------+
              |                     |                     |
              v                     v                     v
          BNG policy           Gateway policy      Generic policy
       DHCP/RADVD files       health sidecar       interpolation/probes
              |                     |                     |
              +---------------------+---------------------+
                                    |
                                    v
                         stable render.Result contract

Migration sequence:

    characterize outputs
            |
            v
    add shared implementation + focused tests
            |
            v
    migrate simplest service ---> validate ---> migrate next
            |
            v
    migrate complex/exempt services last
            |
            v
    remove dead copies and run repository gates