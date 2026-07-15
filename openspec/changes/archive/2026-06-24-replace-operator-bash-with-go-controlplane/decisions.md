## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| Go owns full operator command surface | `scripts/vcpe`, `scripts/*`, `controlplane/cmd/vcpectl/main.go`, `controlplane/internal/app/app.go` | No |
| Bash compatibility lasts one release window after parity | `scripts/vcpe`, `scripts/bng`, `scripts/gateway`, `scripts/routerd`, `scripts/webpa`, `scripts/xb10`, `scripts/client`, `scripts/net` | No |
| Primary binary is `vcpe` with `vcpectl` alias/debug path | packaging scripts, Homebrew formula inputs, `controlplane/cmd/vcpectl/main.go`, top-level docs | No |
| Typed podman-compose adapter replaces service-script compose ownership | `services/*/scripts/*`, `controlplane/internal/backend/podman`, `controlplane/internal/app` | No |
| Go-owned image lifecycle | `services/*/scripts/*`, `controlplane/internal/backend/podman`, service image definitions | No |
| Catalog dependency graph controls service order | `scripts/vcpe`, control-plane planner/catalog packages | No |
| Generated artifacts move to versioned control-plane state paths | service runtime artifact paths, profile compatibility snapshots, control-plane state paths | No |
| Break only undocumented/internal bash behavior | script internals, undocumented env side effects, non-JSON human output formatting | No |

---

## Decisions

