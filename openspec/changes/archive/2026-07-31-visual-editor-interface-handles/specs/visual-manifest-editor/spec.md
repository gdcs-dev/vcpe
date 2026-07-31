## MODIFIED Requirements

### Requirement: Visual canvas node types
The canvas SHALL render the following node types derived from the manifest:
- `NetworkBusNode`: a wide horizontal lane for each `spec.networks[]` entry, labeled with `role`, CIDR(s), `nat`, `firewall`, and `driver` flags
- `ServiceNode`: a card for each `spec.services[]` entry. Each declared interface SHALL render as a row with a React Flow connection handle on both the right side (`iface-{role}`) and the left side (`iface-{role}-left`). Bridge group headers SHALL render with `▣` icon, bridge name, IP, and purple styling, but SHALL NOT have a connection handle. Bridge member interface rows SHALL use full interface handle styling (same `10×10` colored handle as non-bridged interfaces) with left-side indentation to signal bridge membership. The `ServiceNode` SHALL be labeled with `name`, `type`, and `replicas`.
- `PhysicalNicNode`: one node per distinct `driverOptions.parent` value across all macvlan/ipvlan networks, representing the host physical NIC; macvlan networks are visually anchored to their parent NIC node
- `InterfaceEdge`: a solid colored line connecting a `ServiceNode` interface handle to its peer service's interface handle; color is determined by hashing the network role against a fixed 10-color accessible palette. Auto-built edges SHALL always target `iface-{role}` (right-side handle).
- `DependsOnEdge`: a dashed gray arrow from dependent service to dependency (A → B = "A needs B"); visibility is toggled via a canvas toolbar button that defaults to visible

#### Scenario: Network rendered as horizontal bus
- **WHEN** a manifest has a network with `role: wan` and `ipv4.cidr: 10.7.200.0/24`
- **THEN** the canvas shows a `NetworkBusNode` labeled "wan" with the CIDR and NAT/firewall flags

#### Scenario: macvlan network shows physical NIC node
- **WHEN** a manifest has a network with `driver: macvlan` and `driverOptions.parent: eth0`
- **THEN** the canvas shows a `PhysicalNicNode` labeled "eth0" that the macvlan `NetworkBusNode` connects to

#### Scenario: Interface edge colored by network role
- **WHEN** a service interface references network role "wan"
- **THEN** the `InterfaceEdge` connecting that service to its peer uses the same color assigned to the "wan" role throughout the canvas

#### Scenario: Bridge member interface connects at interface row, not bridge header
- **WHEN** a service has `role: lan-p1` assigned to `bridge: brlan0` and a client service also has `role: lan-p1`
- **THEN** the `InterfaceEdge` terminates at the `iface-lan-p1` handle on the `lan-p1` member row inside the `brlan0` group, not at the `brlan0` bridge header

#### Scenario: Bridge header is cosmetic only
- **WHEN** a service has a bridge group `brlan0` with member interface `lan-p1`
- **THEN** the `brlan0` header row displays with `▣` icon, purple color, and the bridge IP, and cannot be used as a connection drag source or target

#### Scenario: DependsOn edge toggleable
- **WHEN** the user clicks the "⇢ Dependencies" toolbar button
- **THEN** all `DependsOnEdge` arrows are hidden; clicking again restores them
