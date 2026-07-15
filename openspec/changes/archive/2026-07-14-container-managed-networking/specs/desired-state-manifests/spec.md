## MODIFIED Requirements

### Requirement: Network driver and options fields in manifest
The manifest `networks[]` schema SHALL accept three optional fields: `driver` (string, default implicit `bridge`), `driverOptions` (map of string key/value pairs), and `ipamDriver` (string). When `driver` is `macvlan` or `ipvlan`, the field `parent` in `driverOptions` SHALL be required. Setting `nat: true` or `firewall: true` on a network with a non-bridge driver SHALL be a validation error. When `ipamDriver` is `none`, the vcpe IPAM SHALL skip CIDR conflict-checking and pool allocation for that network. Interfaces on `ipamDriver: none` networks with explicit `interfaces[].ipv4` values SHALL still have those values resolved and written to `IFACE_*` env vars in compose.env; containers apply these IPs internally via their entrypoint scripts.

#### Scenario: macvlan network declared in manifest
- **WHEN** a manifest declares a network with `driver: macvlan` and `driverOptions: {parent: eth0}`
- **THEN** the manifest passes validation and `vcpe plan` shows the network with driver `macvlan`

#### Scenario: macvlan without parent is rejected
- **WHEN** a manifest declares a network with `driver: macvlan` but no `parent` in `driverOptions`
- **THEN** `vcpe up` fails preflight with an error identifying the missing `parent` option

#### Scenario: nat/firewall rejected on non-bridge driver
- **WHEN** a manifest declares a network with `driver: macvlan` and `nat: true`
- **THEN** `vcpe up` fails preflight with an error stating NAT is not supported for macvlan networks

#### Scenario: default driver is unchanged
- **WHEN** a manifest declares a network with no `driver` field
- **THEN** Podman creates the network using the default bridge driver, identical to existing behavior

#### Scenario: ipamDriver none skips pool allocation
- **WHEN** a manifest declares a network with `ipamDriver: none` and a service interface is attached to it without an explicit `ipv4` address
- **THEN** vcpe IPAM does not allocate a pool address for that interface; `IFACE_*_IPV4` in compose.env is empty unless an explicit `interfaces[].ipv4` was set in the manifest
