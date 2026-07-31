## Context

The `ServiceNode` React component renders service interfaces in two categories: non-bridged (each with a right-side React Flow `Handle`) and bridged (grouped under a bridge header that holds the handle; member rows are visual-only with no handle). Edge-building in `App.tsx` uses a `resolveHandle` helper that maps `svcName:role` → bridge name to route edges to the bridge-level handle when applicable.

This model misrepresents physical topology: a bridge is an internal switching construct inside the container; the physical interfaces are the actual external connection points. The visual should reflect where a cable would physically terminate.

## Goals / Non-Goals

**Goals:**
- Every interface row (bridged or not) has a left-side and right-side React Flow handle.
- Bridge header rows become cosmetic grouping elements — no handle, visual styling preserved.
- Edge auto-routing targets the right-side handle (`iface-{role}`) on all interfaces consistently.
- Left-side handles (`iface-{role}-left`) are passive: available for manual reconnect after user rearrangement, never targeted by auto-built edges.
- `resolveHandle` and `svcBridgeForRole` are deleted; handle ID computation is trivially `iface-${role}`.

**Non-Goals:**
- Position-aware handle selection (auto-picking left vs. right based on node layout).
- Changes to YAML schema, sidecar format, or extension host code.
- Reconnect mutation behavior (edge drag-to-rewire is unchanged).

## Decisions

### D1: Interface row as the connection point, not the bridge

**Decision**: Handles live on interface rows; bridge headers are cosmetic.

**Rationale**: A Linux bridge (`brlan0`) is an internal kernel object that the container manages. External services connect to the physical interface (`eth0`, `eth1`) that is enslaved to the bridge, not to the bridge itself. Representing the bridge as the connection anchor is an abstraction leak.

**Alternative considered**: Keep bridge handle AND add interface handles (both exist). Rejected — creates ambiguity about which handle to use and no clear semantic win.

### D2: Passive left handles (Option A over Option B)

**Decision**: Left handles exist statically; auto-built edges always use the right handle. No position-aware selection.

**Rationale**: Position-aware handle selection requires the edge builder to inspect node layout at edge-build time. Node positions can change (drag) after edges are built, making dynamic selection unreliable without a layout-change listener. The passive approach is stable and gives users manual flexibility without complexity.

**Alternative considered**: Position-aware handle selection at build time. Rejected — fragile when users drag nodes after initial layout.

### D3: Bridge members promoted to full interface styling

**Decision**: Bridge member handles use the same `10×10` colored dot as non-bridged interface handles. Indentation (`padding-left: 22px`) communicates bridge membership instead.

**Rationale**: If a row is a connection point, it should look like one. Dimming the handle would suggest the interface is less capable, which is wrong once it has a handle.

## Risks / Trade-offs

- **Existing sidecar layouts**: Edge IDs are built as `net-{role}-{a}-{b}` and do not encode handle IDs, so sidecar layout files are unaffected. Edges are rebuilt from scratch on every canvas load — no stale handle references.
- **Reconnect UX with two handles per row**: React Flow `ConnectionMode.Loose` treats all handles as source/target. With both left and right handles on a row, a user dragging an edge endpoint near the node has two nearby snap targets. If this proves noisy in practice, handle `isConnectable` could be selectively disabled, but this is not a concern for the initial change.
- **Bridge header loses interactivity**: Users who relied on clicking the bridge handle to initiate a drag will find the header inert. The interface row handles below it serve the same purpose — no capability is lost, only the anchor point moves.