### Decision: Go owns full operator command surface
[BREAKING]
Recommendation: Go should own the full operator surface: `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `profile`, `plan`, `apply`, and `destroy`, with scripts as shims.
Decision: Proceed with full Go ownership.
Rationale: These commands mutate or inspect deployment state and must share validation, locking, profile translation, observability, and rollback behavior. Splitting command ownership between bash and Go would preserve the current drift problem.

Q: Should Go own the full operator command surface, or only deployment lifecycle commands first?
A: Full Go ownership.

---

### Decision: Script compatibility window
[BREAKING]
Recommendation: Preserve all current script paths for one full release window after the Go operator reaches parity, then keep only top-level convenience wrappers.
Decision: Preserve script compatibility for one release window after parity.
Rationale: Existing users and smoke tests depend on script paths, but keeping full script compatibility indefinitely would invite behavior drift.

Q: How long should legacy script compatibility be preserved?
A: One release window after parity.

---

### Decision: Primary binary name
[BREAKING]
Recommendation: Install the Go binary as `vcpe` for end users and keep `vcpectl` as an internal/developer alias during migration.
Decision: Use `vcpe` as the primary packaged command and keep `vcpectl` as an alias/debug path.
Rationale: `vcpe` matches the operator workflow and avoids exposing the control-plane implementation detail as the main UX. `vcpectl` remains useful for direct debugging and migration.

Q: What should the primary Go operator binary be named?
A: `vcpe` primary, `vcpectl` alias.

---

### Decision: Compose strategy
[BREAKING]
Recommendation: Use a typed Compose Adapter that invokes `podman-compose` initially, while Go owns generated inputs, project naming, plan records, and rollback tracking.
Decision: Use a typed `podman-compose` adapter first.
Rationale: Fully parsing and reconciling compose YAML immediately is higher risk and not required to remove operator bash ownership. Delegating to service scripts would keep orchestration opaque.

Q: Should Go parse/reconcile compose files or shell out to podman-compose initially?
A: Typed podman-compose adapter first.

---

### Decision: Renderer migration strategy
Recommendation: Add typed Go renderer interfaces and migrate service renderers incrementally. Temporary Python/shell renderer adapters are allowed only behind those interfaces, with structured inputs/outputs and tests.
Decision: Use incremental typed renderer adapters.
Rationale: A big-bang renderer rewrite would tie operator migration to full service-template parity. Typed adapters let Go own the render contract immediately while replacing implementations safely.

Q: How should legacy renderers be migrated into Go?
A: Initially selected immediate all-Go renderer rewrite, then accepted incremental typed adapters after challenge.

---

### Decision: Image lifecycle ownership
[BREAKING]
Recommendation: Put image lifecycle in Go with a typed `ServiceImage` model and Podman backend methods.
Decision: Go owns build, pull, push, tag, and pull-policy decisions.
Rationale: Image state affects apply correctness. The control plane must know whether it is using local builds, registry images, or cached content before creating containers.

Q: Where should image build, pull, push, and pull-policy decisions live?
A: Go-owned image lifecycle.

---

### Decision: Auto-build policy for `up`
Recommendation: `vcpe up` may build missing images only when the service image policy is `build-if-missing` or a local-development equivalent.
Decision: `up` builds missing images only when image policy allows it.
Rationale: Local development should be ergonomic without making every `up` rebuild everything or surprising registry-oriented profiles.

Q: Should `vcpe up` automatically build missing local images?
A: Build missing images by policy.

---

### Decision: Service ordering model
[BREAKING]
Recommendation: Derive order from a typed service dependency graph in the Service Catalog, with optional manifest overrides for advanced cases.
Decision: Use a catalog dependency graph with optional overrides.
Rationale: Service order is deployment logic and belongs in planner output, not command handlers. This makes dry-runs, rollback, and dependency tests truthful.

Q: How should Go determine service start and rollback order?
A: Catalog dependency graph with optional overrides.

---

### Decision: Privileged host networking model
Recommendation: Go preflights required capabilities and fails before mutation if unavailable. It may invoke a narrow host-network helper/adapter for bridge and firewall operations, but should not prompt for sudo mid-apply.
Decision: Preflight host networking privileges and fail before mutation when required capabilities are unavailable.
Rationale: Mid-apply sudo prompts create partial-state risk. Privileged operations must be explicit, testable, and isolated behind an adapter.

Q: How should privileged host networking operations be handled?
A: Preflight and fail before mutation.

---

### Decision: Legacy profile compatibility guarantee
Recommendation: Supported `.env` fields must round-trip exactly; unsupported fields must produce structured warnings and be preserved in ignored/metadata form where practical.
Decision: Exact supported-field round-trip plus warnings and best-effort unsupported-field preservation.
Rationale: Existing profiles are a user contract. The migration should be reversible for supported fields and explicit about unsupported fields.

Q: What compatibility guarantee should legacy `.env` profile import/export provide?
A: Exact supported round-trip plus warnings.

---

### Decision: Status and logs output contract
Recommendation: Human-readable output by default plus `--json` for automation. `status` should compare desired vs observed state; `logs` should support service/customer filters and include operation journal context separately from container logs.
Decision: Human default plus JSON automation output.
Rationale: Developers need readable CLI output, while tests and automation need stable machine-readable contracts.

Q: What output contract should Go `status` and `logs` provide?
A: Human default plus JSON automation.

---

### Decision: Daemon/watch scope
Recommendation: Defer daemon/watch reconciliation as a follow-up and make CLI parity authoritative first.
Decision: Defer daemon/watch workflows for this migration.
Rationale: Background reconciliation adds process supervision, socket lifecycle, and concurrency complexity that is not required to replace operator scripts.

Q: Should daemon/watch reconciliation be in scope for this bash-to-Go migration?
A: Defer daemon/watch.

---

### Decision: Required typed model scope
Recommendation: Add `ServiceCatalog`, `ServiceImage`, `ServiceDependency`, `RenderContract`, `ComposeGroup`, `HostNetworkIntent`, and `ProfileTranslationWarning` models as the minimum needed to remove bash ownership.
Decision: Add the full minimum model set.
Rationale: These models make service metadata, image policy, dependency order, render behavior, compose grouping, host networking, and profile warnings explicit rather than hidden in ad hoc env maps or scripts.

Q: Which new typed models are required in this change?
A: Add the full minimum model set.

---

### Decision: Generated artifact storage
[BREAKING]
Recommendation: Keep user profiles under existing config paths, but store generated manifests, render outputs, operation journals, and profile snapshots under the control-plane state root with versioned directories.
Decision: Use state-root versioned directories for generated artifacts and snapshots.
Rationale: Generated files should not contaminate source templates or unversioned service runtime directories. Versioned state paths support recovery, debugging, and compatibility export.

Q: Where should generated manifests, rendered files, and compatibility snapshots be stored?
A: State-root versioned directories.

---

### Decision: Disruptive change handling
Recommendation: Detect disruptive changes during planning and require `--allow-disruptive` or explicit interactive confirmation before mutation.
Decision: Require explicit disruptive approval.
Rationale: CIDR changes, identity reset, volume remap, and scale-to-zero can destroy or invalidate local runtime state. Non-interactive mode must fail with a plan summary rather than mutate unexpectedly.

Q: How should Go handle disruptive changes such as CIDR changes, identity reset, volume remap, or scale-to-zero?
A: Require explicit disruptive approval.

---

### Decision: Test gate
Recommendation: Require Go unit tests for translators/planner/catalog/adapters, golden tests for profile import/export and render inputs, command-contract tests for script shims, and Podman smoke tests for representative `bng-7` and `bng-20` flows when Podman is available.
Decision: Use the full focused gate.
Rationale: This migration touches command behavior, stateful orchestration, generated files, compatibility, and runtime Podman flows. A single test style will miss important regressions.

Q: What tests should gate implementation of this migration?
A: Full focused gate.

---

### Decision: Breaking-change boundary
[BREAKING]
Recommendation: Preserve documented command paths and supported profile keys, but allow breaking changes for undocumented env side effects, direct sourcing assumptions, non-JSON human output formatting, and service-script internals.
Decision: Break only undocumented/internal behavior.
Rationale: User workflows should stay stable while implementation internals are allowed to become typed and stateful.

Q: Which legacy behaviors may intentionally break in the Go migration?
A: Break only undocumented/internal behavior.

---

### Decision: Implementation slicing
Recommendation: Phase implementation, but require the first implementation to add real Go public commands for `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `profile`, `plan`, `apply`, and `destroy`; bash wrappers may translate but must not own behavior.
Decision: Phased parity with Go public commands first.
Rationale: Full internal parity in one pass is too large, but allowing bash to keep public command ownership would contradict the architecture. The first slice must establish Go as the visible owner.

