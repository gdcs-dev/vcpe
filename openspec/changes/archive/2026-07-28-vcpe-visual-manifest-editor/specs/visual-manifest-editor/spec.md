## ADDED Requirements

### Requirement: VS Code custom editor for vcpe.dev/v1 manifests
The system SHALL provide a VS Code extension (`gdcs-dev.vcpe-visual-editor`) that registers a `CustomTextEditorProvider` for files matching `**/manifests/*.yaml` with `priority: "option"`. The extension SHALL target a minimum VS Code engine version of `^1.85.0`. The editor SHALL open the YAML file in a sandboxed webview rendering a visual canvas. When opened without a specific file (e.g., via command palette), the editor SHALL display a welcome screen listing discoverable workspace manifests and a "New Manifest" action.

#### Scenario: Editor available as an option for manifest files
- **WHEN** a user opens a file matching `**/manifests/*.yaml`
- **THEN** VS Code offers the vCPE Visual Manifest Editor as an available editor alongside the default text editor

#### Scenario: Welcome screen on file-less activation
- **WHEN** the visual editor is activated without a specific manifest file selected
- **THEN** the canvas displays a welcome screen listing all discovered `vcpe.dev/v1` manifests in the workspace and a "New Manifest" button

### Requirement: Bidirectional YAML synchronization
The extension SHALL maintain bidirectional sync between the YAML file and the visual canvas. Canvas mutations SHALL be written back to the YAML file via `WorkspaceEdit` operations using the `yaml` library's AST to preserve comments and formatting. External YAML edits (text editor changes) SHALL trigger canvas re-renders within one VS Code `onDidChangeTextDocument` event cycle. An `editInFlight` flag SHALL prevent feedback loops when the extension itself applies a `WorkspaceEdit`.

#### Scenario: Canvas mutation writes YAML
- **WHEN** the user modifies the canvas (e.g., changes a service replica count in the property panel)
- **THEN** the corresponding field in the YAML file is updated via WorkspaceEdit, preserving surrounding comments and formatting

#### Scenario: External YAML edit re-renders canvas
- **WHEN** the user edits the manifest YAML in a text editor while the visual editor is open
- **THEN** the canvas re-renders to reflect the new manifest state within one change event cycle

#### Scenario: editInFlight prevents feedback loop
- **WHEN** the extension applies a WorkspaceEdit triggered by a canvas mutation
- **THEN** the resulting `onDidChangeTextDocument` event does not trigger a canvas re-render

### Requirement: Invalid YAML handling
When the manifest YAML cannot be parsed, the canvas SHALL display an error overlay showing the parse error with an "Open in Text Editor" button. The canvas SHALL be frozen (no editing) until the YAML is valid. The canvas SHALL automatically recover and re-render when the YAML becomes valid again.

#### Scenario: Parse error shows error overlay
- **WHEN** the manifest YAML contains a syntax or schema error
- **THEN** the canvas displays a non-interactive error overlay with the error message and line number, and an "Open in Text Editor" button

#### Scenario: Canvas auto-recovers on fix
- **WHEN** a previously invalid manifest YAML is corrected in the text editor
- **THEN** the canvas automatically re-renders the corrected topology without requiring user action

### Requirement: Visual canvas node types
The canvas SHALL render the following node types derived from the manifest:
- `NetworkBusNode`: a wide horizontal lane for each `spec.networks[]` entry, labeled with `role`, CIDR(s), `nat`, `firewall`, and `driver` flags
- `ServiceNode`: a card for each `spec.services[]` entry with one React Flow connection handle per declared interface, labeled with `name`, `type`, and `replicas`
- `PhysicalNicNode`: one node per distinct `driverOptions.parent` value across all macvlan/ipvlan networks, representing the host physical NIC; macvlan networks are visually anchored to their parent NIC node
- `InterfaceEdge`: a solid colored line connecting a `ServiceNode` interface handle to its `NetworkBusNode`; color is determined by hashing the network role against a fixed 10-color accessible palette
- `DependsOnEdge`: a dashed gray arrow from dependent service to dependency (A → B = "A needs B"); visibility is toggled via a canvas toolbar button that defaults to visible

