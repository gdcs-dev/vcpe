## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| Remove top-level compatibility wrappers in this change | `scripts/vcpe`, `scripts/bng`, `scripts/gateway`, `scripts/routerd`, `scripts/webpa`, `scripts/xb10`, `scripts/client`, `scripts/net`, docs, smoke flows, CLI help | Yes |
| Remove legacy `.env` profile compatibility in the same release | `config/profiles/*.env`, profile import/export flows, migration docs, rollback docs, state export tooling | Yes |
| Replace shell entrypoints with Go runtime-init binaries | `services/*/container/entrypoint.sh`, service Containerfiles, startup flows, health verification | No |
| Remove hardcoded customer/network tables and derive host-network state from typed intent | `platform/scripts/lib/bridges.sh`, `platform/scripts/lib/firewall.sh`, `platform/scripts/net`, controlplane host-network/state/catalog code | No |
| Migrate all documented service paths in the same change | service command surface, renderers, runtime bootstrap, smoke/integration coverage, CLI subcommands | No |
| Introduce `vcpe service <name> ...` as the Go-owned service workflow surface | CLI command tree, docs, tests, service-level workflows | No |

---

## Decisions

### Decision: Remove wrapper scripts in the same change
[BREAKING][OVERRIDE]
Recommendation: Preserve the current top-level compatibility wrappers for the full promised one-release window, while only removing deeper behavior-owning bash in platform/runtime/render paths.
Decision: Remove the wrappers in this change as well.
Rationale: The change intentionally makes wrapper path removal part of the breaking release rather than waiting for a later compatibility-window retirement.

Q: Should this change preserve the current top-level compatibility wrappers (`scripts/vcpe`, `scripts/bng`, `scripts/net`, and the other service wrappers) for the full promised one-release window, while only removing deeper behavior-owning bash in platform/runtime/render paths?
A: remove the wrappers in this change as well

---

### Decision: Runtime bootstrap mechanism
[BREAKING]
Recommendation: Use small Go runtime-init binaries embedded in each service image.
Decision: Use small Go runtime-init binaries embedded in each service image.
Rationale: Service startup still requires deterministic ordered bootstrap that compose alone does not express well.

Q: For the remaining container bootstrap logic, should we replace bash entrypoints with small Go runtime-init binaries embedded in each service image, or try to eliminate entrypoint logic by pushing startup behavior into compose/runtime configuration alone?
A: Use small Go runtime-init binaries embedded in each service image

---

### Decision: Interface identity contract preservation
[BREAKING]
Recommendation: Preserve the MAC-based contract during this change, but make the catalog/planner compute it from typed manifest and state instead of bash tables.
Decision: Preserve the MAC-based contract during this change, but make the catalog/planner compute it from typed manifest and state instead of bash tables.
Rationale: This keeps blast radius contained while removing bash as the owner of interface identity.

Q: Should the new Go runtime-init path preserve the current MAC-based interface identity contract (`MGMT_MAC` -> `eth0`, `WAN_MAC` -> `eth1`, `CM_MAC` -> `eth2`) during migration, or should we break that contract and move to a new interface-binding model?
A: Preserve the MAC-based contract during this change, but make the catalog/planner compute it from typed manifest and state instead of bash tables.

---

### Decision: Runtime config application boundary
[BREAKING]
Recommendation: Apply runtime configuration inside the Go runtime-init binary at container startup.
Decision: Apply runtime configuration inside the Go runtime-init binary at container startup.
Rationale: Final activation of service-local files and network-dependent settings belongs inside the container boundary.

Q: Where should runtime configuration application happen after we remove shell entrypoints: inside the new per-service Go runtime-init binary at container startup, or outside the container in the control plane before container launch?
A: Apply runtime configuration inside the Go runtime-init binary at container startup.

---

### Decision: Host-network source of truth
[BREAKING]
Recommendation: Remove the hardcoded tables in this change and make host-network creation fully derived from typed manifest/profile intent and state.
Decision: Remove the hardcoded tables in this change and make host-network creation fully derived from typed manifest/profile intent and state.
Rationale: Removing bash must not simply copy hidden hardcoded topology into Go.

