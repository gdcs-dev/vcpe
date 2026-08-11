## Why

When a service is dragged from the type palette onto the canvas, it is created with bare role names and no device names, bridge assignments, or network sharing semantics. This means every dropped service needs manual wiring before it reflects a real topology. Teams working with fixed service configurations (e.g. a gateway always using `erouter0` on WAN with two bridged LAN ports) have no way to encode those defaults — they re-enter them every time. There is also no way to define variant palette entries (e.g. "Gateway — WAN only") without adding a new Go service type.

## What Changes

- Add two VS Code extension settings: `vcpe.serviceDropDefaults` and `vcpe.paletteVariants`.
- `vcpe.serviceDropDefaults` maps a service type name to a **drop template** — an ordered list of interface definitions (role, optional device name, optional bridge name, sharing semantics `shared`|`unique`) and an optional list of bridge declarations.
- `vcpe.paletteVariants` is an array of **palette variant** entries, each naming a display label, a base service type, a description, and a drop template. Variants appear in the type palette alongside built-in types and produce services with `type` set to the base type.
- The extension host reads both settings at editor activation and passes them to the webview in the `INIT` message.
- The webview `onDrop` handler applies a resolved template: variant template → `serviceDropDefaults` override → `ExpectedRoles()` fallback (current behavior).
- The `TypePalette` sidebar renders both built-in types and palette variants; variants are appended after built-ins.
- Service auto-naming for variants uses the base type name stem (`gateway`, `gateway-1`, `gateway-2`, …) — not the variant label.
- No changes to the Go control plane, type registry, or manifest schema.

## Capabilities

### New Capabilities
- `canvas-service-drop-templates`: Drop template schema, settings keys, extension-host reading, webview merge logic, and `TypePalette` rendering of variants.

### Modified Capabilities
- `visual-manifest-editor`: The `Type palette from vcpe binary` requirement changes — the palette now renders variants from settings in addition to types from the binary.

## Impact

- `extensions/vcpe-visual-editor/package.json`: Add `contributes.configuration` for both settings keys with JSON schemas.
- `extensions/vcpe-visual-editor/src/VcpeEditorProvider.ts`: Read settings and include in `INIT` message.
- `extensions/vcpe-visual-editor/webview/src/types.ts`: Add `DropTemplate`, `DropTemplateInterface`, `PaletteVariant` interfaces; extend `ServiceTypeDescriptor`.
- `extensions/vcpe-visual-editor/webview/src/panels/TypePalette.tsx`: Render variants from settings.
- `extensions/vcpe-visual-editor/webview/src/App.tsx`: Extend `onDrop` to resolve and apply drop templates; extend store/INIT path for variants.
- No Go changes.
