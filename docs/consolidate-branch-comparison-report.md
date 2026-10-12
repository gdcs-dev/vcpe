# Control-Plane Consolidation: Full Branch Comparison

**Evidence date:** 2026-08-12 through 2026-08-18
**Report prepared:** 2026-08-18
**Repository:** `vcpe`

## Executive Summary

Seven branches implement the same goal — remove duplicated code across the control plane's built-in service types, renderer lifecycle, image backends, and related helpers — using different models under two different working arrangements:

- **Base implementations**: one model designs *and* implements the change independently, from the same starting prompt, with no shared specification.
- **Hybrid implementations**: one model (GPT-5.6 Sol) designs the specification once; a different model then implements that same specification.

| Family | Branch | Model | Recorded AI credits |
|---|---|---|---:|
| Base | `sonnet-consolidate` | Sonnet 5 High thinking | 1,099 |
| Base | `luna-consolidate` | GPT-5.6 Luna Medium thinking | 26 |
| Base | `terra-consolidate` | GPT-5.6 Terra Medium thinking | 274 |
| Base | `sol-consolidate` | GPT-5.6 Sol Medium thinking | 3,169 |
| Hybrid | `sol-luna-extrahigh-consolidate` | GPT-5.6 Luna Extra High thinking | 151 + 667 spec = **818** |
| Hybrid | `sol-terra-high-consolidate` | GPT-5.6 Terra High thinking | 829 + 667 spec = **1,496** |
| Hybrid | `sol-sonnet-consolidate` | Sonnet 5 High thinking | 2,775 + 667 spec = **3,442** |

**Recommendation: `sol-terra-high-consolidate`.** It delivers full-scope, clean, well-tested code at the second-lowest true cost of any full-scope branch (**1,496** credits, spec included) — roughly **53% cheaper** than the most expensive full-scope alternative (`sol-sonnet-consolidate`, 3,442) and **47% cheaper** than the only full-scope base effort (`sol-consolidate`, 3,169).

Three findings stand out:

1. **Design ability varies far more across models than implementation ability given a good spec.** `luna-consolidate` and `terra-consolidate`, designing on their own, both under-delivered relative to the full duplication problem. Given Sol's already-vetted specification, both models (`sol-luna-extrahigh-consolidate`, `sol-terra-high-consolidate`) produced complete, high-quality, full-scope implementations.
2. **Sonnet is consistently the most expensive model to execute a given amount of work**, whether designing its own narrower change (1,099 credits) or faithfully implementing Sol's much larger specification (2,775 credits, 3,442 with the spec). `sol-sonnet-consolidate` is, in fact, the single most expensive branch in the entire seven-branch data set — more expensive than Sol's own full design-and-implementation effort.
3. **Cost roughly tracks scope actually delivered, with one exception.** `luna-consolidate` is the cheapest branch of all seven (26 credits) because it delivered the least real consolidation — not because it was more efficient.
4. **Spec quality predicted delivery quality.** Comparing each base branch's own self-authored design documents (see [Specification Quality](#specification-quality-comparing-each-models-own-design-documents)) shows `luna-consolidate`'s specification was the least rigorous of the four — and its own vague, hedged task wording anticipated the incomplete delivery found in its code.

## Scope and Method

The common ancestor for every branch is `d1e7ed1`, the pre-consolidation development commit. The comparison used:

- branch history and diff footprint against that common ancestor;
- build, vet, and full-test results for each branch, run in this evaluation session;
- exact test-name, test-file, and line-count comparisons;
- direct inspection of each branch's shared-renderer and service-type source;
- each branch's own OpenSpec proposal/tasks (where authored independently);
- AI-credit totals from `notes-token-usage-totals.txt`.

### Snapshot References

| Branch | Comparison commit |
|---|---|
| `sonnet-consolidate` | `83d2520` |
| `luna-consolidate` | `c22228a` |
| `terra-consolidate` | `44302f6` |
| `sol-consolidate` | `c787270` |
| `sol-luna-extrahigh-consolidate` | `fd33779` |
| `sol-terra-high-consolidate` | `708f626` (this evaluation snapshot; the branch has since advanced) |
| `sol-sonnet-consolidate` | `f9a9c04` |

