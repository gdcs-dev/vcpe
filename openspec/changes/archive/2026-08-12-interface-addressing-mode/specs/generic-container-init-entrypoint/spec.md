## REMOVED Requirements

### Requirement: Default-route DHCP
**Reason**: Superseded by the manifest-level `interfaces[].addressing` field (see the `interface-addressing-mode` capability). DHCP/static selection is no longer keyed off the single `defaultRoute: true` interface or the `VCPE_INIT_STATIC_ROLE` env var — every declared interface now carries its own `addressing` value, resolved into `IFACE_<ROLE>_ADDRESSING`.
**Migration**: Manifests that relied on `defaultRoute: true` implying DHCP with no other configuration continue to work unchanged, since `addressing` defaults to `dhcp`. Manifests that set `config.env.VCPE_INIT_STATIC_ROLE=<role>` MUST instead set `addressing: static` (with an explicit `ipv4`/`ipv6`) on that interface in `interfaces[]`, and remove the `VCPE_INIT_STATIC_ROLE` env var.

## ADDED Requirements

### Requirement: Per-interface addressing initialization
The entrypoint SHALL iterate every interface for which `IFACE_<ROLE>_DEVICE` is set and, for each, initialize networking according to `IFACE_<ROLE>_ADDRESSING`: when `dhcp` and `IFACE_<ROLE>_NETWORK_MANAGED` is unset, the entrypoint SHALL bring the device up and run `udhcpc` on it (honoring `VCPE_INIT_MAC_ROLE`/`VCPE_INIT_HOSTNAME` as today when they name that role); when `dhcp` and `IFACE_<ROLE>_NETWORK_MANAGED=1` is set, the entrypoint SHALL bring the device up and take no further action (Podman has already assigned the address); when `static`, the entrypoint SHALL apply `IFACE_<ROLE>_IPV4`/`IFACE_<ROLE>_IPV6` via `ip addr add` and, if `IFACE_<ROLE>_DEFAULT_ROUTE=1` is also set for that role, add a default route via `IFACE_<ROLE>_GATEWAY4`/`GATEWAY6`. This replaces the prior behavior of only initializing the single interface marked `defaultRoute: true`. Only the interface marked `IFACE_<ROLE>_DEFAULT_ROUTE=1` may keep a default route from DHCP; the entrypoint SHALL remove any default route a non-default-route interface's DHCP lease installs, so multiple concurrently-DHCP'd interfaces cannot race for the default route.

#### Scenario: Every declared interface is initialized
- **WHEN** a generic-container service declares interfaces for roles `wan` and `lan-p1`, both with `IFACE_<ROLE>_DEVICE` set
- **THEN** the entrypoint initializes both devices, not only one

#### Scenario: Dhcp role runs udhcpc
- **WHEN** `IFACE_LAN_P1_ADDRESSING=dhcp` and `IFACE_LAN_P1_DEVICE=eth0` are set
- **THEN** the entrypoint brings `eth0` up and runs `udhcpc` on it

#### Scenario: Static role applies the manifest address without DHCP
- **WHEN** `IFACE_WAN_ADDRESSING=static`, `IFACE_WAN_IPV4` and `IFACE_WAN_GATEWAY4` are present, and `IFACE_WAN_DEFAULT_ROUTE=1` is set
- **THEN** the entrypoint applies the static address and default route on that interface without running `udhcpc`

#### Scenario: Role with no device is skipped
- **WHEN** a role has no `IFACE_<ROLE>_DEVICE` set
- **THEN** the entrypoint does not attempt to initialize that role

#### Scenario: Dhcp is a no-op on a Podman-managed network
- **WHEN** `IFACE_MGMT_ADDRESSING=dhcp` and `IFACE_MGMT_NETWORK_MANAGED=1` are set
- **THEN** the entrypoint does not run `udhcpc` for that role

#### Scenario: Non-default-route DHCP interface cannot win the default route
- **WHEN** a non-default-route interface's `udhcpc` lease includes a router option
- **THEN** the entrypoint removes any default route installed via that interface's device, leaving the default-route interface's route intact