#### Scenario: Network rendered as horizontal bus
- **WHEN** a manifest has a network with `role: wan` and `ipv4.cidr: 10.7.200.0/24`
- **THEN** the canvas shows a `NetworkBusNode` labeled "wan" with the CIDR and NAT/firewall flags

#### Scenario: macvlan network shows physical NIC node
- **WHEN** a manifest has a network with `driver: macvlan` and `driverOptions.parent: eth0`
- **THEN** the canvas shows a `PhysicalNicNode` labeled "eth0" that the macvlan `NetworkBusNode` connects to

#### Scenario: Interface edge colored by network role
- **WHEN** a service interface references network role "wan"
- **THEN** the `InterfaceEdge` connecting that service to the WAN bus uses the same color assigned to the "wan" role throughout the canvas

#### Scenario: DependsOn edge toggleable
- **WHEN** the user clicks the "⇢ Dependencies" toolbar button
- **THEN** all `DependsOnEdge` arrows are hidden; clicking again restores them

### Requirement: Type palette from vcpe binary
The canvas SHALL render a type palette sidebar populated by the output of `vcpe service types --json`. Each palette entry SHALL display the service type name and description. Dragging a palette entry onto the canvas SHALL create a new `ServiceNode` with the type's `defaultImage` pre-filled and stub interfaces pre-created for each `expectedRoles` entry with `required: true`. If the `vcpe` binary cannot be found, the palette SHALL display an actionable error referencing the `vcpe.binaryPath` VS Code setting.

#### Scenario: Palette populated from binary
- **WHEN** the visual editor activates and the `vcpe` binary is available
- **THEN** the type palette lists all registered service types with their names and descriptions

#### Scenario: Drag service from palette creates service node
- **WHEN** the user drags a service type from the palette onto the canvas
- **THEN** a new `ServiceNode` is created with the default image pre-filled and required interfaces pre-wired as stub connections

#### Scenario: Binary not found shows actionable error
- **WHEN** `vcpe service types --json` fails because the binary is not found
- **THEN** the palette displays an error card instructing the user to set `vcpe.binaryPath` in VS Code settings

### Requirement: Property panel for all manifest fields
The canvas SHALL provide a property panel that renders editing forms for the selected element. Clicking a `NetworkBusNode` shows a `NetworkForm` for all `Network` fields. Clicking a `ServiceNode` shows a `ServiceForm` for all `Service` fields, including a raw YAML text editor widget for the `config` subtree. Clicking an `InterfaceEdge` shows an `InterfaceForm` for all `Interface` fields. Clicking the canvas background shows a "Deployment Settings" drawer with `metadata` fields, `maxReplicasPerService`, `maxActiveDeployments`, and a `spec.secrets[]` table.

#### Scenario: NetworkForm edits network fields
- **WHEN** the user clicks a `NetworkBusNode` and modifies the CIDR in the property panel
- **THEN** the corresponding `networks[i].ipv4.cidr` field in the YAML is updated via WorkspaceEdit

#### Scenario: ServiceForm shows raw YAML config editor
- **WHEN** the user clicks a `ServiceNode` and edits the `config` subtree in the raw YAML editor widget
- **THEN** the `services[i].config` subtree in the YAML is updated verbatim

#### Scenario: Deployment Settings drawer
- **WHEN** the user clicks the canvas background
- **THEN** the property panel shows metadata fields, spec-level scalars, and a secrets table with add/edit/delete actions

### Requirement: Multi-site delete with cross-reference cleanup
When a service or network is deleted from the canvas, the extension SHALL remove all cross-references to the deleted element in a single atomic `WorkspaceEdit`. Deleting a service SHALL remove all `dependsOn` references to that service in other services. Deleting a network SHALL remove all `interfaces` entries referencing that network role in all services.

