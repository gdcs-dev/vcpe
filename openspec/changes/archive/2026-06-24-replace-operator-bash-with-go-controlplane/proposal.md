## Why

The current operator workflow is split between bash scripts and the Go control plane, which leaves profile parsing, service ordering, image lifecycle, rendering, host networking, logs, and compatibility behavior outside the stateful reconciliation model. Moving the operator surface into Go makes the control plane authoritative while preserving local Podman development workflows and existing script paths during migration.

## What Changes

- **BREAKING**: Make the Go operator the owner of the full public command surface: `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `profile`, `plan`, `apply`, and `destroy`.
- **BREAKING**: Convert existing bash scripts into temporary compatibility shims that translate to Go commands for one release window after parity.
- **BREAKING**: Package `vcpe` as the primary user-facing Go command and retain `vcpectl` as an alias/debug path during migration.
- Add a typed service catalog that defines service metadata, dependencies, image policies, render contracts, compose groups, health checks, and log selectors.
- Move build/pull/push/tag decisions into a Go image manager backed by typed Podman operations.
- Add a typed `podman-compose` adapter as the first compose migration step, with Go-owned generated inputs, project naming, plan records, and rollback tracking.
- Add typed renderer adapters, with real first-pass support for smoke-gated `bng-7` and `bng-20` paths and explicit unsupported errors for non-migrated paths.
- Add host-network preflight and adapter boundaries for bridge, NAT, firewall, and capability checks.
- Preserve supported legacy `.env` profile round-trip behavior and emit structured warnings for unsupported fields.
- Store generated manifests, rendered outputs, operation journals, and compatibility snapshots under versioned control-plane state paths.
- Preserve documented workflows and supported profile keys while allowing undocumented bash internals, env side effects, and non-JSON human output formatting to change.

## Capabilities

### New Capabilities

None. This change extends the existing control-plane, reconciliation, profile, rendering, and topology capabilities rather than introducing an independent capability area.

### Modified Capabilities

- `local-control-plane-cli`: Expands the CLI contract from lifecycle commands to the full Go operator command surface, including compatibility aliases and human/JSON output behavior.
- `podman-reconciliation-engine`: Adds service catalog planning, image lifecycle phases, typed compose adapter behavior, host-network preflight, generated artifact state paths, and command-driven rollback boundaries.
- `profile-compat-translation`: Strengthens import/export guarantees for supported legacy profile fields and structured warnings for unsupported fields.
- `rendering-and-secrets-contract`: Adds typed renderer adapter behavior and first-pass real renderer requirements for `bng-7` and `bng-20`.
- `dynamic-topology-ipam`: Ensures service catalog and host-network planning do not regress to hardcoded customer ID network tables.
- `developer-readme-and-build-workflow`: Updates documentation requirements for `vcpe` as the primary Go command and scripts as compatibility shims during migration.

## Impact

- Affected Go code: `controlplane/cmd/vcpectl`, `controlplane/internal/app`, `controlplane/internal/manifest`, `controlplane/internal/planner`, `controlplane/internal/persist`, `controlplane/internal/backend/podman`, `controlplane/internal/render`, and new catalog/profile/image/network adapter packages as needed.
- Affected scripts: `scripts/vcpe`, `scripts/bng`, `scripts/gateway`, `scripts/routerd`, `scripts/webpa`, `scripts/xb10`, `scripts/client`, `scripts/net`, and service-level scripts that become migration references rather than behavior owners.
- Affected service assets: compose files, Containerfiles, templates, customer fixtures, and runtime artifact paths under `services/`.
- Affected packaging/docs: primary binary naming, Homebrew packaging inputs, README/runbook command examples, and Makefile helper targets.
- Test impact: requires Go unit tests, golden profile/render tests, command-contract tests for shims, and Podman smoke coverage for representative `bng-7` and `bng-20` flows when Podman is available.
