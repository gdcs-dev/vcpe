## Overview

This change removes the remaining behavior-owning legacy bash from the vCPE repo without regressing the Go control-plane architecture established by the previous migration. The important distinction is that the remaining problem is not primarily the already-thin top-level `scripts/*` wrappers. The real legacy ownership still lives in platform topology scripts, service container bootstrap logic, and renderer/bootstrap helpers that continue to decide runtime behavior outside typed Go models.

Architecturally, the correct cut is:

- keep Go as the only mutation authority
- remove top-level wrapper command paths in this release
- remove bash ownership from host networking, runtime bootstrap, and renderer/bootstrap flows
- remove legacy `.env` profile compatibility in the same release with explicit migration/rollback bundling

This means the target is a full ownership and surface cut in one externally breaking release: runtime behavior and wrapper paths both move to Go-owned command and runtime contracts.

This change fits directly on top of the archived `replace-operator-bash-with-go-controlplane` baseline. The control plane already owns command semantics, state, planning, image lifecycle, smoke-gated BNG rendering, and compatibility import/export. This architecture completes the ownership transfer by removing the remaining bash runtime islands.

## Components

- **Command Surface Consolidation Layer**: Removes wrapper script paths and consolidates user entrypoints under Go-owned `vcpe` command families, including service-scoped subcommands.
- **Host Network Controller**: Replaces `platform/scripts/lib/bridges.sh`, `firewall.sh`, and `platform/scripts/net` with Go-owned topology planning, bridge lifecycle, NAT/firewall management, and host capability preflight.
- **Runtime Init Library**: Shared Go bootstrap logic for service containers. It replaces bash entrypoint responsibilities such as interface naming, IPv6 readiness gating, runtime config placement, and final process exec.
- **Service Runtime Init Binaries**: Small service image entry binaries built from the shared runtime-init library. They provide service-specific bootstrap wiring without reintroducing shell control flow.
- **Renderer Completion Layer**: Expands typed Go renderers beyond smoke-gated BNG and eliminates remaining Python/bash-owned config generation from runtime-critical paths.
- **Catalog and Topology Expansion**: Extends the typed service catalog and topology planner so MAC identity, interface role assignment, and customer-specific network intents are derived from manifests and state rather than hardcoded bash tables.
- **Compose and Runtime Integration Layer**: Keeps the typed compose adapter but changes compose inputs and container startup contracts so service images boot through Go-owned init logic instead of shell entrypoints.
- **Verification and Deprecation Layer**: Replaces shim-focused validation with direct `vcpe` command coverage, runtime bootstrap tests, host-network tests, and compatibility-window retirement checks.

## Key Architectural Decisions

### Remove wrapper command paths in this release
**Choice**: Remove top-level wrapper scripts in this change and expose only Go-owned command families, including `vcpe service <name> ...` for service-scoped workflows.
**Rationale**: The release intentionally takes a broader external break to complete command-path migration now, not later.
**Alternatives considered**: Preserve wrappers for one compatibility window. Rejected by explicit product decision in this change.

### Define "remaining legacy bash" as runtime behavior ownership
**Choice**: Remove host-network scripts, container entrypoints, startup helpers, and renderer/bootstrap scripts in this same release alongside wrapper removal.
**Rationale**: Wrapper deletion alone would be cosmetic if runtime topology/bootstrap/render behavior remained shell-owned.
**Alternatives considered**: Wrapper-only removal with deferred runtime behavior migration. Rejected because it creates a shallow cut with high hidden risk.

### Move host topology and firewall ownership entirely into Go
**Choice**: Replace `platform/scripts/lib/bridges.sh`, `platform/scripts/lib/firewall.sh`, and `platform/scripts/net` with a Go Host Network Controller driven by manifest topology, IPAM state, and service catalog intent.
**Rationale**: Those scripts still hardcode customer IDs and host bridge behavior, which directly violates the typed topology direction and keeps privileged mutation logic outside the control plane.
**Alternatives considered**: Keep host setup in shell while removing other bash. Rejected because topology is a core mutation boundary; leaving it in bash preserves the highest-risk imperative surface.

### Standardize service startup on Go runtime-init binaries inside service images
**Choice**: Replace bash entrypoints with small Go runtime-init binaries built from a shared bootstrap library and embedded in service images.
**Rationale**: Container startup still owns critical logic today: MAC-based interface assignment, IPv6 readiness waiting, runtime config placement, sysctl application, and process handoff. A shared Go runtime-init path gives deterministic ordering, typed validation, and testable behavior while avoiding another distributed shell layer.
**Alternatives considered**: Pure compose-driven startup with no init binary. Rejected because interface naming, readiness waiting, and runtime config application still need ordered logic. Keep shell entrypoints and call Go helpers from them. Rejected because bash would remain the top-level control flow owner inside the container.