### A Note on Cost Figures

`notes-token-usage-totals.txt` separates the three Hybrid implementers' recorded costs from the one-time, shared 667-credit cost of Sol producing the specification they all implemented. This report always states hybrid costs as **implementation-only / true total (implementation + 667)** so readers can see both the marginal cost of a given implementer and the true end-to-end cost of that path. Base-branch costs already include everything (design and implementation by the same model) and need no adjustment.

All branches were re-verified during this evaluation session. Every branch showed the same four test failures beyond a clean `go build`/`go vet`:

- `TestRunRelease_CoherenceFailure` — expects execution from `main`; fails on every non-`main` branch by design. Not a regression.
- `TestAppliedDeploymentStatusCollectsGenericHealthOverHTTP`, `TestNamedStatusCollectsPersistedHealthOverHTTP`, `TestNamedStatusHealthStateMatrix` — these bind hardcoded loopback ports (`127.0.0.1:47000`/`47003`). A live `vcpe` deployment was running on the evaluation machine on these same ports (confirmed via `lsof`/`podman ps`), and this was reproduced identically across branches to confirm it is a local environment collision, not a code defect.

## Cost Data

### Base implementations (design + implementation by one model)

| Model | Branch | AI credits |
|---|---|---:|
| Sonnet 5 High thinking | `sonnet-consolidate` | 1,099 |
| GPT-5.6 Luna Medium thinking | `luna-consolidate` | 26 |
| GPT-5.6 Terra Medium thinking | `terra-consolidate` | 274 |
| GPT-5.6 Sol Medium thinking | `sol-consolidate` | 3,169 |

### Hybrid implementations (Sol's specification, cost 667, implemented by another model)

| Model | Branch | Implementation credits | True total (+667 spec) |
|---|---|---:|---:|
| GPT-5.6 Luna Extra High thinking | `sol-luna-extrahigh-consolidate` | 151 | **818** |
| GPT-5.6 Terra High thinking | `sol-terra-high-consolidate` | 829 | **1,496** |
| Sonnet 5 High thinking | `sol-sonnet-consolidate` | 2,775 | **3,442** |

### All seven branches, ranked by true cost

| Rank | Branch | True total credits | Family | Full scope delivered? |
|---:|---|---:|---|---|
| 1 | `luna-consolidate` | 26 | Base | No |
| 2 | `terra-consolidate` | 274 | Base | No |
| 3 | `sol-luna-extrahigh-consolidate` | 818 | Hybrid | Yes |
| 4 | `sonnet-consolidate` | 1,099 | Base | No |
| 5 | `sol-terra-high-consolidate` | 1,496 | Hybrid | Yes |
| 6 | `sol-consolidate` | 3,169 | Base | Yes |
| 7 | `sol-sonnet-consolidate` | 3,442 | Hybrid | Yes |

Every branch that delivered the full originally-scoped duplication cleanup cost at least 818 credits. The cheapest two branches overall (`luna-consolidate`, `terra-consolidate`) are also the two that delivered the least.

## Base Implementations: Design and Implementation by One Model

Each model here worked from the same starting prompt with no external specification to follow.

### Scope Each Model Chose to Design

| Branch | Self-authored OpenSpec change | Sections | Declared scope |
|---|---|---:|---|
| `sol-consolidate` | `consolidate-controlplane-reuse` | 11 | Full: renderer lifecycle, registry defaults, image-backend (Docker/Podman) conformance, wizard registry lookup, canonical image references, IPAM/render-helper consolidation |
| `sonnet-consolidate` | `consolidate-servicetype-renderers` | 4 | Narrow: renderer lifecycle and registry defaults only |
| `luna-consolidate` | `consolidate-controlplane-code` | 5 | Declared broad — includes a phase evaluating image-backend and app-persistence consolidation — but that phase explicitly concludes by declining most of it |
| `terra-consolidate` | `consolidate-service-type-renderers` | 3 | Narrowest declared scope: renderer lifecycle and registry defaults only |

### Build, Test, and Diff Footprint

