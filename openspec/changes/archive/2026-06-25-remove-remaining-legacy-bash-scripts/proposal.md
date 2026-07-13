## Why

The repository still has behavior-owning legacy bash in host networking, container bootstrap, and renderer/startup paths, which keeps critical mutation logic outside the Go control plane. This change completes the ownership transfer now, in one intentional breaking release, so runtime behavior and operator command surfaces are both Go-owned and testable.

## What Changes

- Remove all remaining legacy behavior-owning bash in host-network, runtime bootstrap, and renderer/startup flows.
- Replace platform host-network shell paths with a Go Host Network Controller driven by typed manifest/profile intent and persisted allocation state.
- Replace service shell entrypoints with per-service Go runtime-init binaries built from a shared runtime bootstrap library.
- Preserve deterministic interface role behavior by deriving MAC/interface contracts from typed planning and catalog/state inputs rather than hardcoded shell tables.
- Materialize a versioned JSON startup contract per service instance and persist it as operation and deployment-scoped state artifacts.
- Expand typed Go rendering so runtime-critical config generation is Go-owned across all documented services.
- Introduce and standardize service-scoped workflows under `vcpe service <name> ...`.
- **BREAKING** Remove top-level wrapper script command paths in this release.
- **BREAKING** Remove legacy `.env` profile import/export compatibility in this release; canonical typed manifests become the only supported operator input/output.
- **BREAKING** Require all documented service paths to pass direct `vcpe` integration coverage and representative Podman smoke coverage before release.
- **BREAKING** Require explicit pre-upgrade migration + rollback bundle generation from `vcpe` for safe upgrades and rollback.

## Capabilities

### New Capabilities
- `runtime-init-contract-and-bootstrap`: Typed startup contracts plus shared Go runtime-init bootstrap for all documented service classes.
- `host-network-controller-go-ownership`: Go-owned bridge/NAT/firewall/topology reconciliation derived from typed intent and state.
- `service-subcommands-and-diagnostics`: Service-scoped Go command surface (`vcpe service <name> ...`) and first-class startup diagnostics in status/log outputs.
- `migration-bundle-and-rollback-contract`: Versioned migration artifact and rollback bundle generation/validation owned by `vcpe`.

### Modified Capabilities
- `local-control-plane-cli`: Remove wrapper compatibility paths; update command surface and operator flows to direct `vcpe` + `vcpe service` usage.
- `dynamic-topology-ipam`: Remove hardcoded customer/network tables and enforce typed intent + persisted allocation state as the network source of truth.
- `podman-reconciliation-engine`: Integrate startup-contract persistence, runtime-init execution semantics, and bounded rollback-on-bootstrap-failure behavior.
- `profile-compat-translation`: Remove `.env` compatibility translation path and enforce typed manifest-only profile model.
- `rendering-and-secrets-contract`: Complete renderer migration for documented service runtime paths and fail fast for unsupported gaps prior to mutation.
- `developer-readme-and-build-workflow`: Update docs, smoke/test gates, and release checks for wrapper removal, service-subcommand workflows, and migration-bundle requirements.

## Impact

- Affects command entrypoints, host network orchestration, service image startup paths, rendering flows, state persistence, diagnostics, docs, and smoke/integration gating.
- Removes wrapper scripts and `.env` compatibility in one release, requiring users to migrate command usage and profile formats together.
- Requires service Containerfile and runtime startup contract updates across documented services (`bng`, `gateway`, `routerd`, `webpa`, `xb10`, `client`).
- Increases release gate strictness to prove parity and stability across all documented service classes before shipping.
- Adds migration/rollback artifacts to support safer upgrades across the broader breaking cut.
