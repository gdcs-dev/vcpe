## MODIFIED Requirements

### Requirement: Type palette from vcpe binary
The canvas SHALL render a type palette sidebar populated by the output of `vcpe service types --json`. Each palette entry SHALL display the service type name and description. Below the built-in type entries, the palette SHALL also render palette variant cards populated from the `vcpe.paletteVariants` VS Code setting. Variant cards display the variant `label`, `description` (or base type description if absent), and interface roles from the variant template. Dragging a built-in type card SHALL create a `ServiceNode` using the resolved drop template (settings override → `ExpectedRoles()` fallback). Dragging a variant card SHALL create a `ServiceNode` using the variant's template with `type` set to the variant's base type. If the `vcpe` binary cannot be found, the palette SHALL display an actionable error referencing the `vcpe.binaryPath` VS Code setting.

#### Scenario: Palette populated from binary
- **WHEN** the visual editor activates and the `vcpe` binary is available
- **THEN** the type palette lists all registered service types with their names and descriptions

#### Scenario: Palette shows variants from settings
- **WHEN** `vcpe.paletteVariants` defines one or more entries
- **THEN** the palette shows variant cards below the built-in type cards

#### Scenario: Drag service from palette creates service node with drop template
- **WHEN** the user drags a service type from the palette
- **THEN** a new `ServiceNode` is created using the resolved drop template: device names and bridge assignments from the template are written to the manifest, interfaces marked `shared` reuse existing networks, interfaces marked `unique` get suffixed role names if the type already exists

#### Scenario: Drag variant from palette creates service node with base type
- **WHEN** the user drags a palette variant card
- **THEN** a new `ServiceNode` is created with `type` set to the variant's base type and interfaces from the variant template; service auto-naming uses the base type stem

#### Scenario: Binary not found shows actionable error
- **WHEN** `vcpe service types --json` fails because the binary is not found
- **THEN** the palette displays an error card instructing the user to set `vcpe.binaryPath` in VS Code settings
