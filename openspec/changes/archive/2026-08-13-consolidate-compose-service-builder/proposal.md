## Why

`consolidate-controlplane-reuse` deliberately kept Compose service-block construction and health-sidecar wiring local to each of the 7 built-in service types, to avoid a shared abstraction turning into an unbounded "policy matrix" (see that change's design.md Non-Goals: "Create one universal Compose service builder" / "Unify distinct health mechanisms"). That trade-off is now costing real, measurable duplication: `bng`, `gateway`, `webpa`, `xb10`, and `oktopus` each define their own `render*Compose`/`generateCompose` function, and `event-sink`/`generic-container` inline the same pattern — six-plus near-identical blocks that each construct `image`/`container_name`/`hostname`/`env_file`/`networks`/`ports` and then hand-roll one of exactly two known health-sidecar shapes (a proxy sidecar reachable by service name over the shared health network, and a probe sidecar sharing the workload's network namespace via `network_mode: service:<name>`).

An independently developed implementation of the same original consolidation goal (branch `sonnet-consolidate`) centralized exactly this scaffolding into the shared renderer template behind a small, bounded hook surface, and produced roughly 14% less code across the shared package plus all 7 type packages combined, with equivalent behavior on the paths it touched. This change adopts that direction deliberately, with explicit guardrails so the hook surface stays bounded rather than growing into the policy matrix the original non-goal was written to prevent.

## What Changes

- Add a shared Compose service-block builder to `internal/render/servicetemplate` that produces the `image`/`container_name`/`hostname`/`env_file`/`networks`/`ports` fields identically to what all 7 curated per-instance types already produce by hand.
- Add two shared health-sidecar builders — proxy-sidecar and probe-sidecar — matching the exact existing YAML shape (image, container_name, entrypoint/command, networks, ports, restart policy, `depends_on`) already locked down by each type's golden/characterization tests.
- Add exactly one bounded escape-hatch hook (a Compose-service mutator, e.g. `ComposeExtras`) for the small amount of Compose policy that legitimately differs per type: `privileged`, `cap_add`, `volumes`, `network_mode`. No new hook is added per Compose field; anything beyond this one mutator stays a service-owned artifact.
- Add a shared network-attachment hook (mac-only vs. mac+conditional-ipv4-pinning) so the two attachment policies already established by the registry defaults pattern are expressed once instead of re-implemented per type.
- Migrate all 7 built-in types off their bespoke `render*Compose`/`generateCompose`/inline construction onto the shared builder, one type at a time, each verified against its own existing golden/characterization tests plus the cross-type artifact-inventory and Compose-semantics tests before the next type migrates.
- Preserve every existing generated-artifact key, Compose YAML semantic content, health port behavior, network-attachment pinning rule, and manifest/CLI-facing behavior. This is a pure internal refactor.

## Capabilities

### Modified Capabilities
- `controlplane-code-reuse`: replaces the "Service policy remains local" / no-universal-Compose-builder scenario and the Consolidation-boundaries exclusion of service-specific Compose policy and health mechanisms with a bounded shared Compose-block and health-sidecar builder plus one explicit, capped escape hatch.

### New Capabilities
None. This narrows and extends the existing `controlplane-code-reuse` capability; it does not introduce a new one.

## Impact

- Primary code areas: `controlplane/internal/render/servicetemplate`, `controlplane/internal/types/{bng,gateway,webpa,xb10,oktopus,eventsink,genericcontainer}`.
- No change to manifest schema, CLI flags, persisted state format, or generated artifact layout.
- No new runtime dependency, external service, or persisted-state migration.
- Each per-type migration is independently testable and revertible; a regression in one type must not block or hide behind another type's migration.
