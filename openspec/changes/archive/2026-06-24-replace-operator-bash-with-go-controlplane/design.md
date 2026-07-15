## Context

The previous control-plane change introduced a Go `vcpectl` binary with manifest validation, planning, apply/status/destroy flows, SQLite state, writer locking, operation journaling, and Podman-oriented backend seams. The operator experience is still mostly owned by bash: top-level scripts source `.env` profiles, sequence service scripts, invoke image builds, render service files, call `podman-compose`, manage network bootstrap, and expose status/log/config/profile commands.

This design implements the proposal by moving public operator behavior into Go while preserving the existing local Podman workflow. The architecture and decisions artifacts define the high-level boundary: Go owns behavior, scripts become shims, Podman remains the only backend, daemon/watch is deferred, and compatibility lasts one release window after parity.

## Goals / Non-Goals

**Goals:**

- Add a real Go public command surface for `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `profile`, `plan`, `apply`, and `destroy`.
- Move profile loading, active-profile selection, profile import/export, validation, service ordering, image policy, compose invocation, host-network preflight, render invocation, status, and logs into typed Go components.
- Keep existing script paths as compatibility shims that translate to Go commands and propagate exit codes.
- Add functional minimum models for service catalog, images, dependencies, render contracts, compose groups, host-network intent, and profile translation warnings.
- Support real first-pass renderer adapters for smoke-gated `bng-7` and `bng-20` paths.
- Preserve documented workflows and supported legacy profile keys while warning on unsupported mappings.

**Non-Goals:**

- No Kubernetes, multi-node, or alternate container backend support.
- No daemon/watch reconciliation implementation in this change.
- No full native Go compose reconciliation in the first implementation; a typed `podman-compose` adapter is sufficient.
- No immediate renderer rewrite for every service.
- No guarantee that undocumented bash internals, direct shell sourcing assumptions, or non-JSON human output formatting remain byte-for-byte stable.

## Decisions

### Command routing and binary shape

The packaged user-facing command is `vcpe`. The existing `vcpectl` entrypoint remains available as an alias/debug path during migration. Internally both names should call the same Go command registration so behavior cannot diverge.

Top-level commands are grouped as:

- Configuration/profile commands: `init`, `config show`, `config set`, `profile list`, `profile show`, `profile use`, `profile create`, `profile set`, `profile import`, `profile export`.
- Lifecycle commands: `plan`, `apply`, `up`, `destroy`, `down`, `status`, `logs`.
- Image commands: `build`, `push`, and pull-policy handling during `up`/`apply`.
- Compatibility service commands: service-scoped forms such as `vcpe service bng up 7` may be implemented to support script shims, but the primary documented UX remains top-level `vcpe` commands.

The first implementation must make these Go commands real. Bash wrappers may continue to exist, but they only translate arguments and call Go.

### Profile store and translator

Add a Go profile package that reads built-in defaults and profiles from `config/`, manages user config under the existing user config path, and records the active profile. It must parse env-style profile files without sourcing shell code.

The translator converts supported legacy profile fields into canonical manifests and can export supported fields back to env-style snapshots. Unsupported fields produce `ProfileTranslationWarning` values with field name, severity, impact, and preservation status. Supported fields must round-trip exactly.

### Service catalog and dependency planning

Add a service catalog package with functional entries for at least BNG and any service needed by smoke-gated `bng-7` and `bng-20` flows. Catalog entries include:

- service name and supported customer/profile selectors
- image metadata and build context
- dependencies and default ordering
- compose group definition and project naming inputs
- render contract and renderer adapter selector
- health check and log selectors
- host-network intents required by the service

The planner consumes catalog entries and manifest/profile intent to produce deterministic start, health, teardown, and rollback order. Optional manifest overrides may refine ordering, but normal profiles should work from catalog defaults.

### Image lifecycle

Add `ServiceImage` and image manager behavior for build, pull, push, tag, and pull policy. `vcpe build` performs explicit builds for enabled services. `vcpe up` or `apply` may build missing images only when the selected image policy allows it, such as local-development or build-if-missing policies. Registry-oriented profiles can require pull or fail when local images are missing.

The Podman backend should expose typed methods for image existence, build, pull, push, and tag. Command handlers should not shell out directly.

### Compose adapter

Add a typed compose adapter that initially invokes `podman-compose` or equivalent Podman compose commands. Go owns:

- generated env/compose input paths
- project names
- command timeouts
- operation journal entries
- status inspection
- rollback bookkeeping

The adapter boundary allows later native compose parsing without changing command semantics.

### Render adapters

Add a renderer interface that accepts typed render inputs and returns explicit artifacts. The first implementation must provide real adapter-backed rendering for smoke-gated `bng-7` and `bng-20` paths. Temporary Python/shell invocation is allowed only inside renderer adapters with structured arguments and explicit output validation.

Non-migrated service render paths must either use an explicit typed adapter or fail before mutation with a clear unsupported-service error in plan/apply output.

### Host-network adapter

Add a host-network adapter for bridge, NAT, firewall, and capability checks. Apply must preflight required capabilities before mutating runtime resources. The operator must not prompt for sudo mid-apply. If capabilities are missing, it fails with an actionable preflight error.

### State and generated artifacts

Generated manifests, render outputs, operation journals, and compatibility snapshots live under versioned control-plane state directories. User-owned profiles stay under existing config paths. Plans and status output should include the state path used for generated artifacts when useful for debugging.

### Status and logs

`status` defaults to human output and supports `--json`. It compares desired state, planned state, and observed Podman state by profile/customer/service/resource. `logs` defaults to container logs selected by catalog log selectors and may include operation journal context as a separate section or JSON field.

### Compatibility shims

Existing `scripts/*` remain for one release window after Go parity. They must:

- call the Go operator command
- preserve documented argument shapes where possible
- emit deprecation or migration warnings when appropriate
- propagate Go exit codes
- avoid sourcing profiles or mutating deployment state themselves

## Risks / Trade-offs

- Renderer parity can block smoke flows -> require real first-pass adapters for `bng-7` and `bng-20`, plus golden tests for rendered inputs and outputs.
- Compose remains an external subprocess initially -> isolate it behind a typed adapter with timeouts, structured arguments, and journaled operations.
- Host networking requires privileges -> preflight capabilities before mutation and keep bridge/firewall operations behind a single adapter.
- Script compatibility can drift -> make scripts argument translators only and add command-contract tests.
- Full command parity is broad -> phase internals, but require the Go public command surface to land first.
- Existing human output may change -> preserve JSON contracts and documented workflows; allow undocumented formatting changes.

## Migration Plan

1. Add Go command registration for the full public command surface while preserving existing `plan`, `apply`, `status`, and `destroy` behavior.
2. Add profile store and translator support for built-in and user profiles, active-profile selection, import/export, and structured warnings.
3. Add service catalog models and functional BNG catalog entries for smoke-gated paths.
4. Add image manager and Podman image backend methods.
5. Add typed compose and host-network adapters with preflight and journal integration.
6. Add renderer interface and real adapter-backed rendering for `bng-7` and `bng-20`.
7. Wire `build`, `up`, `down`, `status`, and `logs` through catalog/planner/backend components.
8. Convert top-level scripts and service scripts into thin Go-command shims.
9. Update documentation, packaging, Make targets, and smoke tests for `vcpe` primary binary behavior.
10. Keep legacy script compatibility for one release window after parity, then remove or simplify service-level shims in a later change.

Rollback during implementation is straightforward because scripts can remain available until their replacements pass command-contract and smoke tests. Runtime rollback remains governed by the existing control-plane operation journal and bounded rollback model.

## Open Questions

None blocking. Native compose reconciliation, daemon/watch workflows, and full renderer rewrites are intentionally deferred follow-up changes.
