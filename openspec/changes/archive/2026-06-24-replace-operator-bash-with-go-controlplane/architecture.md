## Overview

This change replaces the current operator-owned bash orchestration with a Go operator built on the existing control plane. The target state is a single Go command surface that owns profile/config management, manifest translation, planning, image lifecycle, render, Podman lifecycle, status, logs, and teardown. Bash remains only as temporary compatibility shims and, where unavoidable, as a narrow subprocess boundary for host operations that require privileged system tooling during migration.

The correct architecture is not a line-for-line bash port. The bash scripts currently mix user interface, profile parsing, validation, rendering, sequencing, Podman invocation, network bootstrap, and compatibility behavior. Recreating that shape in Go would preserve the fragility under a different language. Instead, the Go operator should make these concerns explicit behind typed interfaces and drive all mutations through the control-plane plan/apply/destroy/status model.

The control plane remains local-first and Podman-only. There is no Kubernetes adapter, scheduler, or multi-node abstraction in this change.

## Components

- **Go Operator CLI (`vcpectl` / packaged `vcpe`)**: Primary user-facing command surface. Owns `init`, `build`, `up`, `down`, `status`, `logs`, `config`, `profile`, `plan`, `apply`, and `destroy` semantics.
- **Compatibility Shims (`scripts/*`)**: Thin wrappers that preserve existing command paths during migration. They should only translate legacy invocations into Go commands and emit deprecation warnings when appropriate.
- **Profile Store**: Typed Go implementation for default config, built-in profiles, user profiles, active-profile selection, and profile import/export from legacy `.env` files.
- **Manifest Translator**: Converts profile/config inputs into versioned control-plane manifests and can export supported manifest fields back to legacy profile snapshots during the compatibility window.
- **Planner**: Produces deterministic operation plans for networks, images, rendered artifacts, compose groups, containers, volumes, health checks, and rollback order.
- **Service Catalog**: Typed registry of known services (`bng`, `gateway`, `routerd`, `webpa`, `xb10`, `client`) with build contexts, image metadata, compose/render contracts, dependencies, health checks, and log selectors.
- **Image Manager**: Go-owned build/pull/push policy engine. Replaces service-script build/push behavior with typed Podman backend operations.
- **Render Engine**: Typed Go render pipeline for service artifacts and compose env files. Temporary Python or shell renderers may exist only behind explicit renderer interfaces with structured inputs and outputs.
- **Podman Backend Adapter**: Executes Podman and podman-compose-compatible operations through typed methods. It owns lifecycle reconciliation, status inspection, logs, and rollback bookkeeping.
- **Host Network Adapter**: Encapsulates bridge, NAT, firewall, and host capability checks. It is the only approved boundary for privileged host operations.
- **State Store and Lock Manager**: Existing SQLite-backed control-plane state and writer locking remain the single source of truth for desired state, observed state, operation journals, and recovery.
- **Observability Layer**: Structured event logs, human-readable summaries, JSON output modes, metrics counters, and redaction applied consistently across commands.

## Key Architectural Decisions

### Go operator as the primary command surface

**Choice**: The Go operator owns all operator semantics currently exposed through top-level scripts, including `build`, `up`, `down`, `status`, `logs`, `config`, and `profile`.

**Rationale**: These commands mutate or inspect deployment state and must share validation, locking, profile translation, observability, and rollback behavior. Keeping them in bash would preserve split-brain orchestration and make the control plane advisory instead of authoritative.

**Alternatives considered**: Keep bash as the main UX and call Go only for `apply` and `status`. Rejected because profile parsing, service sequencing, and build/pull decisions would still live outside the stateful control plane.

### Bash retained only as compatibility shims

**Choice**: Existing scripts remain during migration but become thin wrappers around Go commands.

**Rationale**: Existing users and smoke tests depend on paths such as `./scripts/vcpe`, `./scripts/bng`, and `./scripts/gateway`. Preserving those paths lowers migration risk while still eliminating bash as the behavior owner.

**Alternatives considered**: Delete scripts immediately. Rejected because it would create unnecessary breakage and make validation harder. Keep full bash implementations indefinitely. Rejected because it blocks the migration goal.

### Control plane as the only mutation authority

**Choice**: All deployment mutations flow through control-plane planning, locking, journaling, apply/destroy phases, and state persistence.

**Rationale**: The existing control-plane architecture already provides manifest validation, deterministic plans, SQLite state, writer locks, and rollback/recovery hooks. New Go operator behavior should strengthen that boundary instead of bypassing it.

**Alternatives considered**: Add independent Go commands that invoke Podman directly without state. Rejected because it would reproduce current script drift and make recovery nondeterministic.

### Typed service catalog instead of script discovery

**Choice**: The Go operator uses a typed service catalog for service metadata, dependency ordering, build contexts, image policies, render contracts, compose groups, health checks, and log selectors.

**Rationale**: Current behavior is implicit in service scripts and environment variables. A catalog makes sequencing and compatibility auditable and testable while still allowing per-service differences.