### Lock runtime bootstrap ordering explicitly
**Choice**: Every service runtime bootstrap follows the same ordered sequence: interface identity resolution, interface rename/verification, IPv6 readiness gate, runtime config application, kernel/network side effects, then final service exec.
**Rationale**: The current bash entrypoints rely on this order implicitly. If the new design does not lock it explicitly, startup races will move from shell into a less obvious failure mode.
**Alternatives considered**: Allow service-specific ordering. Rejected because it makes the architecture harder to reason about and harder to verify across images.

### Keep interface identity deterministic, but derive it from typed planning rather than bash tables
**Choice**: Preserve deterministic interface roles and MAC semantics for compatibility, but compute them from manifest/customer/service intent and control-plane state instead of hardcoded bash arrays.
**Rationale**: Existing services depend on stable interface identity. Removing bash must not introduce nondeterministic network-device mapping. The planner and catalog need to own this mapping explicitly.
**Alternatives considered**: Drop deterministic MAC/interface behavior and trust container network ordering. Rejected because it is operationally fragile and would almost certainly regress service bootstrap correctness.

### Replace remaining renderer/bootstrap scripts with typed Go renderers, not generic subprocess wrappers
**Choice**: Expand the typed renderer model until runtime-critical rendering no longer depends on Python/bash behavior. Non-migrated services continue to fail fast with explicit unsupported errors until their renderer is moved.
**Rationale**: Runtime rendering is part of deployment truth. If config generation still depends on opaque scripts, the system remains split-brain even if command invocation is Go-owned.
**Alternatives considered**: Keep a permanent adapter layer around Python/bash generators. Rejected because it preserves a hidden second architecture and extends the migration indefinitely.

### Shift verification to direct operator/runtime coverage
**Choice**: Verification prioritizes direct `vcpe` command flows, host-network controller tests, runtime-init tests, renderer parity tests, and representative Podman smoke tests for all documented service classes; shim contract tests are removed in this change.
**Rationale**: Wrapper paths are removed, so proof must come from the final Go-owned command and runtime surfaces.
**Alternatives considered**: Keep shim contract tests as a permanent primary gate. Rejected because they validate an interface that should eventually disappear.

## Data Flow

Primary control-plane apply after this change:

```text
User / CI
   |
   v
vcpe
   |
   v
Profile Store / Manifest Translator
   |
   v
Planner + Service Catalog + IPAM
   |
   +--> Host Network Controller
   |        |
   |        v
   |   bridges / NAT / firewall / capability checks
   |
   +--> Renderer Completion Layer
   |        |
   |        v
   |   runtime-config artifacts + compose inputs
   |
   v
Compose Adapter / Podman Backend
   |
   v
Service container starts runtime-init binary
   |
   v
interface identity -> IPv6 readiness -> config apply -> exec service
   |
   v
Health / Status / Journal persistence
```

Container bootstrap flow:

```text
Podman starts container
      |
      v
runtime-init
      |
      +--> read planned interface + MAC contract
      +--> rename or verify interfaces deterministically
      +--> wait for IPv6 global addresses to become usable
      +--> copy/apply rendered runtime-config payload
      +--> apply sysctl / firewall-side local service prerequisites
      +--> exec main service process
```

Primary command flow after wrapper removal:

```text
User / CI
   |
   v
vcpe (deployment + service subcommands)
```

## Integration Points

- **Existing control plane**: Reuses `controlplane/internal/app`, `planner`, `persist`, `state`, `hostnet`, `render`, `manifest`, and `catalog` as the primary orchestration boundary.
- **Platform scripts**: `platform/scripts/lib/bridges.sh`, `platform/scripts/lib/firewall.sh`, and `platform/scripts/net` are replaced by Go components rather than wrapped.
- **Service images**: Service Containerfiles and runtime startup contracts are modified to invoke runtime-init binaries instead of shell entrypoints.
- **Renderer assets**: Existing templates and service runtime artifacts remain inputs, but typed Go renderers own generation and validation.
- **Tests and smoke flows**: `tests/smoke/*`, README usage, and Make targets shift to direct `vcpe` usage; wrapper-path assumptions are removed.
- **OpenSpec constraints**: Must preserve the `local-control-plane-cli`, `dynamic-topology-ipam`, `podman-reconciliation-engine`, `profile-compat-translation`, `rendering-and-secrets-contract`, and `developer-readme-and-build-workflow` main specs.

