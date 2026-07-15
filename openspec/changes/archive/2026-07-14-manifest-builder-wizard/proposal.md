## Why

Creating a `vcpe.dev/v1` manifest today requires detailed knowledge of the schema, manual coordination between network CIDRs and service DHCP config, and separate documentation lookup for every field. There is no guided path from "I have a Podman host with eth0 and want a BNG deployment" to a valid, ready-to-run manifest. The macvlan workflow in particular requires discovering the correct parent interface name from the Podman host, which has no CLI support today.

## What Changes

- Add `vcpe manifest build` subcommand with two modes:
  - **Create mode** (`vcpe manifest build`): interactive wizard that generates a complete manifest from scratch.
  - **Update mode** (`vcpe manifest build --manifest <path>`): loads an existing manifest, pre-fills all wizard prompts with the current values, and writes to `--output <path>` (default: `<stem>-updated.yaml`).
- The wizard runs in four sequential phases: identity → networks → services → output.
- **Network phase** discovers available physical interfaces from the Podman host (via `ip -j link`) for `macvlan`/`ipvlan` networks and presents a numbered menu with interface details (type, state, speed, IP).
- **Service config phase** pre-fills type-specific config fields from the network definitions collected in the network phase (BNG DHCP4 subnet/ranges/routers derived from the attached network's CIDR/pool/gateway; gateway LAN config derived from the lan-p1 network).
- The prompt helper returns the default value without blocking when stdin is not a TTY, making the wizard CI-safe.
- Service types are sourced from `typeregistry.Registered()` so new types are automatically listed.

## Capabilities

### New Capabilities
- `manifest-builder-wizard`: `vcpe manifest build` SHALL provide an interactive, fully guided wizard that collects all manifest fields with contextual defaults and produces a valid `vcpe.dev/v1` manifest. The wizard SHALL discover available interfaces from the Podman host for macvlan/ipvlan parent selection. Service config fields SHALL be pre-filled from the networks defined in the same wizard session. The wizard SHALL be non-blocking when stdin is not a TTY.

### Modified Capabilities
- `local-control-plane-cli`: The `manifest` command group gains a `build` subcommand alongside the existing `list` subcommand.

## Impact

- **`internal/app/commands.go`** — add `runManifestBuild` dispatched by `runManifest`
- **`internal/app/wizard/`** — new package: `prompt.go` (TTY-safe prompt helper), `network.go` (network phase + interface discovery), `service.go` (service phase with type-specific config), `output.go` (YAML serialization)
- **`internal/hostnet/adapter.go`** — add `ListInterfaces(ctx) ([]InterfaceInfo, error)` using `ip -j link` via existing runner pattern
- **`internal/app/help.go`** — add `manifest build` help entry and update `manifest` synopsis
- No changes to manifest schema, planner, or state
