## Why

Building and editing `vcpe.dev/v1` manifests requires manually writing YAML — a process that is error-prone and opaque, especially for complex topologies with multiple networks, services, and interface wirings. A visual canvas editor embedded in VS Code would let operators drag-and-drop networks and services, draw interface connections, and edit all manifest fields, while keeping the YAML file as the canonical source of truth.

## What Changes

- **NEW**: VS Code extension (`extensions/vcpe-visual-editor/`) providing a custom editor for `**/manifests/*.yaml` files with `apiVersion: vcpe.dev/v1`
- **NEW**: Bidirectional YAML sync — canvas changes are written back to the YAML file via surgical `WorkspaceEdit` operations; YAML edits in a text editor re-render the canvas
- **NEW**: Visual canvas with `NetworkBusNode` (horizontal lane per network role), `ServiceNode` (with per-interface handles), `PhysicalNicNode` (for macvlan parent NICs), `InterfaceEdge` (role-colored), and `DependsOnEdge` (dashed, toggleable)
- **NEW**: Type palette populated from `vcpe service types --json`; drag-drop creates services with default image and stub interfaces pre-wired
- **NEW**: Property panel for all manifest fields; "Deployment Settings" drawer for secrets, `maxReplicasPerService`, `maxActiveDeployments`
- **NEW**: `.vcpe-layout.json` sidecar (committed to git) storing canvas node positions keyed by `<kind>:<identifier>`
- **NEW**: `vcpe service types [--json]` CLI command listing all registered service types with descriptions, default images, and expected role requirements
- **BREAKING**: `ServiceType` interface gains `Description() string` and `DefaultImage() string` — all 7 registered type implementations must be updated
- **BREAKING**: `bng.ExpectedRoles()` enriched from `nil` to `{wan: required, cm: optional, mgmt: optional}` — BNG services without a `wan` interface will now fail preflight
- **BREAKING**: `gateway.ExpectedRoles()` corrected from `[{lan}, {erouter}]` to `{wan: required, cm: optional, lan-p1: optional}` — aligns with actual manifest usage

## Capabilities

### New Capabilities

- `visual-manifest-editor`: VS Code custom text editor for `vcpe.dev/v1` manifests — canvas rendering, bidirectional YAML sync, type palette, property panel, manifest dropdown, welcome screen, layout sidecar, and `make build-extension` / `make install-extension` Makefile targets
- `service-type-introspection`: `vcpe service types [--json]` CLI command and the `Description()` / `DefaultImage()` additions to the `ServiceType` interface that back it
- `manifest-canvas-layout`: `.vcpe-layout.json` sidecar schema and persistence contract — node ID format, version field, git-committed, unknown-ID tolerance, dagre-seeded initial layout

### Modified Capabilities

- `service-type-registry`: `ServiceType` interface gains `Description() string` and `DefaultImage() string`; `ExpectedRoles()` implementations for `bng` and `gateway` are corrected to reflect actual manifest network role usage
- `local-control-plane-cli`: New top-level `service` command group with `types` subcommand added to the CLI surface

## Impact

**Go / controlplane**
- `controlplane/internal/typeregistry/registry.go` — interface additions
- `controlplane/internal/types/bng/bng.go` — interface impl + ExpectedRoles fix
- `controlplane/internal/types/gateway/gateway.go` — interface impl + ExpectedRoles fix
- `controlplane/internal/types/eventsink/eventsink.go` — interface impl
- `controlplane/internal/types/genericcontainer/genericcontainer.go` — interface impl
- `controlplane/internal/types/oktopus/oktopus.go` — interface impl
- `controlplane/internal/types/webpa/webpa.go` — interface impl
- `controlplane/internal/types/xb10/xb10.go` — interface impl
- `controlplane/internal/app/cli.go` — adds `service` to `topLevelCommands`
- `controlplane/internal/app/local.go` — adds `case "service":` dispatch
- `controlplane/internal/app/commands.go` — adds `runService` / `runServiceTypes`

**New: VS Code extension**
- `extensions/vcpe-visual-editor/` — TypeScript extension host + React/Vite webview
- npm dependencies: `react`, `react-dom`, `@xyflow/react`, `yaml`, `@dagrejs/dagre`, `zustand`, `lucide-react`
- devDependencies: `vite`, `vitest`, `typescript`, `@vscode/vsce`, `esbuild`

**Root Makefile**
- New targets: `build-extension`, `install-extension`

**No changes to**: manifest schema (`model.go`), planner, renderer, state, IPAM, Podman integration