| Branch | Build/vet | Tests listed | Files changed | Lines vs. baseline | Net LOC change |
|---|---|---:|---:|---|---:|
| `sonnet-consolidate` | Clean | 227 | 13 | +847 / -898 | **-51** |
| `luna-consolidate` | Clean | 224 | 11 | +143 / -126 | +17 |
| `terra-consolidate` | Clean | 229 | 11 | +675 / -472 | +203 |
| `sol-consolidate` | Clean | 251 | 29 | +1,806 / -637 | +1,169 |

Sonnet is the only base branch with a net line-count *reduction* against the pre-consolidation baseline — consistent with it having the most consolidated renderer design of the four, despite the narrowest declared scope.

### Functional Findings

#### `sol-consolidate`: only full-scope base implementation

Implements the complete scope it designed: shared renderer lifecycle, service-registry defaults, wizard lookup through the registry, canonical image-reference formatting, direct Docker/Podman image-backend conformance, conservative IPAM/render-helper consolidation, and cross-type artifact/Compose/registry/XB10 tests. Its renderer abstraction intentionally leaves service-specific Compose construction local to each type, so per-type Compose/sidecar duplication remains within the seven service types.

**Assessment:** complete, conservative, and maintainable — the strongest base-implementation result.

#### `sonnet-consolidate`: best renderer abstraction, incomplete change

Centralizes more renderer policy than any other branch in this report: the standard Compose service block, network-attachment construction, proxy and probe health-sidecar shapes, and both literal and interpolated rendering modes, all behind a hook-driven design. Representative per-type files were the smallest observed anywhere (`webpa.go`: 63 lines vs. 130–134 in the other branches; `gateway.go`: 231 vs. 305–327).

It did not touch `internal/backend/docker`, `internal/backend/podman`, `internal/app/wizard`, `internal/imageref`, or `internal/ipam`, and has no test files for `internal/types/xb10` or `internal/ipam`.

**Assessment:** the best isolated renderer design of any branch in this report, but not a complete implementation of the full duplication problem.

#### `luna-consolidate`: narrowest actual delivery of the four

Despite an OpenSpec task list that includes a "Consolidate Renderer Assembly" phase, `luna-consolidate` never added a shared renderer-lifecycle package — there is no `internal/render/servicetemplate` or equivalent. What it actually delivered:

- a `typeregistry.BaseServiceType`-style shared metadata-defaults embed, and
- one small shared helper, `render.InstanceEnvArtifacts`, adopted by only 4 of the 7 built-in types (`webpa`, `event-sink`, `oktopus`, `xb10`) — `bng`, `gateway`, and `generic-container` were left untouched.

Every service type still hand-rolls its own `Render()` loop, its own no-instance check, and its own per-instance Compose-fragment construction and merge. The core duplication the initiative targeted — repeated renderer lifecycle plumbing — remains present in all 7 types.

