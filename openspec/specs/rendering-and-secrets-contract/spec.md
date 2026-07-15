## Purpose
Define typed rendering and secret-handling contracts for runtime artifact generation and apply-time resolution.

## Requirements

### Requirement: Typed rendering contract
The system SHALL render runtime configuration artifacts from validated typed inputs dispatched by service `type`, and MUST NOT use regex or unbounded string substitution to generate configuration. The rendering contract SHALL be implemented through typed renderer interfaces selected from the service type registry; temporary subprocess renderers MUST receive structured arguments and validate expected outputs.

#### Scenario: Rendering fails on invalid typed input
- **WHEN** validated input is missing a required value
- **THEN** rendering fails with a structured error before runtime mutation

#### Scenario: Renderer is selected by service type
- **WHEN** the engine renders a service
- **THEN** it dispatches to the typed renderer registered for the service's `type`

#### Scenario: Regex substitution is not used
- **WHEN** a service's runtime configuration is generated
- **THEN** the system renders from typed fields and performs no regex or freeform string substitution

### Requirement: Secret reference resolution and redaction
The system MUST support phase-1 secret providers `env` and `file`, resolve secrets at apply time only, and MUST NOT persist secret values in state or logs.

#### Scenario: Missing secret reference blocks apply
- **WHEN** a `secretRef` points to a non-existent key in the selected provider
- **THEN** apply fails with a non-redacted reference identifier and without logging secret payloads

### Requirement: Unsupported renderer paths fail before mutation
The system MUST fail with a clear unsupported-type error before runtime mutation when a service declares a `type` that has no renderer registered in the service type registry.

#### Scenario: Unregistered type is blocked
- **WHEN** a deployment declares a service whose `type` has no registered renderer
- **THEN** planning or preflight fails identifying the service and `type` with an unsupported-type reason

### Requirement: Typed BNG renderer
The system SHALL provide a typed BNG renderer that consumes the bng-type `config` (typed `access[]` with `radvd`, `dhcp4`, and `dhcp6` fields) and emits `dhcpd.conf`, `dhcpd6.conf`, and `radvd.conf` artifacts with no embedded IP, MAC, or gateway literals. NAT source CIDRs SHALL be derived from networks where `nat` is true.

#### Scenario: BNG artifacts render from typed config
- **WHEN** an operator plans or applies a bng-type service
- **THEN** the renderer produces validated `dhcpd.conf`, `dhcpd6.conf`, and `radvd.conf` from typed fields before container mutation

#### Scenario: BNG renderer contains no embedded literals
- **WHEN** the BNG renderer emits artifacts
- **THEN** all addresses, MACs, and gateways come from IPAM, interfaces, and networks rather than embedded literals

### Requirement: Generic container renderer
The system SHALL provide a `generic-container` renderer that produces runtime artifacts from a typed config of optional `command`, `env`, `ports`, and `volumes`, and SHALL generate the compose definition for generic-container services from the manifest.

#### Scenario: Generic container renders from typed config
- **WHEN** a `generic-container` service is planned
- **THEN** the renderer emits its compose definition and environment from the typed config

### Requirement: Canonical interface environment contract
The system SHALL emit a canonical environment contract consumed by curated compose files: for each interface role, `IFACE_<ROLE>_NETWORK`, `IFACE_<ROLE>_DEVICE`, `IFACE_<ROLE>_MAC`, `IFACE_<ROLE>_IPV4`, `IFACE_<ROLE>_IPV6`, `IFACE_<ROLE>_GATEWAY4`, and `IFACE_<ROLE>_GATEWAY6` (the role upper-cased with `-` replaced by `_`), plus `DEPLOYMENT_NAME`, `SERVICE_NAME`, and `IMAGE`.

#### Scenario: Interface variables are emitted per role
- **WHEN** a service declares an interface with role `lan-p1`
- **THEN** the rendered environment includes `IFACE_LAN_P1_NETWORK`, `IFACE_LAN_P1_DEVICE`, `IFACE_LAN_P1_MAC`, and the corresponding address and gateway variables
