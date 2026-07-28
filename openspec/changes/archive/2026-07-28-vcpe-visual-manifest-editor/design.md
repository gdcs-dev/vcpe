## Context

The `vcpe.dev/v1` manifest schema defines a bipartite topology: `networks` (L2/L3 segments) and `services` (workloads with typed configs) connected via `interfaces` (named role attachments). Currently the only authoring tool is a text editor. The control plane binary (`vcpe`) is the sole source of truth for registered service types; the manifest YAML file is the sole source of truth for deployment desired state.

The extension operates in the VS Code extension host (Node.js, sandboxed), communicates with a sandboxed webview (React) via the VS Code `postMessage` API, and reads/writes the manifest YAML file via `WorkspaceEdit`. All decisions are fully specified in `decisions.md`.

## Goals / Non-Goals

**Goals:**
- Bidirectional visual editing of all `vcpe.dev/v1` manifest fields with the YAML file as canonical source
- Drag-and-drop service and network creation from a palette derived from the running `vcpe` binary
- Full interface wiring via canvas edge drawing; `dependsOn` edges with toggleable visibility
- macvlan topology represented with explicit `PhysicalNicNode` per parent NIC
- All manifest mutations write back to YAML via `WorkspaceEdit` with comment and formatting preservation
- `.vcpe-layout.json` sidecar (committed to git) persists canvas positions; dagre seeds initial layout

**Non-Goals:**
- Network role rename (blocked v1 — opaque config subtrees embed role names; safe rename requires config schema not available to the extension)
- Typed per-type config forms (deferred — config edited as raw YAML text in property panel)
- VS Code Marketplace publishing
- Multi-root workspace support
- VS Code integration test suite (deferred — Vitest unit tests cover YAML round-trip only)

## Decisions

### Extension type: CustomTextEditorProvider with `priority: "option"`

The extension registers a `CustomTextEditorProvider` for `**/manifests/*.yaml`. `priority: "option"` means VS Code prompts the user to choose between the visual editor and the standard text editor on first open — the YAML editor remains the default. This preserves git-diffable YAML as the primary interface while making the visual editor an explicit opt-in.

Alternative considered: companion panel (VS Code webview panel alongside the text editor). Rejected because it requires a separate activation gesture and cannot own the save lifecycle. `CustomTextEditorProvider` owns the document lifecycle and receives `onSave` events.

### YAML round-trip: `yaml` library (Eemeli Aro) with AST mutations

The `yaml` npm package parses YAML into a `Document` AST that preserves comments, blank lines, and scalar styles. Targeted mutations (`setIn`, `addIn`, `deleteIn`) operate on the AST tree and re-serialize without destroying surrounding formatting.

Mutation difficulty tiers and implementation approach:
- **Easy (scalar edits, interface wire)**: Direct `setIn` / `addIn` on known paths
- **Medium (add/delete single node)**: `addIn` for append; `deleteIn` for remove; preserve surrounding YAML block style
- **Hard (cross-reference cleanup on delete)**: When deleting a service, scan all `services[*].dependsOn` arrays for references to that service name and delete them; when deleting a network, scan all `services[*].interfaces` for matching `role` and delete them. These are batched into a single `WorkspaceEdit` applied atomically.

Alternative considered: range-based text replacement using YAML source positions. Rejected because reliable range tracking across multiple nested mutations is fragile and loses the AST's semantic model.

### editInFlight mutex to prevent feedback loops

The extension host sets `editInFlight = true` before applying any `WorkspaceEdit` triggered by a canvas mutation. The `onDidChangeTextDocument` handler skips re-parsing when `editInFlight` is true. After `WorkspaceEdit.apply()` resolves, `editInFlight` is reset to false. User-originated YAML edits (editInFlight = false) trigger a re-parse and canvas re-render.

### Canvas: React Flow with custom node and edge types

React Flow is a purpose-built node-edge canvas library (MIT) with first-class support for custom node types, handles (connection ports), and edge types. The manifest's bipartite structure maps directly: `ServiceNode` with one React Flow handle per interface, `NetworkBusNode` as a wide low-height node, `InterfaceEdge` connecting service handles to network bus handles.

`NetworkBusNode` is positioned as a fixed-width full-canvas-width node. Services are free-form nodes positioned above/between buses. The canvas is not a true swimlane layout — it's a free-form React Flow canvas where network bus nodes happen to be wide and placed at specific y positions.

### Initial auto-layout: dagre with networks pinned

On first open (no sidecar), networks are assigned fixed vertical positions (y = index × 200, sorted by total interface count descending so the most-connected network is at top). Services are submitted to `@dagrejs/dagre` for x/y layout using a DAG where services with `dependsOn` relationships form edges. Network nodes are excluded from dagre (pinned). The result is written immediately to `.vcpe-layout.json`; dagre does not run again unless the sidecar is deleted.

### VcpeBinaryClient: spawnSync with persistent cache

`VcpeBinaryClient.getTypes()` runs `vcpe service types --json` synchronously on first call using Node.js `spawnSync`. The result is cached in extension memory for the session. If the binary is not found, the palette shows an error card with the setting name (`vcpe.binaryPath`). The cache is invalidated and refreshed on extension reload (VS Code reload window).

### Build pipeline: esbuild (extension host) + Vite (webview)

The extension host TypeScript is bundled with `esbuild` (standard for VS Code extensions: fast, produces CJS for the extension host runtime). The webview React app is bundled with Vite (ESM, tree-shaken, dev HMR). Both are triggered by `npm run build` in `extensions/vcpe-visual-editor/`. The `webview/dist/` directory is gitignored and rebuilt on install.

### Network role color assignment

Colors are assigned by computing `hash(role) % palette.length` where `palette` is a fixed 10-color accessible array. The same role always maps to the same color index regardless of manifest content. `NetworkBusNode` headers and all `InterfaceEdge` lines for that role use the same color. The palette is defined as a constant in the webview source.

## Risks / Trade-offs

**[Risk] Multi-site delete leaves dangling cross-references if the scan is incomplete**
→ Mitigation: `serialize.ts` implements `cleanDependsOn(serviceName)` and `cleanInterfaceRefs(networkRole)` helpers that do a full document-tree scan before building the `WorkspaceEdit`. Unit tests in `serialize.test.ts` cover delete-with-cross-reference cases.

**[Risk] editInFlight race: concurrent external YAML edit and canvas mutation**
→ Mitigation: VS Code `WorkspaceEdit` is applied serially; canvas mutations are dispatched to the extension host as a queue. A pending edit is rejected if `editInFlight` is already true — the webview will receive a `DOCUMENT_UPDATED` message shortly after and can re-attempt.

**[Risk] Config opacity: opaque `config` subtree may contain network role strings (BNG `access[].role`)**
→ Mitigation: Network role rename is explicitly blocked in v1. The PropertyPanel's NetworkForm does not expose a role edit field after creation. This constraint is documented in the UI (tooltip on the locked field).

**[Risk] `vcpe` binary not on PATH in GUI VS Code shell (macOS)**
→ Mitigation: `vcpe.binaryPath` setting provides an explicit override. The extension logs a clear message to the Output Channel and shows an actionable error in the palette.

**[Risk] Sidecar accumulates stale positions for deleted services/networks**
→ Mitigation: On every open, `LayoutStore` filters sidecar entries against the current manifest's node IDs. Entries for unknown IDs are dropped from the in-memory model (but not written back to the file until a position change occurs).

**[Risk] dagre produces overlapping nodes for complex manifests on first open**
→ Mitigation: Initial layout is a best-effort starting point; users immediately drag to refine. The sidecar persists their arrangement from that point forward. Dagre only runs once.