Q: For host networking, should this change fully remove the hardcoded customer ID bridge/network tables (`7`, `9`, `20`) and require all bridge, NAT, and Podman network creation to be derived from typed manifest/profile intent plus persisted allocation state?
A: remove the hardcoded tables in this change and make host-network creation fully derived from typed manifest/profile intent and state.

---

### Decision: Supported service migration scope
[BREAKING]
Recommendation: Migrate all currently documented service paths in this change.
Decision: Migrate all currently documented service paths in this change.
Rationale: Once the legacy layer is removed, the documented support surface should still work.

Q: Should this change migrate all currently documented service paths (`bng`, `gateway`, `routerd`, `webpa`, `xb10`, and `client`) off legacy bash ownership in the same slice, or is it acceptable to remove the bash implementations while leaving some services explicitly unsupported until a later follow-up?
A: Migrate all currently documented service paths in this change

---

### Decision: Shared bootstrap implementation model
[BREAKING]
Recommendation: Use a single shared Go bootstrap library with thin per-service binaries.
Decision: Use a single shared Go bootstrap library with thin per-service binaries.
Rationale: Shared operational semantics are easier to keep correct and testable across services.

Q: Should the runtime-init implementation be a single shared Go bootstrap library with small per-service binaries (`bng-init`, `gateway-init`, `routerd-init`, etc.) built from that shared core, or should each service own a fully separate bootstrap implementation?
A: Use a single shared Go bootstrap library with thin per-service binaries.

---

### Decision: Service-scoped command surface
[BREAKING]
Recommendation: Preserve service-scoped workflows as first-class Go subcommands.
Decision: Preserve service-scoped workflows as first-class Go subcommands.
Rationale: Service-level debugging and partial lifecycle workflows remain part of the operator contract after wrapper removal.

Q: After removing the legacy service wrapper scripts, should we preserve service-scoped operator workflows as first-class Go subcommands such as `vcpe service bng ...`, `vcpe service gateway ...`, and `vcpe service client ...`, or collapse everything into top-level deployment-oriented `vcpe` commands only?
A: Preserve service-scoped workflows as first-class Go subcommands.

---

### Decision: Startup contract artifact form
[BREAKING]
Recommendation: Materialize an explicit typed startup contract artifact in the rendered runtime payload for each service instance.
Decision: Materialize an explicit typed startup contract artifact in the rendered runtime payload for each service instance.
Rationale: Runtime-init should consume a single explicit startup source of truth rather than infer behavior from incidental container state.

Q: Where should the per-container bootstrap contract live for runtime-init to consume: should the control plane materialize a typed startup contract artifact in the rendered runtime payload for each service instance, or should runtime-init reconstruct its expectations indirectly from environment variables and mounted files?
A: Materialize an explicit typed startup contract artifact in the rendered runtime payload for each service instance.

---

### Decision: Internal cutover strategy
[BREAKING]
Recommendation: Use a staged internal cutover during implementation, but ship this OpenSpec change as a single externally breaking release once all documented services pass the new path.
Decision: Use a staged internal cutover during implementation, but ship this OpenSpec change as a single externally breaking release once all documented services pass the new path.
Rationale: Internal risk should be partitioned even if the external release boundary is a single break.

Q: Should the runtime-init and host-network rewrites land behind a staged per-service/per-component cutover mechanism during implementation, or should this change switch all documented services and host-network behavior to the new Go-owned path in one atomic breaking release?
A: Use a staged internal cutover during implementation, but ship this OpenSpec change as a single externally breaking release once all documented services pass the new path

---

### Decision: Runtime-init failure semantics
[BREAKING]
Recommendation: Treat runtime-init failure as a hard deployment failure that triggers bounded rollback of the current operation.
Decision: Treat runtime-init failure as a hard deployment failure that triggers bounded rollback of the current operation.
Rationale: A startup-contract failure means the service never reached the desired converged state.

