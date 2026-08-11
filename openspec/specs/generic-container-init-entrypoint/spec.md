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

### Requirement: Default-route DHCP
The entrypoint SHALL automatically run DHCP on the interface declared as the default route (identified by `IFACE_<ROLE>_DEFAULT_ROUTE=1`) unless that role is listed in `VCPE_INIT_STATIC_ROLE`. No explicit `VCPE_INIT_DHCP_ROLE` variable is required for the common DHCP case.

#### Scenario: DHCP runs automatically on default-route interface
- **WHEN** a service has an interface with `defaultRoute: true` and `VCPE_INIT_STATIC_ROLE` is not set for that role
- **THEN** the entrypoint brings up the interface device and runs `udhcpc` on it

#### Scenario: Static assignment opts out of DHCP
- **WHEN** `VCPE_INIT_STATIC_ROLE=lan-p1` is set and `IFACE_LAN_P1_IPV4` and `IFACE_LAN_P1_GATEWAY4` are present
- **THEN** the entrypoint applies the static address and default route without running `udhcpc`

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
