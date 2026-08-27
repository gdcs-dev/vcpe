## ADDED Requirements

### Requirement: Routerd LAN bridge topology is manifest-driven via the standard bridge fields
The `routerd` service type SHALL NOT define its own bridge-topology config schema. It SHALL instead derive LAN bridge topology from the manifest's existing, type-agnostic fields already used by `gateway`/`bng`: `interfaces[].bridge` (the bridge name an interface is enslaved to) and `services[].bridges[]` (the bridge's own declaration). Interfaces that do not declare `bridge` remain unbridged. The renderer SHALL group interfaces by their declared `bridge` value and compile each group into one routerd `Bridge` resource whose `members` are `ResourceRef`s to the corresponding `Interface` resources, and SHALL compile every declared interface (bridged or not) into a routerd `Interface` resource.

#### Scenario: Declared bridge produces a Bridge resource
- **WHEN** a `routerd` service declares four interfaces with `bridge: brlan0` (roles `lan-p1`..`lan-p4`)
- **THEN** the rendered `ConfigDocument` includes a `Bridge` resource named `brlan0` whose members reference the four interfaces' devices

#### Scenario: Unbridged interface remains a standalone Interface resource
- **WHEN** a `routerd` service declares an interface for role `wan` with no `bridge` field
- **THEN** the rendered `ConfigDocument` includes an `Interface` resource for that device and no `Bridge` resource references it

#### Scenario: Routerd config accepts no type-specific fields
- **WHEN** a `routerd` service declares a `config:` block with any field (including a `bridges` key, which belongs at `services[].bridges` rather than under `config`)
- **THEN** validation fails, since `routerd`'s typed config schema currently defines no fields

### Requirement: A bridge's declared IPv4 address is applied to the bridge itself
When a `routerd` service's `services[].bridges[]` entry declares `ipv4` for a bridge that is actually created (referenced by at least one interface's `bridge` field), the renderer SHALL compile an `IpAddress` resource referencing that `Bridge` resource with the declared address and prefix length. A bridge declared without `ipv4` SHALL NOT receive an `IpAddress` resource.

#### Scenario: Bridge ipv4 produces an IpAddress resource on the Bridge
- **WHEN** a `routerd` service declares `services[].bridges: [{name: brlan0, ipv4: "10.0.0.1/24"}]` and at least one interface with `bridge: brlan0`
- **THEN** the rendered `ConfigDocument` includes an `IpAddress` resource with `interface_ref: {kind: Bridge, id: brlan0}`, `address: 10.0.0.1`, `prefix_len: 24`

#### Scenario: Bridge without ipv4 gets no IpAddress resource
- **WHEN** a `routerd` service's bridge declaration omits `ipv4`
- **THEN** the rendered `ConfigDocument` contains no `IpAddress` resource referencing that bridge

### Requirement: Routerd WAN addressing is derived from interface addressing mode
For every unbridged interface on a `routerd` service, the renderer SHALL compile the interface's resolved `addressing` value (per the `interface-addressing-mode` capability) into a `WanPolicy` resource referencing that interface: `addressing: dhcp` SHALL produce `WanPolicy{source: Dhcp}`; `addressing: static` SHALL produce `WanPolicy{source: Static{address, prefix_len, gateway}}` populated from the interface's resolved `ipv4` and gateway. Bridged interfaces SHALL NOT receive a `WanPolicy` resource. When more than one unbridged interface exists, the renderer SHALL assign `priority: 1` to the interface marked `defaultRoute: true` (if any) and strictly increasing priorities to the rest in role-name order, so multiple DHCP'd uplinks never collide at the same priority.

#### Scenario: Dhcp addressing produces a Dhcp WanPolicy
- **WHEN** a `routerd` service's `wan` interface resolves to `addressing: dhcp`
- **THEN** the rendered `ConfigDocument` includes a `WanPolicy` resource for that interface with `source: Dhcp`

#### Scenario: Static addressing produces a Static WanPolicy
- **WHEN** a `routerd` service's `wan` interface resolves to `addressing: static` with `ipv4: 10.7.200.10` and a resolved gateway
- **THEN** the rendered `ConfigDocument` includes a `WanPolicy` resource for that interface with `source: Static` carrying the resolved address, prefix length, and gateway

#### Scenario: Bridged interfaces have no WanPolicy
- **WHEN** a `routerd` service declares an interface with `bridge: brlan0`
- **THEN** the rendered `ConfigDocument` contains no `WanPolicy` resource referencing that interface

#### Scenario: Multiple unbridged uplinks get distinct priorities
- **WHEN** a `routerd` service declares two unbridged interfaces, one with `defaultRoute: true`
- **THEN** the `defaultRoute` interface's `WanPolicy` has `priority: 1` and the other has a higher (lower-preference) priority, so both can hold a DHCP lease without a routerd route conflict

### Requirement: Routerd config is rendered ahead of time and applied at startup
The `routerd` renderer SHALL emit the full compiled `ConfigDocument` (all `Interface`, `Bridge`, and `WanPolicy` resources for the service instance) as a single JSON render artifact. The container entrypoint SHALL NOT construct or transform this document at runtime; it SHALL apply the pre-rendered file verbatim via `routerctl apply` after the `routerd` socket becomes available, and SHALL fail startup if the apply command fails.

#### Scenario: Config artifact is rendered without runtime assembly
- **WHEN** a `routerd` service is rendered
- **THEN** the render result includes a JSON config artifact containing the full `ConfigDocument`, and no shell script in the container image constructs `controllers.*`-shaped JSON from environment variables at container start

#### Scenario: Startup applies the rendered config
- **WHEN** the `routerd` container starts and the `routerd` socket becomes available
- **THEN** the entrypoint runs `routerctl apply` against the rendered config file path before the process is considered ready

#### Scenario: Failed apply fails startup
- **WHEN** `routerctl apply` exits non-zero against the rendered config
- **THEN** the container entrypoint exits non-zero rather than continuing to run `routerd` with a partially- or un-applied configuration