Q: When a service instance fails in the new runtime-init phase, should the control plane treat that as a hard deployment failure that triggers bounded rollback of the current operation, or should it allow partial convergence and leave failed instances for later manual repair?
A: Treat runtime-init failure as a hard deployment failure that triggers bounded rollback of the current operation.

---

### Decision: Release test gate strength
[BREAKING]
Recommendation: Require every documented service path to pass direct `vcpe` integration coverage, and require representative Podman smoke coverage for each service class before shipping.
Decision: Require every documented service path to pass direct `vcpe` integration coverage, and require representative Podman smoke coverage for each service class before shipping.
Rationale: Full replacement of a legacy implementation requires stronger-than-normal evidence, not weaker evidence.

Q: What should be the required release gate before this change can ship: must every documented service path pass direct `vcpe` integration tests plus representative Podman smoke coverage on the new Go-owned host-network and runtime-init path, or is lighter coverage acceptable for some services?
A: Require every documented service path to pass direct `vcpe` integration coverage, and require representative Podman smoke coverage for each service class before shipping.

---

### Decision: Startup contract and allocation persistence
[BREAKING]
Recommendation: Persist the startup contracts and derived MAC/interface allocations as first-class control-plane state and operation artifacts.
Decision: Persist them as first-class control-plane state and operation artifacts.
Rationale: Durable startup contracts are necessary for rollback, status, and post-failure inspection.

Q: Should the per-instance startup contract and derived MAC/interface allocations be persisted as first-class control-plane state for each operation/deployment, or should runtime-init recompute them from manifests and catalog inputs on every apply?
A: Persist them as first-class control-plane state and operation artifacts.

---

### Decision: Remove legacy env profile compatibility in the same release
[BREAKING][OVERRIDE]
Recommendation: Keep legacy `.env` profile import/export compatibility in Go for at least one release after wrapper removal.
Decision: Remove legacy `.env` profile compatibility at the same time as the wrappers and runtime bash.
Rationale: The release intentionally combines execution-path and profile-format removal into one architecturally cleaner, but broader, breaking change.

Q: Should this change keep legacy `.env` profile import/export compatibility in the Go control plane after the bash implementations and wrappers are removed, or should it make canonical typed manifests the only supported operator input/output?
A: remove `.env` profile compatibility at the same time as the wrappers and runtime bash. Tradeoff: architecturally cleaner, but the concrete consequence is that users must migrate both execution paths and profile formats in one release. That combines two separate breaking changes and makes rollback materially harder.

---

### Decision: Startup contract versioning model
[BREAKING]
Recommendation: Version the startup contract independently from the top-level deployment manifest schema.
Decision: Version the startup contract independently.
Rationale: Internal execution artifacts should evolve separately from the user-facing manifest API.

Q: Should the new typed startup contract be versioned independently from the top-level deployment manifest schema, or should it always share the same schema version as the manifest that produced it?
A: Version the startup contract independently.

---

### Decision: macOS support model
[BREAKING]
Recommendation: Continue to support macOS through the existing Podman machine / Linux-host delegation model.
Decision: Continue to support macOS through the existing Podman machine / Linux-host delegation model.
Rationale: The change removes legacy bash ownership; it does not intentionally shrink the supported host matrix.

Q: After host-network bash is removed, should the new Go host-network controller continue to support macOS through the current Podman machine / Linux-host delegation model, or should this change narrow support to native Linux hosts only?
A: Continue to support macOS through the existing Podman machine / Linux-host delegation model.

---

### Decision: Bootstrap diagnostics visibility
[BREAKING]
Recommendation: Expose runtime-init and startup-contract diagnostics as first-class per-service-instance data in `vcpe status` and `vcpe logs`.
Decision: Expose runtime-init and startup-contract diagnostics as first-class per-service-instance data in `vcpe status` and `vcpe logs`.
Rationale: A first-class startup contract should have first-class startup observability.

