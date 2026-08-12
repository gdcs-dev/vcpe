## Purpose
Define the initialization entrypoint contract for `generic-container` services: the `VCPE_INIT_*` environment variable protocol, the always-emitted `entrypoint.sh` artifact, and the fixed identity → network → dns → exec initialization sequence.

## Requirements

### Requirement: Generic container init entrypoint
The `generic-container` renderer SHALL always emit an `entrypoint.sh` artifact and SHALL set the `entrypoint:` key in the generated compose service to execute it before the user's command. The script SHALL be embedded in the renderer binary as a string constant and SHALL NOT be read from disk at render time.

#### Scenario: Entrypoint is always emitted
- **WHEN** any `generic-container` service is rendered
- **THEN** the render result includes an `entrypoint.sh` artifact and the generated compose sets `entrypoint: ["/bin/sh", "/run/vcpe/entrypoint.sh"]` with a bind-mount of `./entrypoint.sh:/run/vcpe/entrypoint.sh:ro`

#### Scenario: Entrypoint executes user command
- **WHEN** no `VCPE_INIT_*` environment variables are set for a generic-container service
- **THEN** the entrypoint performs no initialization and `exec`s the configured command unchanged

### Requirement: VCPE_INIT_* initialization protocol
The entrypoint SHALL interpret environment variables with the `VCPE_INIT_` prefix as initialization directives and SHALL pass all other environment variables through to the exec'd command unmodified. The initialization sequence SHALL be fixed: identity operations first, then network, then exec.

#### Scenario: Non-VCPE_INIT vars are not consumed
- **WHEN** `config.env` contains variables without the `VCPE_INIT_` prefix (e.g., `DEVICE_SERIAL`, `TR069_ACS_URL`)
- **THEN** the entrypoint does not inspect or modify them, and they are present in the environment of the exec'd command

#### Scenario: Init variables drive setup
- **WHEN** `VCPE_INIT_HOSTNAME=cpe-extender-01` is set
- **THEN** the entrypoint calls `hostname cpe-extender-01` before exec

### Requirement: Network-layer identity initialization
The entrypoint SHALL support setting network-layer identity before DHCP runs via `VCPE_INIT_MAC_ROLE=<role>` and `VCPE_INIT_HOSTNAME=<name>`. MAC assignment SHALL use `ip link set <device> address <mac>`, where `<device>` is resolved from `IFACE_<ROLE>_DEVICE` and `<mac>` is the value of `IFACE_<ROLE>_MAC`. Hostname SHALL be set with the `hostname` command and SHALL be passed to DHCP as option 12.

#### Scenario: MAC is set before DHCP
- **WHEN** `VCPE_INIT_MAC_ROLE=lan-p1` and `IFACE_LAN_P1_DEVICE=eth0` are set
- **THEN** the entrypoint runs `ip link set eth0 address <mac>` before running `udhcpc`

#### Scenario: Hostname is visible in DHCP
- **WHEN** `VCPE_INIT_HOSTNAME=phone-01` is set
- **THEN** the entrypoint sets the container hostname and passes it to `udhcpc` via `-x hostname:phone-01`

### Requirement: Per-interface addressing initialization
The entrypoint SHALL iterate every interface for which `IFACE_<ROLE>_DEVICE` is set and, for each, initialize networking according to `IFACE_<ROLE>_ADDRESSING`: when `dhcp` and `IFACE_<ROLE>_NETWORK_MANAGED` is unset, the entrypoint SHALL bring the device up and run `udhcpc` on it (honoring `VCPE_INIT_MAC_ROLE`/`VCPE_INIT_HOSTNAME` as today when they name that role); when `dhcp` and `IFACE_<ROLE>_NETWORK_MANAGED=1` is set, the entrypoint SHALL bring the device up and take no further action (Podman has already assigned the address); when `static`, the entrypoint SHALL apply `IFACE_<ROLE>_IPV4`/`IFACE_<ROLE>_IPV6` via `ip addr add` and, if `IFACE_<ROLE>_DEFAULT_ROUTE=1` is also set for that role, add a default route via `IFACE_<ROLE>_GATEWAY4`/`GATEWAY6`. Only the interface marked `IFACE_<ROLE>_DEFAULT_ROUTE=1` may keep a default route from DHCP; the entrypoint SHALL remove any default route a non-default-route interface's DHCP lease installs, so multiple concurrently-DHCP'd interfaces cannot race for the default route.

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

### Requirement: DNS configuration via entrypoint
The entrypoint SHALL configure `/etc/resolv.conf` as part of initialization. When DHCP is used, DNS SHALL be set by the DHCP server's option 6 response via `udhcpc`'s default script. When `VCPE_INIT_NAMESERVER=<ip>` is set, the entrypoint SHALL write a `resolv.conf` containing that nameserver after DHCP completes (overriding the DHCP-provided value if any). The `generic-container` renderer SHALL NOT generate a static `resolv.conf` artifact or bind-mount it.

#### Scenario: DHCP sets DNS
- **WHEN** a generic-container service uses DHCP and the DHCP server sends option 6
- **THEN** `udhcpc`'s default script writes `/etc/resolv.conf` with the DHCP-provided nameserver

#### Scenario: Explicit nameserver overrides DHCP DNS
- **WHEN** `VCPE_INIT_NAMESERVER=192.168.1.1` is set
- **THEN** the entrypoint writes `nameserver 192.168.1.1` to `/etc/resolv.conf` after DHCP

#### Scenario: No static resolv.conf artifact is emitted
- **WHEN** any generic-container service is rendered, regardless of LAN network attachment
- **THEN** the render result does not contain a `resolv.conf` artifact and the compose volumes do not include `./resolv.conf:/etc/resolv.conf:ro`

### Requirement: Optional settle delay
The entrypoint SHALL support `VCPE_INIT_SLEEP=<seconds>` to introduce a pause after network initialization and before exec, allowing workloads that require a settled network to defer startup.

#### Scenario: Sleep before exec
- **WHEN** `VCPE_INIT_SLEEP=2` is set
- **THEN** the entrypoint sleeps 2 seconds after completing initialization and before exec'ing the command
