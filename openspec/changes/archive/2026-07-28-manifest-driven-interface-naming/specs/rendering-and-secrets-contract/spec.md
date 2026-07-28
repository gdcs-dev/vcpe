## MODIFIED Requirements

### Requirement: Typed rendering contract
The system SHALL render runtime configuration artifacts from validated typed inputs dispatched by service `type`, and MUST NOT use regex or unbounded string substitution to generate configuration. The rendering contract SHALL be implemented through typed renderer interfaces selected from the service type registry; temporary subprocess renderers MUST receive structured arguments and validate expected outputs.

The canonical environment variable contract for interface identity SHALL be the `IFACE_<ROLE>_{DEVICE,MAC,IPV4,IPV6,GATEWAY4,GATEWAY6,NETWORK}` family emitted by `render.IfaceEnv()`. Renderers SHALL NOT emit role-specific MAC alias variables (such as `MGMT_MAC=`, `WAN_MAC=`, `CM_MAC=`, `EROUTER0_MAC=`, `WAN0_MAC=`, `LAN1_MAC=`, etc.) as those are superseded by the `IFACE_*` family.

#### Scenario: Rendering fails on invalid typed input
- **WHEN** validated input is missing a required value
- **THEN** rendering fails with a structured error before runtime mutation

#### Scenario: Renderer is selected by service type
- **WHEN** the engine renders a service
- **THEN** it dispatches to the typed renderer registered for the service's `type`

#### Scenario: Regex substitution is not used
- **WHEN** a service's runtime configuration is generated
- **THEN** the system renders from typed fields and performs no regex or freeform string substitution

#### Scenario: IFACE_* env vars are the sole interface identity contract
- **WHEN** a renderer generates compose.env for any service type
- **THEN** interface identity (device name, MAC, IPs) is expressed ONLY via `IFACE_<ROLE>_DEVICE`, `IFACE_<ROLE>_MAC`, etc. — no role-specific alias variables are present