#### Scenario: Delete service cleans dependsOn references
- **WHEN** the user deletes a `ServiceNode` that is referenced in another service's `dependsOn`
- **THEN** a single WorkspaceEdit removes the service and removes its name from all `dependsOn` arrays in the manifest

#### Scenario: Delete network cleans interface references
- **WHEN** the user deletes a `NetworkBusNode`
- **THEN** a single WorkspaceEdit removes the network and removes all `interfaces` entries with that network's role from every service in the manifest

### Requirement: Network role is immutable after creation
Network `role` SHALL be a write-once field in the visual editor. The `NetworkForm` SHALL display the role as a read-only label after a network is created. A tooltip SHALL explain that role rename is not supported in v1 and that users must delete and recreate the network to change the role.

#### Scenario: Network role field is read-only in property panel
- **WHEN** the user opens the `NetworkForm` for an existing network
- **THEN** the `role` field is displayed as a non-editable label with a tooltip explaining the constraint

### Requirement: Manifest dropdown and new manifest creation
The visual editor toolbar SHALL provide a manifest dropdown listing all workspace manifests discovered by scanning `**/manifests/*.yaml` for `apiVersion: vcpe.dev/v1`. Selecting a manifest from the dropdown SHALL open that file in the visual editor. A "New Manifest" action SHALL prompt for a deployment name, create `manifests/<name>.yaml` with a minimal valid skeleton (`apiVersion`, `kind`, `metadata.name`, empty `networks: []`, `services: []`), and open it in the visual editor.

#### Scenario: Dropdown lists workspace manifests
- **WHEN** the user opens the manifest dropdown in the toolbar
- **THEN** all files matching `**/manifests/*.yaml` with `apiVersion: vcpe.dev/v1` are listed

#### Scenario: New manifest creates minimal skeleton
- **WHEN** the user selects "New Manifest", enters name "lab-test", and confirms
- **THEN** `manifests/lab-test.yaml` is created with a valid minimal skeleton and the visual editor opens on it

### Requirement: Extension observability via Output Channel
The extension SHALL create a VS Code Output Channel named "vCPE Visual Manifest Editor" at activation. The extension host SHALL write informational log lines to this channel for binary invocation, manifest parse events, and WorkspaceEdit applications. The channel SHALL be disposed when the extension deactivates.

#### Scenario: Output channel available
- **WHEN** the vCPE Visual Manifest Editor extension is active
- **THEN** a "vCPE Visual Manifest Editor" channel is visible in the VS Code Output panel

### Requirement: Fully self-contained webview
The webview bundle SHALL contain all assets (scripts, styles, fonts, icons) with no runtime external network requests. The webview Content Security Policy SHALL use VS Code nonces for scripts and restrict all resource origins to the webview's own bundle. All npm dependencies (React Flow, Lucide React, etc.) SHALL be bundled by Vite at build time.

#### Scenario: Webview loads without network access
- **WHEN** the visual editor opens in an air-gapped environment
- **THEN** the canvas renders fully without any external resource requests

### Requirement: Extension packaging and distribution
The extension SHALL be packaged as a `.vsix` file using `@vscode/vsce`. The root `Makefile` SHALL provide `build-extension` and `install-extension` targets. `build-extension` SHALL run `npm install` and `npm run build` in `extensions/vcpe-visual-editor/`, then `vsce package`. `install-extension` SHALL additionally run `code --install-extension` with the produced `.vsix`. The webview `dist/` directory SHALL be gitignored.

#### Scenario: build-extension produces installable artifact
- **WHEN** `make build-extension` is run from the repo root
- **THEN** a `.vsix` file is produced in `extensions/vcpe-visual-editor/`

#### Scenario: install-extension installs to VS Code
- **WHEN** `make install-extension` is run
- **THEN** the extension is installed to the local VS Code instance
