## MODIFIED Requirements

### Requirement: Canonical interface environment contract
The system SHALL emit a canonical environment contract consumed by curated compose files and the generic-container init entrypoint: for each interface role, `IFACE_<ROLE>_NETWORK`, `IFACE_<ROLE>_DEVICE`, `IFACE_<ROLE>_MAC`, `IFACE_<ROLE>_IPV4`, `IFACE_<ROLE>_IPV6`, `IFACE_<ROLE>_GATEWAY4`, `IFACE_<ROLE>_GATEWAY6`, and `IFACE_<ROLE>_ADDRESSING` (the role upper-cased with `-` replaced by `_`), plus `DEPLOYMENT_NAME`, `SERVICE_NAME`, and `IMAGE`. `IFACE_<ROLE>_ADDRESSING` SHALL be either `dhcp` or `static`, mirroring the interface's resolved `addressing` value, and SHALL be emitted for every interface including bridge-enslaved ones (where it has no runtime effect). For interfaces where `defaultRoute` is true, the system SHALL additionally emit `IFACE_<ROLE>_DEFAULT_ROUTE=1`. The `DEFAULT_ROUTE` variable SHALL be omitted entirely for interfaces where `defaultRoute` is false or unset. For interfaces whose network is Podman-managed (any `ipamDriver` other than `none`), the system SHALL additionally emit `IFACE_<ROLE>_NETWORK_MANAGED=1`; this variable SHALL be omitted entirely for interfaces on a container-managed (`ipamDriver: none`) network.

#### Scenario: Interface variables are emitted per role
- **WHEN** a service declares an interface with role `lan-p1`
- **THEN** the rendered environment includes `IFACE_LAN_P1_NETWORK`, `IFACE_LAN_P1_DEVICE`, `IFACE_LAN_P1_MAC`, `IFACE_LAN_P1_ADDRESSING`, and the corresponding address and gateway variables

#### Scenario: Addressing variable reflects resolved value
- **WHEN** a service declares an interface with role `wan` and `addressing: static`
- **THEN** the rendered environment includes `IFACE_WAN_ADDRESSING=static`

#### Scenario: Addressing variable defaults to dhcp
- **WHEN** a service declares an interface with role `wan` and omits `addressing`
- **THEN** the rendered environment includes `IFACE_WAN_ADDRESSING=dhcp`

#### Scenario: DEFAULT_ROUTE is emitted for the default-route interface
- **WHEN** a service declares an interface with role `lan-p1` and `defaultRoute: true`
- **THEN** the rendered environment includes `IFACE_LAN_P1_DEFAULT_ROUTE=1`

#### Scenario: DEFAULT_ROUTE is not emitted for non-default-route interfaces
- **WHEN** a service declares an interface with role `mgmt` and no `defaultRoute` flag
- **THEN** the rendered environment does not contain `IFACE_MGMT_DEFAULT_ROUTE`

#### Scenario: NETWORK_MANAGED is emitted for a Podman-managed network
- **WHEN** a service declares an interface with role `mgmt` on a network with no `ipamDriver` (or any value other than `none`)
- **THEN** the rendered environment includes `IFACE_MGMT_NETWORK_MANAGED=1`

#### Scenario: NETWORK_MANAGED is omitted for a container-managed network
- **WHEN** a service declares an interface with role `wan` on a network with `ipamDriver: none`
- **THEN** the rendered environment does not contain `IFACE_WAN_NETWORK_MANAGED`
