## Context

The visual manifest editor's type palette is driven by `vcpe service types --json`, which provides the registered Go type list. When a service is dragged onto the canvas, `onDrop` in `App.tsx` uses `typeDesc.expectedRoles` to determine which interfaces to create. The resulting service has no device names, no bridge assignments, and no sharing semantics — the user must wire everything manually.

There are two settings keys to introduce: `vcpe.serviceDropDefaults` (overrides the default drop template for any registered type) and `vcpe.paletteVariants` (adds new palette entries that reuse an existing Go type but carry a custom drop template).

## Goals / Non-Goals

**Goals:**
- Allow workspace or user settings to define per-type drop templates (interfaces with device names, bridge assignments, and sharing semantics).
- Allow settings to add palette variant entries backed by an existing Go type.
- Preserve full backward compatibility — a missing or empty settings block falls back to the current `ExpectedRoles()` behavior.
- No Go binary changes.

**Non-Goals:**
- Defining new Go service types via settings.
- Persisting drop templates to the manifest sidecar.
- A graphical settings UI (raw JSON editing in `.vscode/settings.json` is sufficient).
- Network-level defaults (`DEFAULT_NETWORKS` CIDRs) — those remain hardcoded in the webview for now.

## Decisions

### D1: Settings read on the extension host, passed to webview in INIT

**Decision**: `VcpeEditorProvider.ts` reads both settings keys via `vscode.workspace.getConfiguration('vcpe')` and includes them in the `INIT` message as `dropDefaults` and `paletteVariants`.

**Rationale**: The extension host has direct access to VS Code settings APIs. Passing resolved values in `INIT` keeps the webview decoupled from the VS Code API — the webview never calls `vscode.workspace.getConfiguration` directly. This also means the webview gets a single authoritative snapshot at open time; live settings changes take effect on the next editor open.

**Alternative considered**: Webview reads settings via the VS Code webview postMessage settings API. Rejected — more complex and unnecessary given that settings changes require re-opening the editor anyway.

### D2: Sharing semantics are explicit on each interface (`shared` | `unique`), not inferred

**Decision**: Each interface in a drop template carries `"sharing": "shared" | "unique"`. The `onDrop` handler uses this directly; the `bngRoles` heuristic introduced earlier serves as the fallback when no template is present.

**Rationale**: The heuristic (infer from whether bng uses the role) is fragile — it breaks for manifests without bng, or for types that share roles with gateway but not bng. Explicit per-interface sharing intent is clear and doesn't require inspecting other services.

### D3: Variant service names use the base type stem

**Decision**: When a palette variant is dropped, `onDrop` computes the auto-generated name from `variant.type` (e.g. `gateway`, `gateway-1`) — not from `variant.label` (e.g. `gateway-wanonly`).

**Rationale**: The manifest `type` field is the Go discriminator. Auto-naming from the base type keeps manifest names consistent and predictable regardless of how many variant labels exist for a given type.

### D4: Variants appear after built-in types in the palette, not interleaved

**Decision**: `TypePalette` renders all built-in types first (sorted as returned by the binary), then all palette variants from settings in declaration order.

**Rationale**: Built-in types are the canonical list; variants are team-specific additions. Visual separation makes the palette predictable across different workspace configs.

### D5: Drop template resolution order

```
onDrop resolution:
  if dropped item is a PaletteVariant  → use variant.template
  else if dropDefaults[type] exists    → use dropDefaults[type]
  else                                 → derive from ExpectedRoles() (current behavior)
```

This means `serviceDropDefaults` does NOT affect variant drops — variants carry their own complete template.

## Risks / Trade-offs

- **Settings drift**: If a Go type renames a role (e.g. `wan` → `erouter`), workspace settings silently mismatch. Mitigation: the webview falls back gracefully — unknown roles are just created as-is. A future validation step could warn.
- **Schema complexity**: The JSON schema for `vcpe.serviceDropDefaults` and `vcpe.paletteVariants` is nested enough that the VS Code settings UI will show raw JSON editing. For a developer tool this is acceptable.
- **Live settings not reflected**: Drop template changes in settings don't apply until the editor is re-opened. This is consistent with how VS Code handles most extension configuration.
