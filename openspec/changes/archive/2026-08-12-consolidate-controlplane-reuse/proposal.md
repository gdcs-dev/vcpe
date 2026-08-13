## Why

The control plane repeats renderer lifecycle, service-type defaults, image backend request translation, deterministic environment formatting, and small network/image helpers across packages. These copies already disagree in user-visible defaults and edge-case behavior, increasing the cost and risk of adding service types or changing shared contracts.

## What Changes

- Introduce shared, typed renderer lifecycle infrastructure for config decoding, replica expansion, canonical artifact placement, and Compose fragment aggregation while keeping service-specific configuration and runtime policy in each service package.
- Introduce reusable service-type defaults for curated health metadata, build image policy, and no-op interface validation, with explicit overrides for types whose behavior differs.
- Make the manifest wizard consume default image metadata from the service-type registry instead of maintaining an incomplete parallel type switch.
- Make Docker and Podman image adapters consume the image lifecycle request contract directly, removing duplicate request structures and forwarding-only wrappers.
- Establish one canonical image-reference formatter and define empty-repository behavior consistently across rendering and image lifecycle operations.
- Add focused shared helpers for deterministic environment entries and conventional per-instance environment artifacts.
- Consolidate only exact network utility duplication where ownership is unambiguous; remove unused copies rather than creating broad utility packages.
- Add cross-type compatibility tests that freeze artifact keys, replica naming, Compose network attachments, health publication, image defaults, and interface environment behavior before migrating implementations.
- Preserve the manifest schema, CLI contract, persisted state format, generated artifact layout, runtime-init binaries, service-specific Compose policy, addressing semantics, and existing error boundaries.

## Capabilities

### New Capabilities
- `controlplane-code-reuse`: Defines shared implementation contracts, compatibility guarantees, ownership boundaries, and migration verification for reusable control-plane behavior.

### Modified Capabilities

None. This change preserves the externally observable requirements of the existing rendering, registry, addressing, reconciliation, image lifecycle, CLI, and persistence capabilities.

## Impact

- Primary code areas: `controlplane/internal/render`, `controlplane/internal/typeregistry`, `controlplane/internal/types/*`, `controlplane/internal/image`, `controlplane/internal/backend/{docker,podman}`, `controlplane/internal/app`, `controlplane/internal/ipam`, and `controlplane/internal/planner`.
- Internal APIs change where concrete image adapters adopt `image.Backend` request types and built-in service types delegate common behavior to shared implementations.
- Generated artifacts and operator-facing behavior remain compatible; migrations require before/after tests for artifact paths and semantic Compose content.
- No new runtime dependency, external service, manifest field, CLI flag, database migration, or persisted-state migration is introduced.
- Implementation should proceed in independently testable slices so renderer, registry, image backend, and utility consolidation can be reviewed and reverted separately.