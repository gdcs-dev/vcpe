## Context

This change brings the Go control plane into compliance with its own ratified specs by making the v1 manifest the sole source of instance data. The full architectural rationale, locked schema, and decision log already exist:

- Architecture: [architecture.md](architecture.md)
- Locked decisions (with `[BREAKING]` markers): [decisions.md](decisions.md)
- Motivation and capability map: [proposal.md](proposal.md)

This document covers only implementation-level concerns those artifacts do not: package layout, refactor sequencing, the renderer/compose wiring, and the test/migration mechanics. The orchestration engine (operation journal, phased apply, bounded reverse-order rollback, IPAM, compose/podman adapters, host-network controller, daemon) is preserved; work concentrates in the policy layer.

## Goals / Non-Goals

**Goals:**
- Replace `catalog/builtin.go` with a behavior-only `typeregistry` and route all instance data through the manifest.
- Make the schema (`manifest`), validation, planner, IPAM, renderers, persist, and CLI consistent with the v1 envelope and the 21 locked decisions.
- Keep the change mechanically reviewable: stable canonical hash key, deterministic golden render output, schema-version state cutover.

**Non-Goals:**
- No changes to journal/rollback/compose/podman adapter semantics beyond renaming inputs.
- No `routerd`/`xb10` service types in v1 (added later via the registry without schema changes).
- No legacy state migration; no backward compatibility with profiles/customer files.
- No new network-exposed surface; CLI/daemon stay local.

## Decisions

> Schema shape, identity model, IPAM authority, MAC hash key, registry contract, compose sourcing, and CLI/state cutover are all locked in [decisions.md](decisions.md). The items below are implementation choices not covered there.

### Package layout
- `internal/typeregistry` — replaces `internal/catalog`. Exposes `Register(ServiceType)`, `Lookup(type) (ServiceType, bool)`, and `Registered() []string`. `ServiceType` interface: `Type() string`, `ValidateConfig(yaml.Node) error`, `Renderer() Renderer`, `ExpectedRoles() []RoleRequirement`, `DefaultImagePolicy() string`. Registration is explicit at package init of each type package (no scattered blank imports beyond a single `register.go` aggregator).
- `internal/types/{bng,gateway,webpa,genericcontainer}` — one package per service type, each owning its typed config struct, validator, renderer, and `ExpectedRoles`. Each calls `typeregistry.Register(...)`.
- `internal/render` — keeps the engine; the per-type renderers move into the type packages and implement the shared `Renderer` interface. `bng_renderer.go` literals are deleted.
- `internal/manifest` — v1 envelope structs (`Document{APIVersion,Kind,Metadata,Spec}`), strict top-level decode plus deferred `yaml.Node` capture for `service.config`.

### Config decoding
`service.config` is captured as a `yaml.Node` during top-level decode, then handed to the registered `ValidateConfig`, which performs a strict (`KnownFields(true)`) decode into the type's struct. This keeps the top-level schema small and pushes type-specific validation into the owning package — the registry is the single definition of "supported type".

### Renderer → compose wiring
- A single `ifaceEnv` producer builds the canonical `IFACE_<ROLE>_*` map (plus `DEPLOYMENT_NAME`/`SERVICE_NAME`/`IMAGE`) from the planned interfaces + IPAM result, shared by all curated types. Role normalization: upper-case, `-`→`_`.
- Curated types (`bng`/`gateway`/`webpa`) return a compose-file path (from the registry/type package) + the generated `compose.env`. `generic-container` returns a fully generated compose document. The compose adapter consumes both uniformly.
- Curated `services/{bng,gateway,webpa}/compose.yaml` are edited to read the canonical env vars.

### Naming and identity helpers
- One `deriveBridgeName(metaName, role)` helper applies the `<name>-<role>` default and the 15-char IFNAMSIZ truncation+hash-suffix rule; used by planner and host-network intent build so names never diverge.
- One `canonicalMAC(metaName, service, role, index)` helper (sha1 of the canonical key, first 5 bytes, `02:...` prefix) is the sole MAC source, called by both planner and `runtimeinit/contract`.

### Persist cutover
`persist` writes a `schemaVersion` record on the state root. A guard at apply/command entry compares it; a non-empty unstamped or mismatched root returns an actionable error. `vcpe state reset` clears the root and re-stamps. No automatic wipe.

### Refactor sequencing (keeps the tree compiling between steps)
1. Add v1 `manifest` structs + strict decode; keep old path behind it until callers move.
2. Introduce `typeregistry` + `ServiceType` and the four type packages (config + validator + `ExpectedRoles`), registered but not yet wired.
3. Move renderers into type packages behind the `Renderer` interface; add the `ifaceEnv` producer; delete `bng_renderer.go` literals.
4. Rewire planner to derive names/MACs from the manifest via the shared helpers; make IPAM the sole address authority.
5. Update validation (cross-object rules + per-type dispatch) and the apply preflight (`registry.Lookup`).
6. CLI `--name`, `maxActiveDeployments`, persist schema stamp + `state reset`.
7. Delete `internal/profile`, `config/profiles/*`, `services/*/customers/*`, profile-compat spec implementation.
8. Update curated compose files, smoke manifests, and tests.

## Risks / Trade-offs

- **Wide blast radius across the policy layer** → Sequence the refactor (above) so the tree compiles at each step; lean on golden render tests and planner-determinism tests to catch regressions early.
- **Canonical MAC/name helpers must agree across planner and runtime-init** → Extract single shared helpers and add a cross-component determinism test asserting identical output for the same key.
- **Curated compose files and the env contract can drift** → Add a render golden test per curated type that pins the exact `compose.env` output the compose file consumes.
- **Strict config decode may reject previously tolerated fields** → Intended (greenfield); table tests cover each failure mode so the errors are explicit and actionable.
- **State cutover refuses operation on legacy roots** → Acceptable; the error names `vcpe state reset` and there are no production deployments to preserve.

## Migration Plan

1. Land the refactor behind the sequencing above; `go build ./...` and `go test ./...` green at each merge point.
2. Convert smoke fixtures to v1 manifests (`apiVersion: vcpe.dev/v1`, `kind: Deployment`) and update `tests/smoke/*`.
3. On first run against an existing dev state root, operators run `vcpe state reset` (state is stamped thereafter).
4. Rollback: revert the change set; since legacy state was reset, redeploy from the prior binary requires a fresh `up` (greenfield, no state bridge).

## Open Questions

- None blocking. `routerd`/`xb10` type packages are deferred and register additively when authored; the registry contract is designed to absorb them without schema changes.
