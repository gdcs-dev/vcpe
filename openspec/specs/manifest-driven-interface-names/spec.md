## Purpose
Define the end-to-end contract for manifest-driven interface device naming, replacing hard-coded interface names in renderers and container entrypoints with values sourced from the manifest.

## Requirements

### Requirement: Interface device names are manifest-driven end-to-end
The system SHALL use the `Interface.device` field from the manifest as the container-internal interface name for all service types that perform MAC-based interface renaming. The control plane SHALL emit `IFACE_<ROLE>_DEVICE` and `IFACE_<ROLE>_MAC` env vars as the canonical contract for interface identity. Container entrypoints for BNG, Gateway, and XB10 SHALL rename interfaces by building a target table from `IFACE_*_MAC` and `IFACE_*_DEVICE` env vars, with no hard-coded target names.

#### Scenario: Manifest device field reaches container as IFACE_*_DEVICE
- **WHEN** a service interface declares `device: erouter0`
- **THEN** the renderer emits `IFACE_<ROLE>_DEVICE=erouter0` in the service's `compose.env`

#### Scenario: Container entrypoint renames by manifest-declared device name
- **WHEN** the container starts and `IFACE_*_DEVICE=erouter0` and `IFACE_*_MAC=02:xx:xx:xx` are set
- **THEN** the entrypoint finds the network interface with that MAC and renames it to `erouter0`

#### Scenario: Legacy role-specific MAC aliases are not emitted
- **WHEN** the renderer generates `compose.env` for BNG
- **THEN** `MGMT_MAC=`, `WAN_MAC=`, `CM_MAC=` do NOT appear in the output

#### Scenario: Legacy role-specific MAC aliases are not emitted for gateway
- **WHEN** the renderer generates `compose.env` for Gateway
- **THEN** `EROUTER0_MAC=`, `WAN0_MAC=`, `LAN1_MAC=`, `LAN2_MAC=`, `LAN3_MAC=`, `LAN4_MAC=` do NOT appear in the output
