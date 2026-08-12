## Context

The current control plane has sound top-level ownership: manifests describe desired state, planning resolves identities and addresses, registered service types validate and render workload-specific artifacts, and application orchestration reconciles those artifacts through image, Compose, Podman, host-network, and persistence components.

Duplication appears inside those boundaries rather than between them. Seven built-in types repeat service metadata methods. Their renderers repeat config decoding, no-instance checks, replica loops, environment sorting, artifact path construction, and Compose aggregation. Docker and Podman repeat image request structures, after which the application translates canonical requests into identical adapter-local structures. The wizard maintains a partial image-default switch despite the registry already owning that metadata. Small image and network helpers are duplicated with either identical or subtly different semantics.

The architecture document defines the target ownership boundaries. This design focuses on concrete contracts, migration sequence, and verification. The decisions document records that the current tree and ratified specs are the sole source of truth for this proposal.

## Goals / Non-Goals

**Goals:**

- Remove repeated production mechanics when a stable shared contract exists.
- Make service registry metadata the only service catalog used by control-plane consumers.
- Preserve generated artifact paths and semantic content across renderer migrations.
- Preserve backend-specific image behavior while eliminating request translation.
- Resolve the inconsistent empty-repository image reference behavior explicitly.
- Improve test coverage for cross-type invariants and under-tested renderer exceptions.
- Keep each migration small enough for a focused test to identify regressions.

**Non-Goals:**

- Change manifest fields, defaults, validation rules, or CLI behavior.
- Correct unrelated service behavior during consolidation, even when tests reveal a specification mismatch; record such mismatches for a separate change.
- Create one universal Compose service builder.
- Unify distinct health mechanisms, backend command builders, persistence queries, or wizard workflows.
- Change state schema, generated-state paths, operation journal format, or reconciliation phases.
- Introduce third-party libraries, reflection, generated code, or runtime plugin loading.
- Optimize performance beyond avoiding repeated config decode and unnecessary translation allocations.

## Decisions

### Shared renderer API shape

Add `internal/render/servicetemplate` as a child package of rendering. Its public surface is intentionally small:

```go
type Mode uint8

const (
    PerInstance Mode = iota
    Interpolated
)

type Hooks[C any] struct {
    Name           string
    Mode           Mode
    DecodeConfig   func(yaml.Node) (C, error)
    RenderInstance func(context.Context, render.Input, C) (render.Result, error)
}

func New[C any](hooks Hooks[C]) render.Renderer
```

`PerInstance` passes a shallow copy of `render.Input` containing one `plan.Instance` to the service hook. The hook returns service-owned artifacts, including one `compose.yaml` fragment. The template merges fragments and lays out artifacts.

`Interpolated` decodes once and invokes the hook once with the complete input. It normalizes renderer identity but otherwise preserves the service result. This mode exists for generic-container and must not manufacture resolved instances or reinterpret `${...}` values.

The template does not expose hooks for privilege, health, volumes, ports, or network attachment maps. Those remain ordinary service package functions, preventing the shared API from turning into a policy matrix.

### Config decoding and validation

Each service supplies a typed decoder function. The decoder owns strict decode, optional-config handling, service-specific validation, and contextual error wording. The shared lifecycle calls it once per service render rather than once per replica.

Preflight remains authoritative and still calls `ServiceType.ValidateConfig`. Rendering repeats decode defensively because renderers may be invoked directly by tests or future tools. The proposal does not merge preflight validation with rendering.

### Artifact aggregation algorithm

For `PerInstance` mode:

1. Reject an empty renderer name, nil decoder, nil instance hook, unsupported mode, or no instances.
2. Decode config exactly once.
3. Iterate resolved instances in their existing plan order.
4. Invoke the service hook with exactly one instance.
5. Require exactly one `compose.yaml` artifact from each invocation.
6. Parse only the top-level `services` and `networks` mappings through `yaml.v3` structured decoding.
7. Merge distinct keys. For duplicate keys, compare normalized decoded values: accept equality and reject conflicts.
8. For `compose.env`, mirror instance index zero to root and always write `instances/<index+1>/compose.env`.
9. For other non-Compose artifacts, mirror instance index zero to root and always write `instances/<index+1>/<key>`.
10. Reject empty artifact keys, absolute paths, path traversal, duplicate output keys with conflicting content, missing Compose fragments, and multiple Compose fragments from one hook.
11. Marshal one final Compose document and append it as root `compose.yaml`.
12. Return the declared renderer name.