**Alternatives considered**: Discover behavior by shelling out to service scripts. Rejected because it keeps the most important orchestration rules opaque.

### Explicit service sequencing in planner output

**Choice**: Service startup, health verification, teardown, and rollback ordering are planner outputs, derived from catalog dependencies and manifest topology.

**Rationale**: Today `bng -> webpa -> gateway -> xb10 -> routerd -> client` ordering is embedded in bash. Moving order into plans makes dry-runs truthful, rollback reversible, and future customer profiles predictable.

**Alternatives considered**: Preserve hardcoded order in CLI command handlers. Rejected because ordering is deployment logic, not UI logic.

### Image lifecycle owned by Go

**Choice**: Build, pull, push, tag, and pull-policy decisions move into the Go image manager and Podman backend.

**Rationale**: Image state affects apply correctness. The operator must know whether it is using a local build, a registry image, or stale cached content before creating containers.

**Alternatives considered**: Continue invoking service `build`/`push` scripts. Rejected because the control plane cannot reason about image provenance or failure recovery through opaque subprocesses.

### Incremental renderer replacement behind interfaces

**Choice**: Go renderers are the target. Existing Python or shell rendering can remain temporarily only behind typed renderer interfaces with structured validation and explicit deprecation.

**Rationale**: Rewriting every renderer at once increases risk. A renderer interface allows staged migration while preventing subprocess renderers from leaking untyped env-file behavior into the rest of the operator.

**Alternatives considered**: Rewrite all rendering immediately. Rejected as too much blast radius for the architecture slice. Keep Python rendering as permanent. Rejected because it preserves a second configuration model.

### Privileged host operations isolated behind adapters

**Choice**: Bridge, firewall, NAT, and host capability operations are isolated in a Host Network Adapter with preflight checks and explicit failure modes.

**Rationale**: These operations are platform-sensitive and may require sudo or rootful Podman. The operator should know before apply whether required capabilities are available and should avoid scattering privileged subprocess calls through command handlers.

**Alternatives considered**: Shell out directly wherever needed. Rejected because it makes privilege behavior inconsistent and hard to test.

### Compatibility window with structured warnings

**Choice**: Legacy `.env` profiles remain importable/exportable for a defined migration window. Unsupported keys produce structured warnings and are not silently dropped.

**Rationale**: Existing profiles are a user contract. The migration should be reversible for supported fields and explicit about unsupported fields.

**Alternatives considered**: Convert profiles once and remove export. Rejected because it makes rollback to legacy script mode harder during migration.

## Data Flow

Legacy-compatible invocation:

```text
User / existing smoke test
        |
        v
scripts/vcpe or scripts/<service>
        |
        | translate args only
        v
Go operator CLI
        |
        v
Profile Store -----> Manifest Translator -----> Versioned Manifest
        |                                            |
        |                                            v
        |                                     Validator / Preflight
        |                                            |
        v                                            v
State Store <------------------------------- Planner
        |                                            |
        | operation journal                           v
        |                                  Apply / Destroy Phases
        |                                            |
        |              +-----------------------------+------------------+
        |              |                             |                  |
        v              v                             v                  v
Observed State   Host Network Adapter        Image Manager        Render Engine
        |              |                             |                  |
        |              v                             v                  v
        |         bridges/NAT/firewall          Podman images      compose/env/config
        |                                            |
        +--------------------------------------------v
                                           Podman Backend Adapter
                                                    |
                                                    v
                                           containers / networks / logs
```

Apply phase flow:

```text
parse command
  -> load active profile or manifest
  -> translate legacy profile if needed
  -> validate schema and service catalog references
  -> acquire writer lock
  -> compute plan
  -> preflight host capabilities and image policy
  -> create/update networks
  -> build/pull required images
  -> render service artifacts
  -> apply compose/container groups in dependency order
  -> run health checks
  -> persist observed status
  -> release lock
```

Rollback flow:

```text
phase failure
  -> classify retryable vs terminal
  -> stop creating new resources
  -> read operation journal
  -> undo current-operation resources in reverse plan order
  -> persist failed operation status
  -> emit structured remediation guidance
```

## Integration Points

- **Existing Go control plane**: Extends `controlplane/internal/app`, `manifest`, `planner`, `persist`, `backend/podman`, `render`, `observability`, and state packages.
- **Existing scripts**: `scripts/vcpe` and service shims route to Go during the compatibility window.
- **Config files**: Built-in profile files under `config/` remain supported inputs but are parsed by Go instead of sourced by bash.
- **Service definitions**: Compose files, Containerfiles, templates, customer fixtures, and runtime directories under `services/` remain the backing assets for initial migration.
- **Podman**: The only container backend. Invoked through typed backend methods, not arbitrary command construction in command handlers.
- **Host networking**: Existing bridge/firewall behavior is preserved through the Host Network Adapter, then progressively replaced with native Go implementations where practical.
- **Smoke tests**: Existing script paths continue to work, but expected behavior is provided by Go.
- **OpenSpec specs**: Existing control-plane, profile compatibility, rendering, IPAM, observability, and developer workflow specs constrain this design.