Q: Should `vcpe status` and `vcpe logs` expose runtime-init and startup-contract diagnostics as first-class per-service-instance data, or should bootstrap internals remain mostly hidden unless a user inspects low-level container logs?
A: Expose runtime-init and startup-contract diagnostics as first-class per-service-instance data in `vcpe status` and `vcpe logs`.

---

### Decision: Pre-upgrade migration and rollback bundle
[BREAKING]
Recommendation: Require an explicit pre-upgrade migration artifact and rollback bundle.
Decision: Yes, require an explicit pre-upgrade migration artifact and rollback bundle.
Rationale: Multiple simultaneous breaking changes require a stronger rollback story than a normal release.

Q: Because this change removes wrapper paths, runtime bash, and `.env` profile compatibility in one release, should we require an explicit pre-upgrade migration artifact and rollback bundle, such as exported canonical manifests plus persisted startup contracts/state snapshots, before users can apply the new release?
A: Yes, require an explicit pre-upgrade migration artifact and rollback bundle.

---

### Decision: Startup contract artifact format and location
[BREAKING]
Recommendation: Use a versioned JSON startup contract file, rendered per service instance into the runtime payload and persisted as both an operation artifact and deployment-scoped state snapshot.
Decision: Use a versioned JSON startup contract file, rendered per service instance into the runtime payload and persisted as both an operation artifact and deployment-scoped state snapshot.
Rationale: JSON plus dual persistence gives runtime-init a direct typed input and gives operators deterministic forensic state.

Q: What exact form should the per-instance startup contract take, and where should it live at runtime?
A: Use a versioned JSON startup contract file, rendered per service instance into the runtime payload and persisted as both an operation artifact and deployment-scoped state snapshot.

---

### Decision: Migration bundle ownership and contents
[BREAKING]
Recommendation: Make `vcpe` generate a single versioned migration bundle per deployment/customer that contains both the upgrade input and the rollback payload.
Decision: Make `vcpe` generate a single versioned migration bundle per deployment/customer that contains both the upgrade input and the rollback payload.
Rationale: Migration and rollback must remain deterministic and Go-owned after `.env` compatibility and wrapper removal.

Q: What should the required pre-upgrade migration artifact and rollback bundle contain, and who should generate it?
A: Make `vcpe` generate a single versioned migration bundle per deployment/customer that contains both the upgrade input and the rollback payload.

---

### Decision: Service workflow command grammar
[BREAKING]
Recommendation: Use `vcpe service <name> ...`.
Decision: Use `vcpe service <name> ...`.
Rationale: A single explicit service namespace keeps the command surface coherent after wrapper removal.

Q: What exact Go command shape should preserve the service-scoped workflows after wrapper removal: `vcpe service <name> ...`, or top-level service nouns such as `vcpe bng ...`, `vcpe gateway ...`, and `vcpe client ...`?
A: Use `vcpe service <name> ...`.

---

### Decision: Internal cutover control surface
[BREAKING]
Recommendation: Use explicit catalog/runtime feature flags in the Go control plane.
Decision: Use explicit catalog/runtime feature flags in the Go control plane.
Rationale: Migration state should remain visible in planning, status, and tests rather than being buried in image builds.

Q: For the staged internal cutover during implementation, should we drive old-vs-new behavior through explicit catalog/runtime feature flags in the Go control plane, or through build-time/image-level branching as services are migrated?
A: Use explicit catalog/runtime feature flags in the Go control plane.

---

### Decision: Source of truth for required migrated service paths
[BREAKING]
Recommendation: Use the user-facing command/documentation surface in the repo as the source of truth, and make that set explicit in the change.
Decision: Use the user-facing command/documentation surface in the repo as the source of truth, and make that set explicit in the change.
Rationale: The replacement must satisfy the public support surface rather than a private engineering subset.

Q: What should be the source of truth for “all currently documented service paths” that must be migrated before release: the user-facing command/documentation surface in the repo, or a narrower implementation-maintained allowlist?
A: Use the user-facing command/documentation surface in the repo as the source of truth, and make that set explicit in the change.

---