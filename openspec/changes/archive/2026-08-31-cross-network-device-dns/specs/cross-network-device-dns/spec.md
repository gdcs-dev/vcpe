## ADDED Requirements

### Requirement: Planned DHCP identities have deterministic DNS names
For every non-BNG service interface attached to a role served by BNG DHCPv4, the system MUST associate the interface's planned MAC address with a role-qualified name `<service>-<replica>-<role>`. The system MUST associate one primary DHCP interface per instance with `<service>-<replica>` and MUST associate replica one's primary interface with the bare `<service>` name. The primary interface SHALL be the eligible `defaultRoute` interface, or the first eligible interface in manifest order when no eligible interface is the default route. Replica numbers in DNS names SHALL be one-based.

The system MUST reject duplicate generated aliases before applying runtime artifacts. Client-supplied DHCP hostnames MUST NOT override planned identities, and DHCP clients whose MAC addresses do not match a planned identity MUST NOT be published.

#### Scenario: Single-interface Gateway receives canonical aliases
- **WHEN** Gateway replica one commits a DHCPv4 lease on its planned WAN MAC
- **THEN** BNG DNS resolves `gateway`, `gateway-1`, and `gateway-1-wan` to the committed lease address

#### Scenario: Multiple access interfaces remain distinguishable
- **WHEN** an XB10 instance has active DHCPv4 leases on WAN and CM and WAN is its default-route interface
- **THEN** `xb10-1` resolves to the WAN lease
- **THEN** `xb10-1-wan` resolves to the WAN lease and `xb10-1-cm` resolves to the CM lease

#### Scenario: Replicas receive distinct names
- **WHEN** two replicas of a DHCP-attached service commit leases
- **THEN** `<service>-1` and `<service>-2` resolve to their respective primary lease addresses
- **THEN** the bare `<service>` name resolves only to replica one's primary lease

#### Scenario: Unknown client hostname is not published
- **WHEN** an undeclared DHCP client requests a lease with hostname `webpa`
- **THEN** the lease may be granted but BNG DNS does not publish `webpa` from that lease

### Requirement: DNS records follow DHCP lease lifecycle
BNG MUST publish planned aliases only while their corresponding DHCPv4 lease is active. Commit and renewal events MUST atomically create or update aliases to the current leased address. Release and expiry events MUST remove only records matching the event's identity and address. Every successful record-set change MUST become visible to dnsmasq without restarting the BNG container.

#### Scenario: Lease commit publishes the active address
- **WHEN** a planned interface commits a DHCPv4 lease
- **THEN** all aliases mapped to that interface resolve to the committed address after dnsmasq reloads

#### Scenario: Address move replaces stale records
- **WHEN** a planned MAC commits a new address after previously holding another address
- **THEN** its aliases resolve only to the new address

#### Scenario: Delayed release does not delete a renewed lease
- **WHEN** a release event for an old address arrives after the same MAC has committed a new address
- **THEN** the aliases continue resolving to the new address

#### Scenario: Lease expiry removes aliases
- **WHEN** an active planned lease expires without renewal
- **THEN** its aliases no longer resolve from BNG DNS

#### Scenario: Concurrent callbacks preserve complete state
- **WHEN** lease callbacks for different planned interfaces execute concurrently
- **THEN** the resulting DNS host file contains the complete active record set without partial lines or lost updates

### Requirement: Management services use BNG for cross-network lookup
For each non-BNG service attached to the same Podman-managed `mgmt` network as a BNG instance with an allocated IPv4 address, the system SHALL render BNG's management address as the service's first DNS server and the Podman network gateway/Aardvark address as a fallback. BNG itself MUST retain Aardvark as its resolver and MUST forward names outside its local records to that upstream resolver. If no eligible BNG management attachment exists, service DNS configuration MUST remain unchanged.

#### Scenario: WebPA resolves an XB10 access address
- **WHEN** XB10 has an active WAN lease and WebPA shares BNG's management network
- **THEN** resolving `xb10` from WebPA returns the active XB10 WAN lease address

#### Scenario: Management peer resolution is preserved
- **WHEN** a management service uses BNG as its first DNS server and queries an Aardvark management alias such as `event-sink`
- **THEN** BNG forwards the query and returns the management peer address

#### Scenario: External resolution is preserved
- **WHEN** a management service queries a public DNS name not owned by the deployment
- **THEN** BNG forwards the query through its captured upstream resolver and returns the upstream result

#### Scenario: BNG does not forward to itself
- **WHEN** BNG starts on the management network
- **THEN** its own resolver remains Aardvark and its dnsmasq upstream does not contain BNG's management address

#### Scenario: Deployment without BNG is unchanged
- **WHEN** a deployment has management services but no BNG attached to their management network
- **THEN** those services retain their existing Podman DNS configuration

### Requirement: DNS record ownership remains isolated
Management-peer records derived from Aardvark and access-device records derived from DHCP MUST be stored in separately owned runtime host files. Updating either record set MUST NOT remove or rewrite records owned by the other source. DNS records MUST remain scoped to the deployment networks and MUST NOT be published to host-global DNS.

#### Scenario: DHCP commit preserves management records
- **WHEN** WebPA and event-sink management records exist and a Gateway lease commits
- **THEN** the Gateway aliases are added without removing the WebPA or event-sink records

#### Scenario: Management refresh preserves DHCP records
- **WHEN** active Gateway aliases exist and BNG refreshes management aliases from Aardvark
- **THEN** the Gateway aliases remain present and resolvable

#### Scenario: Names do not leak between deployments
- **WHEN** two deployments each run a BNG and a service named `gateway`
- **THEN** each deployment's management services resolve `gateway` only through their own BNG and receive only their own deployment's lease address

### Requirement: WebPA routes access traffic through BNG
When WebPA and BNG share a Podman-managed `mgmt` network, WebPA MUST route each other IPv4 deployment network attached to that BNG through the BNG's allocated management IPv4 address. Routes MUST be installed before WebPA services start. If no eligible BNG or routed IPv4 network exists, WebPA startup MUST remain unchanged.

#### Scenario: Caduceus reaches a Gateway callback
- **WHEN** WebPA and BNG share `mgmt`, Gateway has an active lease on a BNG-connected WAN network, and Caduceus sends a callback to the Gateway lease address
- **THEN** WebPA routes the callback through BNG and reaches the Gateway TCP listener

#### Scenario: Deployment without an eligible BNG is unchanged
- **WHEN** WebPA has no BNG sharing its managed `mgmt` network
- **THEN** no BNG route settings are rendered or installed