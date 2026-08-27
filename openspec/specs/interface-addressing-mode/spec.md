## Purpose
Define the manifest-level, per-interface `addressing` field (`dhcp`/`static`) and the cross-service-type runtime contract for how each service type honors it — which service types run a real DHCP client, which stay static-only or unaffected, and how Podman-managed networks and bridge-enslaved interfaces are excluded.

## Requirements

### Requirement: Per-interface addressing mode field
The manifest `services[].interfaces[]` schema SHALL accept an optional `addressing` field with value `dhcp` or `static`. When omitted, `addressing` SHALL default to `dhcp`. This field SHALL apply uniformly to every interface on every registered service type, regardless of which network `role` it binds to or that network's `ipamDriver` setting.

#### Scenario: Addressing defaults to dhcp when omitted
- **WHEN** an interface declares no `addressing` field
- **THEN** the system treats it as `addressing: dhcp`

#### Scenario: Addressing is explicit
- **WHEN** an interface declares `addressing: static`
- **THEN** the system treats the interface as statically addressed

### Requirement: Static addressing requires an explicit address
An interface with `addressing: static` MUST declare at least one of `ipv4` or `ipv6`. Validation SHALL reject a `static` interface with neither address set.

#### Scenario: Static without an address is rejected
- **WHEN** a service interface declares `addressing: static` and no `ipv4` or `ipv6`
- **THEN** validation fails identifying the service and interface role

#### Scenario: Static with an address is accepted
- **WHEN** a service interface declares `addressing: static` and `ipv4: "10.7.200.10"`
- **THEN** validation succeeds

### Requirement: DHCP addressing forbids an explicit address
An interface with `addressing: dhcp` (explicit or defaulted) MUST NOT declare `ipv4` or `ipv6`. Validation SHALL reject a `dhcp` interface that sets either address.

#### Scenario: Dhcp with an address is rejected
- **WHEN** a service interface declares `addressing: dhcp` and `ipv4: "10.7.200.10"`
- **THEN** validation fails identifying the service and interface role

#### Scenario: Default dhcp with an address is rejected
- **WHEN** a service interface omits `addressing` but declares `ipv4: "10.7.200.10"`
- **THEN** validation fails identifying the service and interface role, since the omitted field defaults to `dhcp`

### Requirement: Addressing is ignored on bridge-enslaved interfaces
An interface that declares `bridge:` (enslaving it to a container-internal bridge) MAY also declare `addressing`, but the value SHALL be ignored entirely: it is not validated against the interface's `ipv4`/`ipv6` fields, and it has no runtime effect.

#### Scenario: Bridge-enslaved interface with addressing set is accepted regardless of value
- **WHEN** a service interface declares `bridge: brlan0` and `addressing: static` with no `ipv4`
- **THEN** validation succeeds (the static-requires-address rule does not apply to bridge-enslaved interfaces)

### Requirement: DHCP addressing has no runtime effect on a Podman-managed network
Every interface resolves whether its network is Podman-managed (any `ipamDriver` other than `none`, e.g. `mgmt`) or container-managed (`ipamDriver: none`, e.g. `wan`/`cm`). `addressing: dhcp` on a Podman-managed-network interface is accepted by validation and has no runtime effect: no DHCP client is invoked, since Podman has already assigned the address before the container starts and a DHCP negotiation would be redundant with (and could conflict with) that address. `addressing: static` is unaffected on a Podman-managed network — the address is still applied via the existing Podman-native address-pinning mechanism.

#### Scenario: Dhcp on a Podman-managed network is a no-op
- **WHEN** a service interface on a network without `ipamDriver: none` (e.g. `mgmt`) resolves to `addressing: dhcp`
- **THEN** the service's entrypoint does not invoke a DHCP client for that interface

#### Scenario: Static on a Podman-managed network is unaffected
- **WHEN** a service interface on a network without `ipamDriver: none` resolves to `addressing: static`
- **THEN** the interface's declared address is still applied via the network's native address-pinning mechanism

### Requirement: No addressing guard for DHCP-server-side services
The system SHALL NOT validate that a service acting as a DHCP server on a given network role (e.g. `bng`) has `addressing: static` on its own interface for that role. A manifest that sets `addressing: dhcp` on such an interface is accepted by validation and MAY fail at runtime.

#### Scenario: BNG interface set to dhcp passes validation
- **WHEN** a `bng` service interface for a role it also serves DHCP on (per its own `config.access[]`) declares `addressing: dhcp`
- **THEN** validation succeeds; no error is raised for this combination

### Requirement: Per-interface addressing runtime contract
For every service type except `xb10`, the control plane's rendering and container entrypoints SHALL honor each interface's resolved `addressing` value. For `generic-container`, `bng`, `gateway`, `webpa`, `event-sink`, and `oktopus`: `dhcp` SHALL result in a real DHCP client (e.g. `udhcpc` or `dhclient`) being run against that interface's device at container start, unless the interface's network is Podman-managed (see the Podman-managed-network requirement above), in which case it is a no-op; `static` SHALL result in the interface's `ipv4`/`ipv6` being applied directly (via `ip addr add` and/or a Podman-managed `ipv4_address` compose pin, depending on the network's `ipamDriver`). For `routerd`: `dhcp` and `static` SHALL instead be honored by compiling the interface into a `WanPolicy` resource (`source: Dhcp` or `source: Static{...}` respectively) applied via `routerctl`, per the `routerd-manifest-config` capability — routerd never invokes a DHCP client or `ip addr add` directly from shell. `xb10` is exempt from this requirement; its existing interface initialization behavior is unchanged by this capability.

#### Scenario: Dhcp interface runs a real DHCP client
- **WHEN** a `gateway`, `bng`, `webpa`, `event-sink`, `oktopus`, or `generic-container` service interface on a container-managed (`ipamDriver: none`) network resolves to `addressing: dhcp`
- **THEN** the service's entrypoint runs a DHCP client against that interface's device before the workload starts

#### Scenario: Static interface applies its manifest address
- **WHEN** a service interface of one of those types (any except `xb10`/`routerd`) resolves to `addressing: static`
- **THEN** the service's entrypoint or rendered compose applies the interface's declared `ipv4`/`ipv6` without running a DHCP client on that device

#### Scenario: Routerd interface addressing is applied via WanPolicy, not shell
- **WHEN** a `routerd` service's unbridged interface resolves to `addressing: dhcp` or `addressing: static`
- **THEN** the rendered config contains a corresponding `WanPolicy` resource for that interface, and no shell-level `udhcpc`/`ip addr add` is invoked for it by the container entrypoint

#### Scenario: xb10 is unaffected
- **WHEN** an `xb10` service interface declares any `addressing` value
- **THEN** `xb10`'s existing entrypoint behavior is unchanged; the value has no effect on its rendering or runtime initialization
