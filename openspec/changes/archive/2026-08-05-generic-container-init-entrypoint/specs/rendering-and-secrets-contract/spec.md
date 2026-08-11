## MODIFIED Requirements

### Requirement: Generic container renderer
The system SHALL provide a `generic-container` renderer that produces runtime artifacts from a typed config of optional `command`, `env`, `ports`, and `volumes`, and SHALL generate the compose definition for generic-container services from the manifest. The renderer SHALL always emit an `entrypoint.sh` artifact (content embedded in the renderer binary) and SHALL NOT emit a `resolv.conf` artifact. The generated compose definition SHALL set `entrypoint:` to execute the embedded script and SHALL include a bind-mount of `./entrypoint.sh:/run/vcpe/entrypoint.sh:ro`. The compose definition SHALL NOT include a `./resolv.conf:/etc/resolv.conf:ro` volume mount.

#### Scenario: Generic container renders from typed config
- **WHEN** a `generic-container` service is planned
- **THEN** the renderer emits `compose.env`, `compose.yaml`, and `entrypoint.sh` artifacts from the typed config; no `resolv.conf` artifact is present

#### Scenario: Compose sets entrypoint
- **WHEN** the renderer generates the compose definition for a generic-container service
- **THEN** the compose service entry includes `entrypoint: ["/bin/sh", "/run/vcpe/entrypoint.sh"]` and a volume bind-mount `./entrypoint.sh:/run/vcpe/entrypoint.sh:ro`

#### Scenario: No static resolv.conf in compose volumes
- **WHEN** the renderer generates the compose definition for a generic-container service connected to a LAN network
- **THEN** the compose volumes do not include `./resolv.conf:/etc/resolv.conf:ro`

## MODIFIED Requirements

### Requirement: Canonical interface environment contract
The system SHALL emit a canonical environment contract consumed by curated compose files and the generic-container init entrypoint: for each interface role, `IFACE_<ROLE>_NETWORK`, `IFACE_<ROLE>_DEVICE`, `IFACE_<ROLE>_MAC`, `IFACE_<ROLE>_IPV4`, `IFACE_<ROLE>_IPV6`, `IFACE_<ROLE>_GATEWAY4`, and `IFACE_<ROLE>_GATEWAY6` (the role upper-cased with `-` replaced by `_`), plus `DEPLOYMENT_NAME`, `SERVICE_NAME`, and `IMAGE`. For interfaces where `defaultRoute` is true, the system SHALL additionally emit `IFACE_<ROLE>_DEFAULT_ROUTE=1`. The `DEFAULT_ROUTE` variable SHALL be omitted entirely for interfaces where `defaultRoute` is false or unset.

#### Scenario: Interface variables are emitted per role
- **WHEN** a service declares an interface with role `lan-p1`
- **THEN** the rendered environment includes `IFACE_LAN_P1_NETWORK`, `IFACE_LAN_P1_DEVICE`, `IFACE_LAN_P1_MAC`, and the corresponding address and gateway variables

#### Scenario: DEFAULT_ROUTE is emitted for the default-route interface
- **WHEN** a service declares an interface with role `lan-p1` and `defaultRoute: true`
- **THEN** the rendered environment includes `IFACE_LAN_P1_DEFAULT_ROUTE=1`

#### Scenario: DEFAULT_ROUTE is not emitted for non-default-route interfaces
- **WHEN** a service declares an interface with role `mgmt` and no `defaultRoute` flag
- **THEN** the rendered environment does not contain `IFACE_MGMT_DEFAULT_ROUTE`
