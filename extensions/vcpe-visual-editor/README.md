# vCPE Visual Manifest Editor

A VS Code extension for visually building and editing `vcpe.dev/v1` deployment manifests. Drag-and-drop networks and services onto a canvas, draw interface connections between them, and edit all manifest fields — while keeping the YAML file as the canonical source of truth.

## Requirements

- **VS Code** `^1.85.0`
- **vcpe binary** on `PATH`, or the path configured via `vcpe.binaryPath` in settings

## Installation

```bash
make install-extension
```

This builds the extension, packages it as a `.vsix`, and installs it into VS Code using the CLI at `/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code`.

To build the `.vsix` without installing:

```bash
make build-extension
# → extensions/vcpe-visual-editor/vcpe-visual-editor-0.1.0.vsix
```

## Opening a manifest

1. Right-click any file in `manifests/` in the Explorer panel
2. Select **Open With… → vCPE Visual Manifest Editor**

VS Code remembers your choice per file. The visual editor has `priority: "option"` — the standard YAML text editor remains the default.

You can also open the editor from the Command Palette (`⌘⇧P`):

```
vCPE: Open with vCPE Visual Manifest Editor
```

## Canvas overview

```
┌── toolbar ─────────────────────────────────────────────────────────┐
│  [manifest dropdown ▾]  [+ Network]  [+ Service]  [⇢ Dependencies]│
├── type palette ──┬── canvas ────────────────────────┬── properties ┤
│                  │                                  │              │
│  bng             │  ┌─ mgmt ──────────────────────┐ │  Service: bng│
│  gateway         │  │  ●──────────●               │ │  Type: bng   │
│  webpa           │  └─────────────┼───────────────┘ │  Replicas: 1 │
│  event-sink      │                │                  │  Image: ...  │
│  generic-...     │  ┌────────┐    │  ┌──────────┐   │              │
│  xb10            │  │  BNG   │────┘  │ Gateway  │   │  Config:     │
│  oktopus         │  └────────┘       └──────────┘   │  access:     │
│                  │                                  │    - role:.. │
└──────────────────┴──────────────────────────────────┴──────────────┘
```

| Element | Description |
|---|---|
| **NetworkBusNode** | Horizontal lane per network role. Color-coded; shows CIDR, NAT/firewall flags, and driver. |
| **ServiceNode** | Card per service. One connection handle per declared interface, labeled with the network role. |
| **PhysicalNicNode** | Appears for `macvlan`/`ipvlan` networks; represents the host NIC (`driverOptions.parent`). |
| **InterfaceEdge** | Solid colored line from a service interface handle to its network bus. Color matches the network. |
| **DependsOnEdge** | Dashed gray arrow from dependent → dependency (`A → B` = "A needs B"). Toggle with **⇢ Dependencies**. |

## Editing

All canvas mutations are written back to the YAML file in real time via VS Code `WorkspaceEdit`, preserving your comments and formatting.

| Action | How |
|---|---|
| Add a service | Drag a type from the palette onto the canvas. Enter a name when prompted. |
| Add a network | Click **+ Network** in the toolbar. |
| Wire an interface | Drag from a service handle to a network bus. |
| Edit properties | Click any node or edge — the property panel opens on the right. |
| Delete | Select a node and press `Delete`/`Backspace`. Cross-references (dependsOn, interfaces) are cleaned up automatically. |
| Reposition nodes | Drag any node. Positions are saved to `.vcpe-layout.json` alongside the manifest. |
| Toggle dependency arrows | Click **⇢ Dependencies** in the toolbar. |

> **Note:** Network `role` is read-only after creation. To rename a role, delete the network and recreate it. This restriction exists because opaque `config` subtrees (e.g., BNG DHCP config) may embed role names.

## Layout persistence

Node positions are stored in a sidecar file next to each manifest:

```
manifests/
  example.yaml
  example.vcpe-layout.json   ← x/y positions, committed to git
```

The sidecar uses schema `{ version: 1, nodes: { "<kind>:<id>": { x, y } } }`. On first open (no sidecar), positions are computed automatically using `dagre` and then persisted.

## Configuration

| Setting | Default | Description |
|---|---|---|
| `vcpe.binaryPath` | `""` | Absolute path to the `vcpe` binary. If empty, falls back to `PATH` lookup. |

Set via **Settings → Extensions → vCPE Visual Manifest Editor**, or in `settings.json`:

```json
{
  "vcpe.binaryPath": "/usr/local/bin/vcpe"
}
```

The type palette is populated by running `vcpe service types --json`. If the binary cannot be found, the palette shows an error card with the setting name.

## Diagnostics

Open **View → Output → vCPE Visual Manifest Editor** to see extension logs — useful for diagnosing binary discovery, YAML parse errors, or WorkspaceEdit failures.

## Development

```bash
cd extensions/vcpe-visual-editor

# Install dependencies
npm install

# Build extension host + webview bundle
npm run build

# Run YAML round-trip unit tests (Vitest)
npx vitest run --config webview/vite.config.ts

# Type-check extension host only
npx tsc --noEmit

# Package .vsix
npm run package
```

Source layout:

```
extensions/vcpe-visual-editor/
  src/                      ← Extension host (TypeScript, compiled by esbuild)
    extension.ts            ← Activation entry point
    VcpeEditorProvider.ts   ← CustomTextEditorProvider + editInFlight sync
    VcpeBinaryClient.ts     ← vcpe service types --json invocation + cache
    ManifestScanner.ts      ← Workspace manifest discovery
    LayoutStore.ts          ← .vcpe-layout.json read/write
  webview/
    src/                    ← React app (bundled by Vite)
      App.tsx               ← Canvas root, VS Code message bridge
      yaml/
        parse.ts            ← YAML text → ManifestModel
        serialize.ts        ← Canvas mutations → YAML text (all mutation types)
        *.test.ts           ← Vitest tests for the round-trip
      nodes/                ← NetworkBusNode, ServiceNode, PhysicalNicNode
      edges/                ← InterfaceEdge, DependsOnEdge
      panels/               ← TypePalette, PropertyPanel, ManifestDropdown, WelcomeScreen
      layout/               ← autoLayout.ts (dagre-based initial positioning)
      store/                ← Zustand manifest store
      utils/                ← roleColor.ts (stable network → color mapping)
```
