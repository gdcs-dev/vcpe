## MODIFIED Requirements

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
