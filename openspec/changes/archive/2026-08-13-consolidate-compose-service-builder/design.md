## Context

`consolidate-controlplane-reuse` migrated all 7 built-in service types onto a shared `internal/render/servicetemplate` lifecycle (config decode, replica fan-out, standard artifact placement, Compose fragment merge), but explicitly kept Compose service-block construction and health-sidecar wiring in each type's own package. Measured on the current tree:

```
bng.go        renderBNGCompose()
gateway.go    renderGatewayCompose()
webpa.go      renderWebPACompose()
xb10.go       renderXB10Compose()
oktopus.go    generateCompose()
eventsink.go  (inline in RenderInstance)
genericcontainer.go (inline, two places)
```

Each of these independently builds the same `map[string]any{"image":.., "container_name":.., "hostname":.., "env_file":.., "networks":.., "ports":..}` shape, then — for the 3 types that need it (gateway, generic-container, and any future opt-in type) — hand-rolls one of exactly two health-sidecar YAML blocks that are otherwise byte-for-byte identical in structure across types.

The prior design.md recorded this as a deliberate Non-Goal, reasoning that a shared builder risks becoming "a policy matrix" if it grows a flag for every field that might differ per type (privileges, volumes, commands, network attachment, health). That risk is real and is the central design constraint for this change: the shared builder must not grow an unbounded hook surface. The goal here is to consolidate only the parts that are already provably identical across types (the block scaffolding, the two known sidecar shapes) and to cap the amount of per-type escape hatch to exactly one mutator function, so that a type needing something genuinely new must add a new local Compose fragment rather than pushing a new flag into the shared builder.

## Goals / Non-Goals

**Goals:**
- Eliminate the ~6 duplicated Compose service-block constructors across built-in types.
- Eliminate the ~2 duplicated health-sidecar YAML constructors (proxy shape, probe shape).
- Cap the shared escape-hatch surface at exactly one Compose-mutator hook plus one network-attachment hook — no per-field flags.
- Preserve every guarantee already ratified by the "Stable renderer artifact contract" requirement (artifact keys, first-instance mirroring, Compose semantic equivalence).
- Verify every migrated type against its own existing golden/characterization tests before moving to the next type.

**Non-Goals:**
- Do not invent a third, generalized health mechanism. Only the two shapes that already exist (proxy-sidecar, probe-sidecar) are shared; a future type needing a genuinely different mechanism adds its own, local sidecar block.
- Do not centralize service-specific auxiliary artifact generation — bng's `dhcpd.conf`/`dhcpd6.conf`/`radvd.conf`/etc, and generic-container's `entrypoint.sh`, stay local, produced the same way they are today (as additional artifacts alongside the shared `compose.yaml`).
- Do not change how a type decides whether it needs a health sidecar (gateway's `hasHealthUpstream`, generic-container's `cfg.Health != nil`) — those triggers stay in service packages; only the resulting sidecar block's YAML construction is shared.
- Do not change `NetworkAttachment`/health trigger evaluation timing, ports/health-port allocation, or the `render.Input`/`render.Result` contracts.
- Do not touch `internal/backend/{docker,podman}`, `internal/app/wizard`, `internal/imageref`, or `internal/ipam` — those are already consolidated by `consolidate-controlplane-reuse` and are out of scope here.

## Decisions

### Shared Compose service-block builder

Add a builder function (owned by `servicetemplate`, invoked from the existing per-instance render path) that constructs the standard fields every curated type already emits identically:

```go
func buildComposeService(input render.Input, instance plan.Instance, image string, ports []string) map[string]any
```

It returns `image`, `container_name` (`<deployment>-<service>-<n>`), `hostname` (`<service>-<n>`), `env_file` (`instances/<n>/compose.env`), and `networks` (populated via the network-attachment hook below). Callers append `ports` themselves (existing per-type port logic is unchanged; ports are not part of the duplication being removed).

### Network-attachment hook

```go
type NetworkAttachment func(iface plan.Interface, managed bool) map[string]any
```

Two implementations already exist in practice and are promoted to shared, named helpers:
- `DefaultAttachment` — always pins `mac_address`; pins `ipv4_address` iff the interface's network is Podman-managed (`ipamDriver != "none"`).
- `MACOnlyAttachment` — always pins `mac_address`, never pins `ipv4_address` (gateway, xb10's existing addressing exemption).

A type that supplies neither gets `DefaultAttachment`. No third variant is introduced without a proposal amendment.

### Shared health-sidecar builders

```go
func buildProxySidecar(input render.Input, instanceName string, index int, image string) (services, networks map[string]any)
func buildProbeSidecar(serviceName string, index int, image string, command []string) map[string]any
```

These reproduce the exact existing YAML already characterized by `gateway_golden_test.go` (`TestGatewayRendersHealthSidecarOnlyWhenHealthUpstreamDeclared`) and `genericcontainer_golden_test.go` (`TestGenericContainerRendersConfiguredHealthSidecar`). A type opts in by calling the matching builder from within its `RenderInstance` hook when its own trigger condition is true — the shared lifecycle does not decide when a sidecar is needed.

### The one escape hatch

```go
type ComposeExtras func(input render.Input, cfg C, instance plan.Instance, svc map[string]any)
```

Called once per instance after the shared block is built and before it is returned as part of the Compose fragment, so a type can set `privileged`, `cap_add`, `volumes`, `network_mode`, or `command` directly on the `map[string]any`. This is the single, deliberate, capped point of per-type divergence. If a future need doesn't fit this shape, the answer is "keep that type's Compose construction local for that piece," not "add a new hook."

### Migration order

Migrate lowest-risk types first so failures are cheap to isolate:
1. `event-sink`, `webpa`, `oktopus` — no privileges, no sidecar, `DefaultAttachment` only.
2. `xb10` — `MACOnlyAttachment`, no sidecar.
3. `gateway` — `MACOnlyAttachment` + proxy sidecar + `ComposeExtras` (privileged, cap_add, volumes).
4. `generic-container` — probe sidecar + `ComposeExtras`, still on `Interpolated` mode (unchanged).
5. `bng` — migrated last; largest surface, most auxiliary artifacts, exercises the escape hatch least.

Each step: migrate, run that type's package tests plus `internal/render/servicetemplate` and the cross-type `port_parity_test.go`/artifact-inventory tests, fix before proceeding.

## Risks / Trade-offs

- **Blast radius**: a bug in the shared builder now affects up to 7 types instead of 1. Mitigated by migrating one type at a time behind its existing tests, and by adding dedicated unit tests for `buildComposeService`/`buildProxySidecar`/`buildProbeSidecar` in `servicetemplate` before any type migrates onto them.
- **Escape-hatch creep**: the exact failure mode the original non-goal was written to avoid. Mitigated by hard-capping the surface to one mutator hook and two named attachment helpers; adding a third attachment variant or a second mutator hook requires revisiting this design, not a quiet extension.
- **Spec tension**: this change explicitly reverses a ratified non-goal in `controlplane-code-reuse`. The spec delta must replace, not silently ignore, the conflicting scenario and Consolidation-boundaries text.
