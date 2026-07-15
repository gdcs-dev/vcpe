## Context

Three READMEs are out of date relative to the current `vcpe` implementation. The root README is the most impactful — it is the primary operator onboarding document. The Homebrew README has a few targeted factual errors (wrong default channel, wrong primary command). The event-sink README is mostly accurate but frames deployment with `docker compose` instead of `vcpe up`.

Current implementation facts that READMEs miss:
- Commands added since last README update: `manifest list`, `manifest build` (wizard), `build`, `push`, `release`, `version`
- Registered service types: `bng`, `gateway`, `webpa`, `event-sink`, `xb10`, `oktopus`, `generic-container`
- Network fields: `driver`, `driverOptions`, `ipamDriver` (e.g. `ipamDriver: none` skips Podman IPAM)
- Homebrew channel default is `release` (changed from `development`)
- `vcpe apply` is an alias for `vcpe up`; `vcpe up` is the primary command
- Example manifests live in `manifests/` and are discoverable via `vcpe manifest list`
- `vcpe release --version vX.Y.Z` is the full release workflow (stamp + git + image push)

## Goals / Non-Goals

**Goals:**
- README.md: accurate Installation, Quick Start, Commands, and Service Types sections
- packaging/homebrew/README.md: correct channel default, correct primary command
- services/event-sink/README.md: `vcpe up` as primary deployment path

**Non-Goals:**
- Rewriting docs/ (architecture.md, networking.md, runbook.md) — separate effort
- Adding API reference or deep design rationale to READMEs
- Documenting every flag on every command (help text covers that)

## Decisions

### Decision: Quick Start uses manifests/example.yaml, not inline YAML

The inline manifest block in the current README is 40+ lines and immediately stale. `manifests/example.yaml` is version-controlled and kept current. The Quick Start should point to it and show `vcpe manifest build` as the way to create new manifests.

### Decision: Commands section replaces the scattered usage examples

A concise table or list of all top-level commands replaces ad-hoc examples. Each entry maps to `vcpe <cmd> --help` for full flag reference.

### Decision: event-sink README keeps the docker compose section as secondary

The standalone `docker compose` invocation is valid for unit-testing the sink in isolation. Keep it, but move it to a "Standalone / Development" sub-section below the `vcpe up` primary path.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Quick Start references manifests/example.yaml which has real service images | The image `pullPolicy: always-pull` means first run pulls; the README notes this |
| Removing the inline manifest block may confuse users who want a minimal example | Keep a minimal 2-network example in a collapsed code block or link to the manifest discovery docs |

## Open Questions

None — all decisions are clear-cut factual corrections.
