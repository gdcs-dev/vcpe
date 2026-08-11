## Purpose
Define VS Code extension settings and runtime behavior for configurable service drop templates and palette variants in the visual manifest editor.

## Requirements

### Requirement: vcpe.serviceDropDefaults extension setting
The extension SHALL contribute a VS Code setting `vcpe.serviceDropDefaults` of type `object`. Its keys SHALL be service type names (e.g. `"gateway"`); its values SHALL be drop template objects. A drop template SHALL contain an `interfaces` array of interface definitions and an optional `bridges` array. Each interface definition SHALL contain: `role` (string, required), `device` (string, optional), `bridge` (string, optional), `sharing` (string enum `"shared"` | `"unique"`, optional — defaults to `"shared"` if omitted). Each bridge declaration SHALL contain `name` (string, required) and `ipv4` (string, optional). The setting SHALL be available at both workspace and user scope, with workspace taking precedence.

#### Scenario: Workspace setting defines gateway drop template
- **WHEN** `.vscode/settings.json` contains `"vcpe.serviceDropDefaults": { "gateway": { "interfaces": [...], "bridges": [...] } }`
- **THEN** the visual editor uses that template when a gateway is dragged from the palette

#### Scenario: Missing setting falls back to ExpectedRoles
- **WHEN** `vcpe.serviceDropDefaults` is empty or does not define a key for the dropped type
- **THEN** `onDrop` derives the interface list from `ExpectedRoles()` using the existing fallback behavior

### Requirement: vcpe.paletteVariants extension setting
The extension SHALL contribute a VS Code setting `vcpe.paletteVariants` of type `array`. Each entry SHALL be a palette variant object with: `label` (string, required — display name in palette), `type` (string, required — must match a registered Go service type), `description` (string, optional), and the same `interfaces` and `bridges` fields as a drop template. The setting SHALL be available at both workspace and user scope.

#### Scenario: Workspace setting defines a gateway-wanonly variant
- **WHEN** `vcpe.paletteVariants` contains `{ "label": "Gateway (WAN only)", "type": "gateway", "interfaces": [{ "role": "wan", "device": "erouter0", "sharing": "shared" }] }`
- **THEN** the palette sidebar shows a "Gateway (WAN only)" entry below the built-in types

#### Scenario: Dropping a variant creates a service with the base type
- **WHEN** the user drags "Gateway (WAN only)" onto the canvas
- **THEN** the created service has `type: gateway` in the manifest, uses the variant's interface template, and is auto-named using the `gateway` stem (`gateway`, `gateway-1`, etc.)

### Requirement: Drop templates applied during canvas drop
When an item is dropped from the type palette, the `onDrop` handler SHALL resolve a drop template using the following precedence: (1) if the dropped item is a palette variant, use the variant's template; (2) else if `vcpe.serviceDropDefaults` defines a template for that type, use it; (3) else derive from `ExpectedRoles()`. The resolved template SHALL determine interface roles, device names, bridge assignments, and sharing semantics. Interfaces marked `"unique"` SHALL have their role suffixed when a service of the same base type already exists (e.g. `lan-p1` → `lan-p1-2`). Interfaces marked `"shared"` SHALL always use the base role name and reuse an existing network of that role.

#### Scenario: Drop template supplies device names
- **WHEN** a drop template defines `{ "role": "wan", "device": "erouter0" }`
- **THEN** the created service has `interfaces: [{ role: wan, device: erouter0 }]` in the manifest

#### Scenario: Drop template supplies bridge assignments
- **WHEN** a drop template defines `{ "role": "lan-p1", "bridge": "brlan0", "sharing": "unique" }` and a `bridges: [{ "name": "brlan0", "ipv4": "10.0.0.1/24" }]` entry
- **THEN** the service is created with `interfaces: [{ role: lan-p1-N, device: ..., bridge: brlanN }]` and a `bridges:` block using the suffixed bridge name

#### Scenario: Unique interfaces suffixed on second drop
- **WHEN** a `gateway` already exists and a second gateway (or variant) is dropped
- **THEN** interfaces with `"sharing": "unique"` get role and bridge names suffixed with `-2`; interfaces with `"sharing": "shared"` use the original role names unchanged

### Requirement: Palette renders variants from settings
The `TypePalette` sidebar SHALL render all palette variants from `vcpe.paletteVariants` after the built-in type cards. Variant cards SHALL display the variant `label` as the card title, the variant `description` (if set, else the base type description), and the interface roles from the variant template as role tags. Variant cards SHALL be draggable in the same way as built-in type cards.

#### Scenario: Variant card appears in palette
- **WHEN** `vcpe.paletteVariants` defines a "Gateway (WAN only)" entry
- **THEN** the palette shows a draggable card labeled "Gateway (WAN only)" below the built-in type cards

#### Scenario: No variants shows no extra cards
- **WHEN** `vcpe.paletteVariants` is empty or absent
- **THEN** the palette renders only the built-in types, unchanged from current behavior
