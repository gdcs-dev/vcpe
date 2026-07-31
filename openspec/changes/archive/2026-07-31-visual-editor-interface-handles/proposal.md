## Why

The visual manifest editor currently connects service nodes to each other through bridge handles, meaning edges land on a bridge group header rather than on the physical interface that actually carries the connection. This misrepresents the topology — bridges are internal switching constructs, not external ports. Connecting at the interface level makes the canvas read as a physical wiring diagram.

## What Changes

- Each interface row on a `ServiceNode` gets connection handles on both the left and right sides (`iface-{role}` right, `iface-{role}-left` left), regardless of whether it belongs to a bridge.
- Bridge group header rows lose their connection handle; they remain as cosmetic grouping elements showing the bridge name, IP, and member interfaces.
- Bridge member interface rows are promoted to full interface styling: same `10×10` colored role handle as non-bridged interfaces, with indentation retained to signal bridge membership.
- Auto-built edges continue to wire right-to-right (`iface-{role}`); left handles are available for manual reconnection when users arrange nodes horizontally.
- The `resolveHandle` helper and `svcBridgeForRole` map in `App.tsx` are deleted — handle IDs are always `iface-{role}`.

## Capabilities

### New Capabilities

- `visual-editor-interface-handles`: Per-interface bidirectional connection handles on all service node interface rows, including bridge members; bridge headers as cosmetic-only grouping elements.

### Modified Capabilities

- `visual-manifest-editor`: The `ServiceNode` visual contract changes — bridge member rows gain handles and promoted styling; bridge header rows lose their handle. The `InterfaceEdge` target semantics shift from bridge-level to interface-level.

## Impact

- `extensions/vcpe-visual-editor/webview/src/nodes/ServiceNode.tsx`: Add left+right handles to all interface rows (including bridge members); remove handle from bridge header rows; add `position: relative` to member row divs.
- `extensions/vcpe-visual-editor/webview/src/App.tsx`: Delete `svcBridgeForRole`, delete `resolveHandle`, replace all `resolveHandle(...)` calls with `iface-${role}`.
- No YAML schema changes; no extension host changes; no sidecar layout format changes.