Artifact path validation belongs in the template because it constructs and combines paths. This hardening prevents a hook from escaping the service artifact root without changing valid current outputs.

For `Interpolated` mode, the hook result is validated for non-empty, relative artifact keys and duplicate conflicting keys, but no per-instance mirroring or Compose merge is applied.

### Service migration order

Migrate from the simplest policies to the richest:

1. WebPA and event-sink: simple config/env and direct health publication.
2. Oktopus: simple env with volume and attachment behavior.
3. Gateway: per-instance aggregation plus health-sidecar and generated artifacts.
4. BNG: richest auxiliary artifact set and network-derived configuration.
5. XB10: explicit addressing exemption and currently missing package-local tests.
6. Generic-container: interpolated mode, optional health, embedded entrypoint, and different replica semantics.

Migration order is a risk-control mechanism, not a required runtime ordering. After each migration, run template tests, that package's tests, and cross-type invariant tests before proceeding.

### Service metadata defaults

Add a zero-value `typeregistry.BaseServiceType` implementing:

- curated health behavior on port `9878`;
- default image policy `build`;
- no-op interface validation.

Built-in concrete service types embed the base. Generic-container keeps its explicit `Health` override. Any future type with interface validation or a different image policy overrides the corresponding method normally. The registry interface itself remains unchanged.

Tests must assert every built-in type's effective metadata, not merely that methods return non-empty values. This catches accidental changes hidden by embedding.

### Registry-backed wizard metadata

Replace `wizard.defaultRepo`'s type switch with a helper that performs `typeregistry.Lookup` and returns `DefaultImage`. Registration remains performed by the existing application composition root; wizard unit tests register the built-ins they exercise or use a test registration strategy compatible with the global idempotent registry.

The wizard must not import concrete service packages solely to discover image defaults. Existing imports used for type-specific config prompts remain valid until those prompts have a separate extensibility design.

### Environment helpers

Add two narrow rendering helpers:

- `SortedEnv(map[string]string) []string`: lexically sorted `KEY=value` entries without mutating the input.
- `InstanceEnvArtifacts(render.Input, func(plan.Instance) []string) []render.Artifact`: preserves plan order, joins lines with one trailing newline, mirrors instance zero at root, and emits one-based instance paths.

Services remain responsible for ordering computed entries relative to `IfaceEnv`, bridge entries, and configured environment entries. The helper does not deduplicate keys because duplicate-key precedence may be intentional and is currently order-dependent.

### Canonical image-reference owner

Introduce a leaf package such as `internal/imageref` containing a formatter over `manifest.Image`. Both `render.ImageRef` and `image.ImageReference` either delegate to it temporarily or are replaced directly once callers migrate.

Rules:

- trim only for determining whether repository/tag are absent;
- return empty for an absent repository;
- preserve the supplied repository and non-empty tag text;
- default absent tag to `latest`.

During migration, retain compatibility wrapper names if removing them would expand the review surface. Remove wrappers after all direct callers are migrated and tests demonstrate one owner.

### Image backend dependency direction

Concrete adapters import `internal/image` and use `image.BuildRequest`, `image.PullRequest`, `image.PushRequest`, and `image.TagRequest` in method signatures. `internal/image` does not import adapters, so there is no cycle. The application imports both and chooses an implementation.

Add compile-time assertions:

```go
var _ image.Backend = (*Adapter)(nil)
```

Delete adapter-local image request structs and application forwarding wrappers only after adapter tests compile and pass against canonical requests. Keep `noopImageBackend` in the application because it is application test policy rather than a container-runtime adapter.

### Focused network helper decisions

- Move identical `ipWithPrefix` behavior used by service renderers to `render.IPWithPrefix`, preserving empty and invalid CIDR behavior.
- Move or delegate duplicated `primaryCIDR` selection to the IPAM-owned function only if doing so does not create an `app` dependency inversion; otherwise create an IPAM-domain exported helper with focused tests.
- Confirm the Podman `lastUsableIP` copy has no production caller, then delete it. Keep planner allocation behavior local unless a second real consumer emerges.
- Do not combine host-network execution parsing with IPAM calculations.

### Compatibility test strategy

