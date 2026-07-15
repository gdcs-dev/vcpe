## Context

`vcpe manifest list` already exists as a subcommand dispatched through `runManifest`. Adding `build` follows the same pattern: `case "build": return runManifestBuild(opts)`. The wizard logic lives in a new `internal/app/wizard/` package to keep `commands.go` clean.

The hostnet adapter's `runner()` already handles "run locally or via `podman machine ssh`" for Linux host commands. `ip -j link` for interface discovery follows exactly the same pattern — one new method on `Adapter`.

The `yaml.Node` approach (already used for `service.Config`) gives comment-preserving round-trips for update mode, but is complex for full-document use. For v1, update mode uses struct round-trip (comments may be lost) and writes to a new file to make this explicit. The user reviews and renames.

## Goals / Non-Goals

**Goals:**
- Four-phase wizard: identity → networks → services → output.
- Interface discovery for macvlan/ipvlan parent selection.
- Smart defaults: service config pre-filled from network definitions.
- TTY-safe prompt helper (returns default when stdin is not a TTY).
- Update mode: load existing manifest, pre-fill prompts, write to new file.
- All registered service types available (from `typeregistry.Registered()`).

**Non-Goals:**
- Comment-preserving in-place update (v2).
- `ServiceType` interface change to add `ConfigWizard()` (v2 refactor).
- Wizard support for `webpa`, `event-sink`, `xb10` config (empty configs — no questions needed).
- `--json` / non-interactive batch mode (TTY-safe default covers CI).

## Decisions

**Wizard lives in `internal/app/wizard/` package**
Keeps the 300+ line wizard logic out of `commands.go`. The package exports a single `Run(opts WizardOpts) (manifest.Document, error)` function. Easy to test in isolation.

**Type-specific config: switch in wizard package, not per-type method**
Avoids touching the `ServiceType` interface and all type packages for v1. The wizard has `case "bng": askBNGConfig(...)`, `case "gateway": askGatewayConfig(...)`, etc. Refactor to `ConfigWizard()` method per type is a clean v2.

**Smart defaults: network lookup map passed to service phase**
After the network phase, the wizard holds `map[role]NetworkEntry{CIDR, Gateway, PoolStart, PoolEnd}`. The service phase passes this to each type-specific config asker. BNG derives `dhcp4.Subnet` = CIDR, `dhcp4.Ranges[0]` = pool, `dhcp4.Options["routers"]` = gateway. User overrides by typing a different value.

**Interface discovery: `hostnet.Adapter.ListInterfaces(ctx)`**
Returns `[]InterfaceInfo{Name, Type, State, Addresses}` from `ip -j link`. Filters out loopback, virtual bridges (`podman*`, `cni-*`), and macvlan sub-interfaces (`link_type == "macvlan"`). macOS auto-delegates to Podman machine via existing runner.

**Update mode writes to `<stem>-updated.yaml`**
Avoids in-place destruction of the original and makes the workflow explicit. `--output` overrides. Update mode pre-fills every prompt with the existing value so pressing Enter keeps the current value unchanged.

**Prompt helper: `wizard.Prompt(label, defaultVal string) string`**
Writes `label [defaultVal]: ` to stderr. Reads one line from stdin. Returns `defaultVal` if stdin is not a TTY or line is empty. This is the entire interactive I/O surface.

## Risks / Trade-offs

**Risk: Interface discovery fails (no `ip` command, no Podman machine)**
→ Wizard falls back to free-text entry: "Enter parent interface name manually:". Never blocks.

**Risk: Comment loss in update mode**
→ Documented clearly: "Note: comments from the original manifest are not preserved in the output file."

**Risk: Wizard is long for full deployments (many Enter presses)**
→ Phase-level skip: pressing `s` at any "Add another network/service?" prompt jumps to next phase. Documented in help text.