## Security Model

- The Go control plane remains the only trusted mutation authority.
- Privileged host operations stay behind explicit host-network controller boundaries with preflight checks before mutation.
- Runtime-init binaries run inside service containers and only consume rendered artifacts and planned interface contracts; they do not resolve secrets themselves.
- Secrets remain apply-time inputs owned by the control plane and must not be persisted to logs, state, or runtime bootstrap diagnostics.
- Removing bash reduces shell-injection risk by eliminating string-concatenated runtime mutation paths in host and container startup code.
- Command-path compatibility dispatchers are removed in this release.

## Error Handling Strategy

- **Host network errors**: Fail during preflight or host-network apply phases before container mutation where possible. No shell fallback path is allowed.
- **Runtime-init errors**: Fail container startup deterministically with structured stderr and exit codes. Health checks should surface bootstrap phase and failure reason.
- **Interface identity errors**: Treated as non-retryable unless the planner contract or container attachment state changes.
- **IPv6 readiness errors**: Bounded retry inside runtime-init with explicit timeout and diagnostics; not an unbounded sleep loop.
- **Renderer gaps**: Fail before mutation with explicit unsupported-service or missing-artifact errors.
- **Command-surface errors**: Service and deployment subcommands return typed Go errors and consistent exit behavior with no wrapper translation layer.

The architecture intentionally avoids hidden shell fallbacks. If the Go path cannot perform a mutation safely, it fails explicitly rather than dropping back to bash.

## Observability Strategy

- Host-network controller logs bridge, NAT, firewall, and capability decisions as structured phase events.
- Runtime-init emits structured startup phase markers: `interface_contract`, `interface_ready`, `ipv6_ready`, `runtime_config_applied`, and `service_exec`.
- Metrics should distinguish host preflight failures, runtime bootstrap failures, renderer failures, migration-bundle failures, and service-subcommand failures.
- Traces should extend through the full bootstrap chain: plan -> host-network -> render -> compose -> runtime-init -> health.
- Observability includes migration-bundle generation/validation events for upgrade and rollback readiness.

## Constraints

- Go remains the only mutation authority.
- Wrapper script paths are intentionally removed in this change as an explicit breaking decision.
- Legacy `.env` profile compatibility is intentionally removed in this change as an explicit breaking decision.
- Dynamic topology and IPAM rules must not regress to hardcoded customer ID network tables.
- Deterministic interface identity for services must be preserved.
- Runtime config application must remain ordered and reproducible across service images.
- Non-migrated renderers must fail before mutation rather than silently falling back to script-owned behavior.
- No new long-term subprocess abstraction layer should be introduced to preserve shell logic under a different name.
- The architecture must stay local Podman-first and must not introduce Kubernetes or multi-node complexity.

## Diagrams

System ownership after this change:

```text
+---------------------+
| vcpe                |
| Go operator         |
+----------+----------+
           |
   +-------+--------+-------------------+
   |                |                   |
   v                v                   v
+------+     +-------------+     +-------------+
| plan |     | hostnetwork |     | renderers   |
|+cat  |     | controller  |     | typed Go    |
+--+---+     +------+------+     +------+------+ 
   |                 |                    |
   +--------+--------+---------+----------+
            |                  |
            v                  v
      +-----------+      +-----------+
      | compose    |----->| container |
      | adapter    |      | runtime-  |
      +-----------+      | init      |
                         +-----+-----+
                               |
                               v
                           main service
```

Behavior-removal sequencing:

```text
Phase 1: staged internal cutover (feature-flagged)

hostnetwork + runtime-init + render paths
   -> migrate per service/component behind explicit Go flags
   -> prove parity with direct vcpe coverage + representative smoke

Phase 2: single externally breaking release

remove scripts/* wrappers
remove shell entrypoints/startup scripts
remove legacy .env profile compatibility
switch to vcpe + vcpe service <name> as the only command surface

Phase 3: post-cut stabilization

remove transitional cutover flags
retain migration/rollback bundle tooling for upgrade safety

Legacy behavior replacements:

platform/scripts/lib/*      -> Go host-network controller
service entrypoint.sh       -> runtime-init binaries
python/bash render helpers  -> typed Go renderers
```

Runtime-init sequence:

```text
container start
   |
   v
runtime-init
   |
   +--> verify interface contract
   +--> rename or validate eth0/eth1/eth2 mapping
   +--> wait for IPv6 readiness
   +--> apply runtime config payload
   +--> apply bootstrap side effects
   +--> exec main process
```