Use semantic assertions where byte ordering is not contractual and exact assertions where it is:

- Exact: artifact key sets, environment file bytes, renderer names, one-based replica paths, image references, registry metadata, adapter argument arrays.
- Parsed semantic: Compose YAML service/network trees and network attachment fields.
- Service-local: BNG generated configs, gateway health topology, generic-container entrypoint and probe sidecar, XB10 addressing exemption.
- Cross-type tables: manifest port preservation, replica health-port uniqueness, root/per-instance artifact conventions, canonical interface environment variables, IPAM-driver pinning invariants.

A test-only `internal/types/testsupport` package may contain fixture construction and artifact lookup helpers, but not assertions that hide service expectations. Introduce it only after at least three tests share an exact helper contract.

## Risks / Trade-offs

- **[Risk] Shared artifact aggregation changes output paths or loses auxiliary files.** → Add template tests for every path rule and service-level artifact inventory tests before migration.
- **[Risk] YAML decode/remarshal changes byte ordering.** → Compare parsed YAML for semantic compatibility and retain exact tests only for fields or files whose bytes are contractual.
- **[Risk] Duplicate network keys from replicas appear as collisions.** → Accept semantically equal definitions and reject only conflicts; test both cases.
- **[Risk] Config is decoded differently between preflight and rendering.** → Reuse each service's strict typed decoder logic and test malformed/unknown fields through both paths.
- **[Risk] Generic-container does not fit per-instance semantics.** → Keep an explicit interpolated mode that delegates rather than transforms its complete result.
- **[Risk] Embedding metadata defaults conceals unintended behavior.** → Assert effective metadata for every built-in type and explicit overrides for exceptional types.
- **[Risk] Canonical empty-image behavior changes rendered bytes for invalid or incomplete manifests.** → Specify empty repository behavior, add focused tests, and confirm valid manifests are unaffected.
- **[Risk] Adapter imports introduce a package cycle.** → Keep request contracts in `internal/image`, which has no adapter imports; verify with focused compilation before deleting wrappers.
- **[Risk] Renderer migration accidentally changes interface address pinning.** → Freeze parsed network attachment maps before migration and treat discovered nonconformance as separate work.
- **[Risk] XB10 lacks local coverage.** → Add focused XB10 tests before any production migration.
- **[Trade-off] Generic hooks add indirection to stack traces.** → Preserve renderer-specific names and wrap errors with service and instance context.
- **[Trade-off] Some small duplication remains intentionally.** → Prefer visible domain policy over a generic abstraction whose options exceed its consumers.

## Migration Plan

1. Add characterization tests for current cross-type artifact, Compose, registry, wizard-default, image-reference, and adapter behavior. Resolve test ambiguity without changing production behavior.
2. Add `BaseServiceType`, deterministic env helpers, canonical image-reference formatter, and their focused unit tests.
3. Migrate metadata and environment callers incrementally; verify each package.
4. Replace wizard default-image lookup with registry metadata and add coverage for every registered built-in type.
5. Add `render/servicetemplate` with synthetic tests for both modes, hook errors, artifact validation, equivalent duplicate networks, and conflicting keys.
6. Migrate renderers in the stated order, validating after each service. Do not mix policy fixes into these migrations.
7. Convert Docker and Podman adapters to canonical image requests, add compile-time interface assertions, then remove application forwarding wrappers.
8. Consolidate exact network helpers and delete confirmed dead duplicates.
9. Run formatting, focused tests, `go test ./...`, `go build ./...`, and available non-destructive smoke tests. Document environment-gated checks separately.
10. Review the final diff for residual duplicate owners, accidental API expansion, generated artifact changes, and unrelated edits.

Rollback is source-level because this change introduces no persisted-state or runtime migration. Each migration slice can be reverted independently while retaining characterization tests and shared infrastructure until the failing consumer is understood.

## Open Questions

- Should artifact keys be validated against a strict portable relative-path grammar immediately, or should the first implementation reject only absolute paths and `..` traversal to minimize behavior change?
- Should canonical image-reference wrappers remain as deprecated internal delegates for one release, or be removed in the same change after all callers migrate?
- Are Podman-backed smoke tests available in the implementation environment, or should they be recorded as an operator-run release gate?
- Should test fixture helpers be introduced during the first three renderer migrations or deferred until exact repetition appears in the migrated tests?