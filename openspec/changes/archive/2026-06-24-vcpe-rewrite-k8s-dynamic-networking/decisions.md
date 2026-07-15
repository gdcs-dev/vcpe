## BREAKING CHANGES

| Decision | Affects | Override? |
|----------|---------|-----------|
| Data model split (profile + deployment + status) | scripts/vcpe, config/profiles/*.env, services/*/scripts/*, platform/scripts/lib/common.sh | No |
| Declarative CLI contract (plan/apply/status/destroy) | scripts/vcpe, scripts/bng, scripts/gateway, scripts/routerd, scripts/webpa, scripts/xb10, scripts/client | No |
| SQLite state store and crash recovery | platform/scripts/lib/common.sh, state management paths under services/*/runtime/*, new control-plane storage layer | No |
| Fail-fast with bounded rollback | scripts/vcpe, service lifecycle scripts under services/*/scripts/* | No |
| Bounded local scaling limits | config/profiles/*.env, new manifest schema, service orchestration logic | No |
| Disruptive change gating (`--allow-disruptive`) | plan/apply workflow in new control-plane CLI and compatibility wrappers in scripts/vcpe | No |

---

## Decisions

### Decision: Desired state data model
[BREAKING]
Recommendation: Use one versioned manifest per customer deployment with separate Profile, Customer Deployment, and Runtime Status objects.
Decision: Accepted recommendation.
Rationale: Prevents config drift and separates reusable defaults from desired intent and observed state.

Q: Which object should be the single source of truth for desired state?
A: Option 1 (recommended model).

---

### Decision: CLI API contract
[BREAKING]
Recommendation: Adopt an idempotent declarative CLI with `apply`, `plan`, `status`, and `destroy`.
Decision: Accepted recommendation.
Rationale: Enables predictable local behavior, dry-run planning, and safer convergence.

Q: How should users interact with the control plane for local workflows?
A: Option 1 (recommended contract).

---

### Decision: Local authorization model
Recommendation: Enforce OS-user-level authorization with optional project-scoped `policy.yaml`; no full RBAC in this phase.
Decision: Accepted recommendation.
Rationale: Matches local dev/test scope while still guarding dangerous operations.

Q: What access model should be enforced for local control-plane operations?
A: Option 1 (recommended model).

---

### Decision: Durable state persistence
[BREAKING]
Recommendation: Use SQLite with transactional writes, single-writer locking, and startup crash recovery replay.
Decision: Accepted recommendation.
Rationale: Provides durability and recovery without external database complexity.

Q: How should reconcile and allocation state be persisted locally?
A: Option 1 (recommended state model).

---

### Decision: Apply failure behavior
[BREAKING]
Recommendation: Fail fast on terminal phase error, perform bounded rollback for resources created in the operation, preserve forensics, and require explicit retry.
Decision: Accepted recommendation.
Rationale: Avoids hidden drift and keeps debugging deterministic.

Q: What should be the default behavior when one phase in apply fails?
A: Option 1 (recommended behavior).

---

### Decision: Test gate policy
Recommendation: Require unit + Podman integration + smoke tests with all three gates passing.
Decision: Accepted recommendation.
Rationale: Networking/orchestration risk requires integration coverage beyond unit tests.

Q: What minimum test pyramid should be required before a feature is considered done?
A: Option 1 (recommended testing policy).

---

### Decision: Secret handling model
Recommendation: Keep secret references only in manifests/state, resolve from external source, and redact logs by default.
Decision: Accepted recommendation.
Rationale: Prevents secret leakage in repository, state database, and logs.

Q: How should secrets and sensitive config be handled in local state and manifests?
A: Option 1 (recommended model).

---

### Decision: Local scaling and resource limits
[BREAKING]
Recommendation: Enforce conservative defaults (max replicas per customer-service 3, max active customers 10), service resource caps, and reconcile concurrency 1 by default.
Decision: Accepted recommendation.
Rationale: Protects developer machines while preserving scale behavior testing.

Q: What default scaling and resource policy should be enforced for local Podman runs?
A: Option 1 (recommended policy).

---

### Decision: Observability baseline
Recommendation: Require structured JSON logs, core metrics, drift timeline in status output, and optional tracing.
Decision: Accepted recommendation.
Rationale: Sufficient local diagnosability without heavyweight stack requirements.

Q: What minimum observability contract should be mandatory for every apply/plan/status operation?
A: Option 1 (recommended observability contract).

---

### Decision: Control-plane runtime mode
Recommendation: CLI-first binary with optional local daemon mode, both sharing the same state and locking rules.
Decision: Accepted recommendation.
Rationale: Keeps onboarding simple while enabling continuous reconciliation when needed.

Q: How should the control plane itself run on developer machines?
A: Option 1 (recommended deployment model).

---

### Decision: Disruptive network/topology change policy
[BREAKING]
Recommendation: Mark disruptive plans explicitly, require `--allow-disruptive`, perform ordered replacement with health checks, and rollback on failure.
Decision: Accepted recommendation.
Rationale: Prevents accidental outages from unsafe in-place remapping.

Q: What should happen when network intent changes require disruptive remapping?
A: Option 1 (recommended disruptive-change policy).

---

### Decision: Reconcile execution authority
Recommendation: Use single execution authority: in CLI mode, command process mutates state; in daemon mode, daemon owns all mutating operations.
Decision: Accepted recommendation.
Rationale: Avoids split-brain writes and simplifies recovery semantics.

Q: Where should the reconciliation loop run when daemon mode is enabled?
A: Option 1 (recommended model).

---

### Decision: Phase-1 secret providers and contract
Recommendation: Support `env` and local `file` providers only in phase 1, require explicit `secretRef`, fail on missing secrets, and never persist secret values.
Decision: Accepted recommendation.
Rationale: Minimal, testable provider surface suitable for local workflows.

Q: What secret source(s) should be supported in phase 1 for local development?
A: Option 1 (recommended providers and behavior).

---

### Decision: Disruptive-change classification rules
Recommendation: Treat CIDR changes, interface-role reassignment, service identity resets, volume remaps, and scale-to-zero of required services as disruptive.
Decision: Accepted recommendation.
Rationale: Establishes deterministic safety boundaries for apply gating.

Q: Which exact changes must be classified as disruptive by plan and require `--allow-disruptive`?
A: Option 1 (recommended disruptive classification).

---
