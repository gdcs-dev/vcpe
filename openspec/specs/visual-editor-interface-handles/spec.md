## Purpose
Define the per-interface bidirectional connection handle behaviour for `ServiceNode` in the visual manifest editor, including handle placement on bridge member rows and the cosmetic-only treatment of bridge group headers.

## Requirements

### Requirement: Interface rows are the connection points on service nodes
Every interface row on a `ServiceNode` SHALL have a React Flow `Handle` on both the right side (ID `iface-{role}`) and the left side (ID `iface-{role}-left`). This applies to all interfaces: non-bridged interfaces, bridge member interfaces, and the placeholder "drag to connect" row. Bridge header rows SHALL NOT have a handle of any kind.

#### Scenario: Non-bridged interface has both handles
- **WHEN** a service has an interface with `role: wan` and no `bridge` assignment
- **THEN** the canvas renders an `iface-wan` handle on the right edge of that row and an `iface-wan-left` handle on the left edge

#### Scenario: Bridge member interface has both handles
- **WHEN** a service has an interface with `role: lan-p1` assigned to `bridge: brlan0`
- **THEN** the canvas renders an `iface-lan-p1` handle on the right edge of that member row and an `iface-lan-p1-left` handle on the left edge

#### Scenario: Bridge header row has no handle
- **WHEN** a service has a bridge declaration named `brlan0` with member interfaces
- **THEN** the `brlan0` group header row renders its name, IP, and `▣` icon with purple styling, and has no React Flow connection handle

### Requirement: Bridge member rows use full interface handle styling
Bridge member interface rows SHALL render their connection handles at `10×10` pixels with the role color and role-colored border, identical to non-bridged interface handles. Row indentation (`padding-left: 22px`) SHALL be the sole visual indicator of bridge membership.

#### Scenario: Bridge member handle matches non-bridged handle size and color
- **WHEN** a bridge member interface has `role: lan-p1`
- **THEN** its right-side handle is `10×10` pixels, colored with `roleColor('lan-p1')`, with a matching role-colored semi-transparent border — identical in appearance to a non-bridged `lan-p1` handle

### Requirement: Auto-built interface edges always target the right-side handle
The edge builder in `App.tsx` SHALL always use `iface-{role}` as the handle ID for both `sourceHandle` and `targetHandle` on interface edges, regardless of whether the interface belongs to a bridge. The left-side handle (`iface-{role}-left`) is available for manual reconnection by the user and SHALL NOT be targeted by auto-built edges.

#### Scenario: Edge to bridge member uses interface handle
- **WHEN** the `client` service has `role: lan-p2` and the `gateway` service has `role: lan-p2` assigned to `bridge: brlan1`
- **THEN** the auto-built interface edge uses `sourceHandle: iface-lan-p2` and `targetHandle: iface-lan-p2` (not `iface-bridge-brlan1`)

#### Scenario: Left handle available for manual reconnect
- **WHEN** the user drags an edge endpoint from a right-side handle toward the left side of a service node
- **THEN** the `iface-{role}-left` handles on the left edge of that node act as valid drop targets
