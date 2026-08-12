## MODIFIED Requirements

### Requirement: Service interface declarations
The system SHALL accept `services[].interfaces[]` entries that bind to a network by `role`, with an optional `device` (defaulting to ordered `eth<n>`), an optional `mac`, at most one `ipv4` and one `ipv6` address, and an optional `addressing` field (`dhcp` or `static`, defaulting to `dhcp`). The interface gateway is inherited from its network. At most one interface per service MAY set `defaultRoute: true`; if none does, the interface bound to the `wan` role is the default route. An `addressing: static` interface MUST declare `ipv4` and/or `ipv6`; an `addressing: dhcp` interface (explicit or defaulted) MUST NOT declare either. Interfaces that also declare `bridge:` are exempt from this addressing/address consistency check.

#### Scenario: Interface inherits network gateway
- **WHEN** an interface binds to a network by `role`
- **THEN** the interface uses the network's gateway for its address family

#### Scenario: Multiple default routes are rejected
- **WHEN** more than one interface in a service sets `defaultRoute: true`
- **THEN** validation fails identifying the conflicting interfaces

#### Scenario: Addressing defaults to dhcp
- **WHEN** an interface omits the `addressing` field
- **THEN** the system treats the interface as `addressing: dhcp`

#### Scenario: Static addressing without an address is rejected
- **WHEN** an interface declares `addressing: static` and neither `ipv4` nor `ipv6`
- **THEN** validation fails identifying the service and interface role

#### Scenario: Dhcp addressing with an address is rejected
- **WHEN** an interface declares `addressing: dhcp` (or omits the field) and also declares `ipv4` or `ipv6`
- **THEN** validation fails identifying the service and interface role

#### Scenario: Bridge-enslaved interface is exempt from the addressing/address check
- **WHEN** an interface declares `bridge: brlan0` and any combination of `addressing` and `ipv4`/`ipv6`
- **THEN** validation does not apply the addressing/address consistency rule to that interface
