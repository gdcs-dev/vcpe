## ADDED Requirements

### Requirement: Gateway LAN bridge name is manifest-driven
The gateway service type SHALL read the LAN bridge name from `config.lan.bridge`, defaulting to `brlan0` when unset. The renderer SHALL emit `LAN_BRIDGE=<value>` in `compose.env`. The container entrypoint SHALL create the bridge with name `$LAN_BRIDGE` and attach all `$LAN_DEVICES` to it.

#### Scenario: lan.bridge config field controls bridge name
- **WHEN** a gateway service declares `config.lan.bridge: cpe-lan`
- **THEN** the renderer emits `LAN_BRIDGE=cpe-lan` and the entrypoint creates that bridge

#### Scenario: Default bridge name is brlan0
- **WHEN** a gateway service does not set `config.lan.bridge`
- **THEN** the renderer emits `LAN_BRIDGE=brlan0`

### Requirement: Gateway LAN devices list is manifest-driven
The gateway renderer SHALL emit `LAN_DEVICES` as a space-delimited list of device names for all interfaces whose role matches the `config.erouter.lanPrefix` pattern. The container entrypoint SHALL iterate `$LAN_DEVICES` to determine which interfaces to bridge, replacing the hard-coded `eth0 eth1 eth2 eth3` loop.

#### Scenario: LAN_DEVICES reflects declared LAN interface device names
- **WHEN** a gateway declares lan-p1 (device=eth0), lan-p2 (device=eth1), lan-p3 (device=eth2), lan-p4 (device=eth3)
- **THEN** the renderer emits `LAN_DEVICES=eth0 eth1 eth2 eth3`

#### Scenario: Gateway configure_networking reads erouter interface name from env
- **WHEN** the gateway entrypoint runs configure_networking
- **THEN** it uses `$IFACE_WAN_DEVICE` for the erouter interface name and `$IFACE_CM_DEVICE` for the wan0 (CM) interface name, rather than hard-coded `erouter0` and `wan0`
