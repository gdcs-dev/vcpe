## 1. Types — shared interfaces

- [x] 1.1 Add `DropTemplateInterface` type to `types.ts`: `{ role, device?, bridge?, sharing?: 'shared'|'unique' }`
- [x] 1.2 Add `DropTemplate` type to `types.ts`: `{ interfaces: DropTemplateInterface[], bridges?: Array<{ name, ipv4? }> }`
- [x] 1.3 Add `PaletteVariant` type to `types.ts`: `{ label, type, description?, interfaces: DropTemplateInterface[], bridges?: Array<{ name, ipv4? }> }`
- [x] 1.4 Add `dropDefaults?: Record<string, DropTemplate>` and `paletteVariants?: PaletteVariant[]` to the webview's message shape (document in `types.ts` or inline in `App.tsx`)

## 2. Extension host — read settings and pass to webview

- [x] 2.1 Add `contributes.configuration` to `package.json` for `vcpe.serviceDropDefaults` (object, keys = type names, values = drop template schema)
- [x] 2.2 Add `contributes.configuration` to `package.json` for `vcpe.paletteVariants` (array of palette variant schema)
- [x] 2.3 In `VcpeEditorProvider.ts` `sendInitialState`, read `vscode.workspace.getConfiguration('vcpe').get('serviceDropDefaults')` and include as `dropDefaults` in `INIT` message
- [x] 2.4 In `VcpeEditorProvider.ts` `sendInitialState`, read `vscode.workspace.getConfiguration('vcpe').get('paletteVariants')` and include as `paletteVariants` in `INIT` message

## 3. Webview store — receive and store settings data

- [x] 3.1 Add `dropDefaults: Record<string, DropTemplate>` and `paletteVariants: PaletteVariant[]` to `manifestStore.ts` state and actions
- [x] 3.2 In `App.tsx` `INIT` handler, call `store.setDropDefaults` and `store.setPaletteVariants` from the message payload

## 4. TypePalette — render variants

- [x] 4.1 Add `paletteVariants: PaletteVariant[]` prop to `TypePalette`
- [x] 4.2 Render variant cards below built-in type cards; variant card dragging sets a JSON payload that the `onDrop` handler can distinguish as a variant (e.g. include `_variant: true` flag)
- [x] 4.3 Variant card displays `label`, `description` (or base type description fallback), and interface role tags from the variant template
- [x] 4.4 Pass `store.paletteVariants` to `TypePalette` in `App.tsx`

## 5. onDrop — resolve and apply drop templates

- [x] 5.1 Extend `onDrop` to detect if the dropped payload is a `PaletteVariant` (check `_variant` flag); if so, extract `type` for name generation and use the variant's template
- [x] 5.2 If not a variant, check `store.dropDefaults[typeDesc.name]`; if present, use it as the template
- [x] 5.3 If no template from either source, derive from `ExpectedRoles()` using the existing fallback (current behavior preserved)
- [x] 5.4 Apply the resolved template: write `device` to interface if set; write `bridge` to interface if set; use `sharing` field instead of `bngRoles` heuristic to determine unique vs shared role
- [x] 5.5 Apply bridge suffix when `sharing === 'unique'` and a service of the same base type already exists (bridge name gets the same `-N` suffix as the role)
- [x] 5.6 Write the resolved `bridges` array (with suffixed names) to the `insertService` mutation

## 6. Verification

- [x] 6.1 With no settings defined, drop behavior is identical to pre-change (existing tests pass, manual check with `example.yaml`)
- [x] 6.2 Define a `vcpe.serviceDropDefaults.gateway` template in `.vscode/settings.json`; drop a gateway; confirm device names and bridges appear in the manifest
- [x] 6.3 Define a `vcpe.paletteVariants` entry; confirm the variant card appears in the palette below built-ins
- [x] 6.4 Drop the variant; confirm `type: gateway` in manifest, correct interfaces, base-type name stem
- [x] 6.5 Drop a second gateway with a template defining unique LAN roles; confirm suffixed role/bridge names
- [x] 6.6 Run `npm run build` in `extensions/vcpe-visual-editor` with no TypeScript errors