Q: For this change, should implementation deliver full Go command parity in one pass or phase it?
A: Phased parity with Go public commands first.

---

### Decision: New model implementation depth
Recommendation: Implement functional minimum behavior for each required new model; avoid definition-only structs that do not drive real command or planner behavior.
Decision: Functional minimum.
Rationale: Empty models create architectural theater and do not reduce bash ownership. Each model must support at least one real command path, planner path, or compatibility path in the first implementation.

Q: Should the new catalog/image/render/network models be definition-only or functional in the first implementation?
A: Functional minimum.

---

### Decision: First-pass renderer depth
Recommendation: Implement real typed renderer adapters for smoke-gated `bng-7` and `bng-20` paths and allow explicit adapter-backed behavior or clear unsupported plan errors for non-migrated services.
Decision: Real adapters for smoke-gated paths.
Rationale: Keeping placeholder rendering would create fake parity, while rewriting every service renderer immediately would over-expand the first implementation slice.

Q: What rendering depth is required in the first implementation slice?
A: Real adapters for smoke-gated paths.

---

## Deferred Follow-Up Items (8.7 Coverage Review)

- Daemon/watch reconciliation: keep out of this change and track as a separate
	control-plane follow-up once CLI parity hardens and lock/state semantics are
	stable across longer-running sessions.
- Native compose reconciliation: current typed adapter delegates to
	`podman-compose`; full native compose state diff/reconcile remains deferred to
	a dedicated architecture change.
- Full renderer rewrites: only smoke-gated BNG renderers are rewritten. Service
	renderers outside the smoke-gated BNG paths remain explicit follow-up work and
	must keep returning clear unsupported-plan errors until migrated.
