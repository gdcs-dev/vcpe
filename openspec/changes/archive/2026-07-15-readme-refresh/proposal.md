## Why

The three target READMEs (`README.md`, `packaging/homebrew/README.md`, `services/event-sink/README.md`) were written before several significant features landed: the interactive manifest builder wizard, the `vcpe release` workflow, multi-arch image building, the `ipamDriver` network field, and the Homebrew channel default change. They also reference outdated command names, missing service types, and an old quick-start pattern.

## What Changes

### README.md (root)

- Replace the verbose inline `manifest-bng-7.yaml` quick-start with a reference to `manifests/example.yaml` and `vcpe manifest build` as the way to author new manifests.
- Add the full current command surface: `manifest list`, `manifest build`, `build`, `push`, `release`, `version`.
- Update the service types table: add `event-sink`, `xb10`, `oktopus` alongside the existing types.
- Add `ipamDriver: none` to the network features description.
- Add Homebrew as an installation path (alongside the existing `go build` path).
- Replace `manifest-bng-7.yaml` references in troubleshooting examples with `--name <deployment>` generic form.

### packaging/homebrew/README.md

- Fix the channels table: `release` is the default channel, not `development`.
- Replace `vcpe apply` with `vcpe up` (primary command; `apply` is an alias).
- Update the sync example to use `VCPE_HOMEBREW_CHANNEL=release` (the default).
- Add `vcpe release` to the usage section as the release workflow.
- Add `vcpe manifest build` to the usage section.

### services/event-sink/README.md

- Reframe "Starting the service": `vcpe up --manifest ...` is the primary path; standalone `docker compose` is a secondary dev-only path.
- Remove the `docker compose --env-file services/webpa/compose.env` invocation as the primary example (it no longer reflects the operator workflow).

## Capabilities

### New Capabilities

_(none — this is a documentation-only change)_

### Modified Capabilities

_(none — no spec-level behavior changes)_

## Impact

- `README.md`: significant rewrite of Quick Start, Service Types, and command sections; new Installation section.
- `packaging/homebrew/README.md`: targeted corrections to channels table, command examples, and sync example.
- `services/event-sink/README.md`: reorder Starting the Service section; add `vcpe up` as primary path.
- No Go code changes. No spec changes.