## Security Model

- Local user trust boundary remains unchanged: the operator runs on a developer machine and controls local Podman resources.
- Privileged host networking operations require explicit preflight detection. If elevation is required and unavailable, apply fails before partial mutation.
- Secrets are accepted only through existing local providers (`env` and `file`) unless a later change expands providers.
- Secret values are resolved at apply time, never persisted to SQLite, logs, rendered state summaries, or profile exports.
- Logs and status output use the existing redaction layer before emitting human or JSON output.
- Compatibility import/export must warn on unsupported sensitive fields and avoid writing resolved secret values.
- Subprocess adapters receive structured arguments and context timeouts. They must not construct shell strings from untrusted input.

## Error Handling Strategy

- Errors are classified as validation, preflight, planning, render, image, host-network, Podman lifecycle, health, state, or internal errors.
- Validation and preflight errors happen before mutation and return actionable messages with no rollback required.
- Apply-phase errors are journaled with the phase, resource identity, command category, and rollback eligibility.
- Retry is allowed only for bounded transient operations such as Podman inspection, IPv6 readiness, container health checks, and image pull network failures.
- Destructive or disruptive changes require explicit approval through `--allow-disruptive` or an interactive confirmation path where supported.
- Rollback is scoped to resources created or modified by the current operation. Orphan cleanup is a separate explicit command, not automatic hidden behavior.
- Compatibility shims propagate the Go command exit code exactly so existing automation can fail predictably.

## Observability Strategy

- Every command emits a concise human summary by default and supports JSON output for automation.
- Operation events include command, profile, manifest version, customer, service, phase, resource type, duration, result, and rollback status.
- Status compares desired state, planned state, and observed Podman state per customer and service.
- Logs command combines operation journal context with Podman logs selected through the service catalog.
- Metrics counters should cover operations started/completed/failed, phase durations, rollback counts, image operations, render failures, health failures, and drift detections.
- Trace boundaries align with major phases: profile translation, planning, preflight, image lifecycle, render, network, lifecycle, health, and cleanup.
- Redaction is applied before logs, events, JSON output, and persisted diagnostic snapshots.

## Constraints

- Podman remains the only backend for this change.
- The project remains local-development focused; no Kubernetes scheduler or multi-node model is introduced.
- Existing service containers and compose assets are preserved initially.
- Existing script command paths must continue to work during the compatibility window.
- The control plane remains the only mutation authority for deployment lifecycle operations.
- Dynamic topology and IPAM behavior from the previous control-plane architecture must not regress to hardcoded customer IDs.
- Default resource limits remain bounded for local machines.
- Profile import/export must preserve supported fields and warn on unsupported fields.
- Host networking operations must be explicit, preflighted, and isolated behind adapters.
- The migration must be testable without requiring every service renderer to be rewritten in the first implementation slice.

## Diagrams

Component ownership:

```text
+--------------------+        +-----------------------+
| compatibility      |        | direct users / CI     |
| scripts/*          |        | make targets          |
+---------+----------+        +-----------+-----------+
          |                               |
          +---------------+---------------+
                          |
                          v
                 +-----------------+
                 | Go Operator CLI |
                 +--------+--------+
                          |
          +---------------+----------------+----------------+
          |                                |                |
          v                                v                v
+-------------------+          +-------------------+  +----------------+
| Profile/Manifest  |          | Planner/State     |  | Observability  |
| Translation       |          | Lock/Journal      |  | Logs/Status    |
+---------+---------+          +---------+---------+  +----------------+
          |                              |
          v                              v
+-------------------+          +-------------------+
| Service Catalog   |          | Apply Engine      |
+---------+---------+          +----+---------+----+
          |                         |         |
          v                         v         v
+-------------------+    +----------------+ +----------------+
| Render Engine     |    | Podman Backend | | Host Network   |
| Go + adapters     |    | Adapter        | | Adapter        |
+-------------------+    +----------------+ +----------------+
```

Command migration matrix:

```text
Current command path          Target owner                 Migration shape
--------------------------    -------------------------    -----------------------------
./scripts/vcpe init           Go profile store             shim -> go init
./scripts/vcpe build          Go image manager             shim -> go build
./scripts/vcpe up             Go apply composite           shim -> go up/apply
./scripts/vcpe down           Go destroy composite         shim -> go down/destroy
./scripts/vcpe status         Go status                    shim -> go status
./scripts/vcpe logs           Go logs                      shim -> go logs
./scripts/vcpe config         Go config commands           shim -> go config
./scripts/vcpe profile        Go profile commands          shim -> go profile
./scripts/<service> up/down   Go service-scoped lifecycle  shim -> go service ...
./scripts/net setup/verify    Go host network adapter      shim -> go network ...
service render scripts        Go render engine             temporary typed adapter allowed
```

Service dependency example:

```text
networks
  |
  v
bng
  |
  +----> webpa
  |
  +----> gateway
            |
            +----> xb10
            |
            +----> routerd
            |
            +----> client peers
```

Rollback order is the reverse of the planner's successfully applied dependency order.