Both `webpa.go` and `event-sink.go` discard the render-time strict-decode error (`_ = typeregistry.StrictDecode(...)`) rather than propagating it. This was initially flagged as a regression introduced by this branch; checking the pre-consolidation baseline shows this exact pattern already existed there for these two types before any consolidation work began. It is therefore a **faithfully preserved pre-existing inconsistency**, not something `luna-consolidate` introduced — see [Cross-Cutting Observations](#cross-cutting-observations) for how this is handled differently across branches.

**Assessment:** builds and passes tests, but delivers the least consolidation of the four base efforts.

#### `terra-consolidate`: honest narrow scope, real (if minimal) shared lifecycle

Its own OpenSpec change explicitly scoped itself to renderer lifecycle and registry defaults, and delivered exactly that: a genuine `internal/render/servicetemplate` package (`Hooks[C]`/`New[C]`, per-instance dispatch, Compose-fragment aggregation) used by all 7 built-in types. It is smaller and less defensive than the equivalent packages in `sol-consolidate` or `sonnet-consolidate` — no artifact-key validation (no path-traversal or absolute-path checks), no conflict detection on duplicate Compose keys (a later fragment silently overwrites an earlier one via plain map assignment), and no validation that a fragment's `services`/`networks` sections are actually mappings.

**Assessment:** the most internally consistent narrow-scope branch — its own proposal and its delivered code agree, and it actually removed the targeted duplication within that scope, just without the defensive edge-case handling `sol`/`sonnet` added.

### Base Implementation Comparison Matrix

| Criterion | Sonnet | Sol | Luna | Terra |
|---|---|---|---|---|
| Declared scope breadth | Narrow (4 sections) | Full (11 sections) | Declared broad, delivered narrow (5 sections) | Narrowest (3 sections) |
| Shared renderer lifecycle actually built | Yes — richest (compose-block + sidecar builders) | Yes | **No** — one helper, adopted by 4 of 7 types | Yes — minimal but real |
| Net LOC vs. baseline | -51 (net reduction) | +1,169 | +17 | +203 |
| Image backend / wizard / imageref / IPAM consolidation | No | Yes | No (evaluated, declined) | No |
| Overall judgment | Best isolated renderer design, narrow scope | Only full-scope base implementation | Weakest delivery relative to its own plan | Leanest complete narrow-scope delivery |

### Specification Quality: Comparing Each Model's Own Design Documents

The functional findings above compare delivered code. This section compares what each model *wrote down before building anything* — its self-authored `proposal.md`, `design.md`, and `tasks.md` under `openspec/changes/archive/` — to see whether spec quality predicted delivery quality.

#### Document Size

| Branch | Archived change | `proposal.md` | `design.md` | `tasks.md` |
|---|---|---:|---:|---:|
| `sol-consolidate` | `2026-08-18-consolidate-controlplane-reuse` | 31 lines | 205 lines | 93 lines |
| `sonnet-consolidate` | `2026-08-12-consolidate-servicetype-renderers` | 27 lines | 139 lines | 29 lines |
| `luna-consolidate` | `2026-08-12-consolidate-controlplane-code` | 43 lines | 72 lines | 31 lines |
| `terra-consolidate` | `2026-08-12-consolidate-service-type-renderers` | 26 lines | 44 lines | 17 lines |

Document length alone is a weak signal (Luna's proposal is the longest of the four), but `design.md` length tracks delivered scope and depth closely: Sol (205 lines, full scope) and Sonnet (139 lines, narrow-but-rich scope) both wrote substantially more detailed designs than Luna (72 lines) or Terra (44 lines), and both went on to deliver more than Luna and Terra did.

#### Investigative Rigor

`sonnet-consolidate`'s proposal stands out here: it names the exact duplicated elements per package (`Register()`/`sync.Once` boilerplate, the compose "service block" map fields) and — uniquely among all four — reports a **specific, already-verified conformance bug**, citing two already-ratified specs by name (`interface-addressing-mode`, `podman-reconciliation-engine`) and auditing each service type's actual `ipv4_address`-pinning behavior against them: `bng`/`webpa` correct, `gateway` never pins (non-conformant), `oktopus`/`event-sink` always pin (non-conformant), `xb10` correctly exempt. This is a falsifiable, specific claim, not a generic "duplication causes drift" statement — and the later code-level comparison in this report confirms it was accurate and that the fix was actually delivered.

`sol-consolidate`'s proposal is concrete about *which packages* repeat *which kinds* of logic (renderer lifecycle, image backend request translation, environment formatting, network/image helpers) but does not perform a similarly specific pre-implementation audit in the proposal itself; the equivalent rigor shows up later, in its design.md's `Open Questions` and its explicit decision to *not* fix any conformance mismatches it might find, deferring them to a separate change instead.

`luna-consolidate` and `terra-consolidate`'s proposals stay categorical ("repeated service metadata methods," "three classes of common behavior") with no named examples, no specific bug citations, and no evidence of an audit against existing ratified specs.

#### Decision Depth

Both `sol-consolidate` and `sonnet-consolidate` specify a concrete Go API shape in `design.md` — full struct/interface signatures, not prose — and both explicitly document **rejected alternatives** with reasoning (a strong design-doc practice that surfaces trade-offs rather than presenting the chosen approach as the only option considered). Both also include an `Open Questions` section flagging a genuine unresolved judgment call rather than presenting every decision as settled.

`sol-consolidate` goes further with a fully-ordered, numbered artifact-aggregation algorithm (12 explicit steps) and a taxonomy of test strategies (exact vs. parsed-semantic vs. service-local vs. cross-type table) — the most procedurally detailed design of the four.

`terra-consolidate`'s design is brief but does commit to a concrete shared-template contract and includes one explicit rejected-alternative ("one broad Compose builder with flags for every service behavior... cannot cover BNG configuration artifacts cleanly") — thin, but directly actionable, which matches its later delivery of a real (if minimal) shared package.

`luna-consolidate`'s design is the least committal on the one decision that mattered most: its renderer-consolidation decision reads "Add render-owned helpers for artifact list construction and common instance artifact naming **only after confirming each caller has the same semantics**" — no concrete API shape, no committed contract. Its distinctive `Evaluation Outcomes` section transparently documents what it decided *not* to consolidate (app persistence-store lifecycle, Docker/Podman command plumbing) with brief reasoning, which is a good practice — but it does not compensate for the vagueness on the core renderer-consolidation decision.

#### Plan vs. Delivery

Cross-referencing each proposal's own words against the Functional Findings above:

| Branch | What the design promised | What the code delivered |
|---|---|---|
| `sol-consolidate` | Full shared renderer lifecycle + image backend/wizard/imageref/IPAM consolidation | Matches — delivered in full |
| `sonnet-consolidate` | Shared renderer template (all 7 types) + named health-sidecar mechanisms + `ipv4_address` conformance fix | Matches — delivered exactly as designed, conformance fix included |
| `terra-consolidate` | Shared renderer template (all 7 types), registry defaults only | Matches — narrow scope delivered as designed |
| `luna-consolidate` | "Consolidate mechanically identical renderer artifact assembly" across built-in renderers | **Does not match** — only one small env-artifact helper shipped, adopted by 4 of 7 types; no shared renderer lifecycle exists |

Three of the four branches did what their own design documents said they would do. `luna-consolidate`'s task list used hedged language ("only mechanically identical paths," "small... helpers") that, in hindsight, gave itself room to under-deliver relative to what its own proposal's "What Changes" section ("Consolidate mechanically identical renderer artifact assembly and validation") implies to a reader — the gap between the design's framing and its own qualifying language is itself a signal worth noting for future spec reviews.

#### Verdict

`sonnet-consolidate` and `sol-consolidate` wrote the two strongest specifications in this data set — both grounded in specific evidence, both showing rejected alternatives and open questions, and both accurately predicting what their own code would deliver. `terra-consolidate` wrote an honest, appropriately narrow specification and delivered on it exactly. `luna-consolidate` wrote the least rigorous specification of the four, and its own vagueness on the central renderer-consolidation decision is a leading indicator — visible before a single line of code was reviewed — of the incomplete delivery documented earlier in this report.

## Hybrid Implementations: Implementing Sol's Specification

All three implementers worked from the identical `consolidate-controlplane-reuse` specification Sol designed (667 credits), so scope is held constant — this isolates implementation quality and cost from design choice.

### Build, Test, and Diff Footprint

| Branch | Build/vet | Tests listed | Files changed vs. baseline | Lines vs. baseline |
|---|---|---:|---:|---|
| `sol-luna-extrahigh-consolidate` | Clean | ~253 | — | — |
| `sol-terra-high-consolidate` | Clean | 244 | — | — |
| `sol-sonnet-consolidate` | Clean | **271** | 43 | +2,472 / -715 |

`sol-sonnet-consolidate` has both the largest diff footprint and the highest test count of any branch in this report, including the base `sol-consolidate` branch it shares a specification lineage with.

### Functional Findings

#### `sol-luna-extrahigh-consolidate`: complete scope with residual boilerplate

Covers the same full scope as `sol-consolidate`, but every built-in service type retains a forwarding wrapper of the form:

```go
type renderer struct{}

func (renderer) Name() string { ... }
func (renderer) Render(...) { return servicetemplate.New(...).Render(...) }
```

Those wrappers add no service-specific behavior and contradict the consolidation goal; the branch also retains redundant no-instance checks the shared lifecycle already enforces. Per-type files were consistently larger than `sol-consolidate`'s equivalents with no corresponding coverage advantage (e.g. `bng.go` ~817 lines vs. 797).

**Assessment:** functionally complete, but the least polished of the three hybrid implementations.

#### `sol-terra-high-consolidate`: best value among the full-scope branches

Implements the same full scope as `sol-consolidate` and converges on the clean, direct-renderer pattern rather than the Luna wrapper. Its test-count difference from Sol (244 vs. 251) is mostly test reorganization, not lost coverage. Two details gave it an edge over `sol-consolidate` itself: more defensive Compose-fragment handling (structured YAML-node decoding, explicit mapping validation, normalized-value equality checks on merge) and an explicit compile-time assertion plus dedicated test (`TestNewImageBackend`) verifying concrete Docker/Podman adapter selection.

**Assessment:** the best balance of completeness, cleanliness, and cost among all seven branches.

#### `sol-sonnet-consolidate`: most thorough, most expensive

Implements the same full scope, spread across a well-modularized `internal/render/servicetemplate` package (`servicetemplate.go`, `perinstance.go`, `interpolated.go`, `compose.go`, `validate.go`, each with a matching test file). Its architecture is clean — every service type wires its renderer through a small `newRenderer()` factory that returns `servicetemplate.New(...)` directly, with no dead wrapper struct.

Its Compose-fragment handling is as defensive as `sol-terra-high-consolidate`'s, or slightly more explicit: a dedicated `composeAggregator` type merges services/networks with `reflect.DeepEqual`-based conflict detection, and a separate `validateArtifactKey` helper rejects empty, absolute, and parent-traversal artifact keys. Per-instance rendering additionally rejects a missing or multiple `compose.yaml` fragments from a single hook invocation and duplicate artifact keys within one invocation — slightly more explicit checking than either `sol-consolidate` or `sol-terra-high-consolidate`.

Test coverage is the deepest observed in this report: 271 tests, including dedicated files for image-reference behavior, IP-prefix formatting, instance-environment artifacts, sorted-environment output, base service-type defaults, cross-type artifact inventory, cross-type Compose semantics, XB10-specific behavior, template lifecycle, artifact placement, and hook validation.

Like `luna-consolidate`, it preserves the pre-existing (not newly introduced) decode-error-swallowing behavior in `webpa`/`event-sink` — but here it is a **documented, deliberate choice**, with an explicit code comment: "preflight owns strict validation, so a decode error here is swallowed rather than failing rendering." This is a defensible, intentional design position, not an unexplained inconsistency.

**Assessment:** the most thoroughly tested and most defensively coded of all seven branches — but at nearly the cost of Sol's own full design-and-implementation effort, and roughly 2.3x `sol-terra-high-consolidate`'s true total cost for a difference in real-world behavior that is marginal.

### Hybrid Implementation Comparison Matrix

| Criterion | Luna Extra High | Terra High | Sonnet 5 High |
|---|---|---|---|
| Full scope delivered | Yes | Yes | Yes |
| Clean renderer pattern (no dead wrapper) | No — retains `type renderer struct{}` | Yes | Yes |
| Compose-fragment defensiveness | Standard | Strong (structured decode, mapping checks) | Strongest (dedicated aggregator + key validation) |
| Tests listed | ~253 | 244 | **271** |
| True total cost | 818 | 1,496 | 3,442 |
| Overall judgment | Complete but least polished | **Best overall value** | Most thorough, most expensive |

## Cross-Cutting Observations

### Design vs. Implementation Capability

Luna and Terra each appear twice in this data set: once designing their own change, and once implementing Sol's. This is a direct, controlled before/after for the same two models:

| Model | Own design + implementation | True cost | Scope achieved | Implementing Sol's spec | True cost | Scope achieved |
|---|---|---:|---|---|---:|---|
| Luna | `luna-consolidate` | 26 | Narrow, incomplete relative to its own plan | `sol-luna-extrahigh-consolidate` | 818 | Full |
| Terra | `terra-consolidate` | 274 | Narrow, but complete for its own declared scope | `sol-terra-high-consolidate` | 1,496 | Full — best overall of all 7 branches |

Both models delivered dramatically more complete, higher-quality results when given a concrete, already-vetted specification to execute than when asked to design the consolidation themselves. The highest-leverage workflow observed in this entire evaluation is: **use a strong model to produce the specification once, then use a different model to execute it** — rather than asking that model to do both.

### Sonnet Is Consistently the Most Expensive Executor

Sonnet appears twice as well, and the pattern is the opposite of Luna/Terra's: it is expensive regardless of whether it is designing or merely implementing.

| Role | Branch | True cost | Scope |
|---|---|---:|---|
| Own design + implementation | `sonnet-consolidate` | 1,099 | Narrow (4 sections) |
| Implementing Sol's spec | `sol-sonnet-consolidate` | 3,442 | Full (11 sections) |

Implementing a larger, more thorough specification naturally costs more regardless of model — but Sonnet's cost for executing the *identical* specification Luna Extra High and Terra High also implemented was **4.2x** Terra High's and **22.8x** Luna Extra High's. The resulting code is arguably the most thorough of the three, but not proportionally so: `sol-terra-high-consolidate` matches it on scope, cleanliness, and functional completeness at well under half the cost.

### A Corrected Observation on Decode-Error Handling

An earlier pass of this analysis flagged `webpa`/`event-sink`'s discarded render-time decode error in `luna-consolidate` as a regression. Comparing against the pre-consolidation baseline (`d1e7ed1`) shows this exact behavior already existed there for those two types — both `luna-consolidate` and `sol-sonnet-consolidate` preserve it faithfully, while `sol-consolidate` (and, by inheritance, its hybrid derivatives) changed it to propagate the error. Preserving pre-existing behavior exactly is the correct default for a pure refactor; whether the stricter behavior is a deliberate, separately-justified improvement or an incidental change worth flagging is a judgment call, not a defect in either direction. This report treats it as a minor, low-severity note rather than a quality differentiator.

## Final Recommendation

### Recommended branch

Ship **`sol-terra-high-consolidate`**.

It delivers the same full scope as the most thorough branch (`sol-sonnet-consolidate`) and the same cleanliness as the most expensive full-scope branches, at **1,496** true total credits — less than half of `sol-sonnet-consolidate`'s 3,442 and well below `sol-consolidate`'s 3,169.

### If a single model must both design and implement, with no outside spec

- **`sol-consolidate`** is the only base branch that delivers the full originally-scoped duplication cleanup.
- **`sonnet-consolidate`** is the best budget option if a narrower, renderer-only scope is acceptable.
- **`terra-consolidate`** is an acceptable narrow-scope option, honestly matched to its own stated plan.
- **`luna-consolidate`** is not recommended as a design-and-implement effort in its current form — its own task list claims work its code does not deliver.

### If maximum thoroughness is the priority regardless of cost

`sol-sonnet-consolidate` is the most defensively coded and most thoroughly tested branch evaluated. It is a reasonable choice only when the ~2.3x cost premium over `sol-terra-high-consolidate` is acceptable for the incremental test depth and slightly more explicit validation it adds.

### Process takeaway

The clearest lesson from this data set is procedural, not branch-specific: **have one capable model produce a specification once, then have a different (and, per this data, ideally a lower-cost) model implement it.** Every hybrid branch outperformed its corresponding model's own from-scratch design in completeness, and every hybrid branch's true cost was less than doing the equivalent scope of work as a single from-scratch design effort would likely cost with the same model.

## Caveats

- All test results in this report were collected on non-`main` branches, so the release-coherence test failure is expected on every branch and unrelated to code quality.
- Test-count comparisons are approximate where a branch renamed or reorganized existing tests rather than changing coverage.
- This report evaluates each branch at the commit noted in Snapshot References. `sol-terra-high-consolidate` has since advanced with additional work (a follow-on `consolidate-compose-service-builder` change); that later work is out of scope for this comparison.
- Cost figures are as recorded in `notes-token-usage-totals.txt` and are not independently verified beyond taking the source at face value.
