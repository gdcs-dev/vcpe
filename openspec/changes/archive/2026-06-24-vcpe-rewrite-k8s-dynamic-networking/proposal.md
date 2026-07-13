## Why

The current vCPE workflow is tightly coupled to Bash orchestration, hardcoded network topology, and env-file-only configuration, which makes local testing brittle and difficult to evolve. A local-first declarative control plane is needed now to support safer scale up/down testing, dynamic network mapping, and deterministic recovery behavior on a single Podman host.

## What Changes

- Introduce a Go-based local control-plane CLI with declarative `plan`, `apply`, `status`, and `destroy` operations.
- Add versioned YAML/JSON desired-state manifests with explicit profile, deployment, and runtime status separation.
- Add a topology planner and IPAM for dynamic network mapping and customer onboarding without hardcoded customer IDs.
- Implement a Podman-only reconciler with SQLite-backed state, single-writer semantics, crash recovery, and bounded rollback.
- Add migration tooling to import/export existing env profiles during transition.
- Replace fragile regex-style config rendering with typed render inputs and validated templates.
- Enforce local resource/safety controls: bounded replica/customer limits, disruptive-change gating, and explicit override flags.

## Capabilities

### New Capabilities
- `local-control-plane-cli`: Declarative local control-plane contract (`plan`/`apply`/`status`/`destroy`) with optional daemon mode.
- `desired-state-manifests`: Versioned YAML/JSON schema for profiles, customer deployments, runtime status, and validation.
- `dynamic-topology-ipam`: Dynamic network planning and address allocation for Podman-backed customer/service attachments.
- `podman-reconciliation-engine`: Idempotent reconcile/apply/rollback engine with SQLite journal and crash recovery.
- `profile-compat-translation`: Import/export compatibility layer between legacy env profiles and canonical manifests.
- `rendering-and-secrets-contract`: Typed rendering inputs, phase-1 secret providers (`env`, `file`), and log redaction policy.

### Modified Capabilities
- None (no existing capability specs under `openspec/specs/`).

## Impact

- Affected orchestration entrypoints and wrappers: `scripts/vcpe`, `scripts/bng`, `scripts/gateway`, `scripts/routerd`, `scripts/webpa`, `scripts/xb10`, `scripts/client`.
- Affected configuration model: `config/profiles/*.env`, `config/defaults.env`, and service runtime config generation paths.
- Affected network/control behavior: bridge/network planning logic in `platform/scripts/lib/*` and service network attachment semantics.
- New dependencies: Go toolchain for control-plane binary and SQLite for local durable state.
- Testing impact: expanded unit + Podman integration + smoke gating required for rollout.
