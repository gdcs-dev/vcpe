## MODIFIED Requirements

### Requirement: Versioned manifest schema
The system SHALL define a versioned, Kubernetes-style manifest schema using `apiVersion: vcpe.dev/v1`, `kind: Deployment`, `metadata` (name and labels), and `spec` (networks and services). The system SHALL reject manifests whose `apiVersion` or `kind` is unsupported before planning or apply.

#### Scenario: Manifest version is validated
- **WHEN** a manifest with an unsupported `apiVersion` or `kind` is submitted
- **THEN** validation fails before planning or apply

#### Scenario: Deployment identity comes from metadata.name
- **WHEN** a valid v1 manifest is loaded
- **THEN** the system uses `metadata.name` as the deployment identity for resource naming and state, and treats `metadata.labels` (including `customer`) as opaque metadata

### Requirement: Cross-object validation
The system MUST validate manifest objects against each other before planning, including: every interface `role` resolves to a declared network; service names are unique; `dependsOn` references exist and form no cycle; explicit interface addresses fall within their network CIDR; each service satisfies its type's expected roles; `replicas` does not exceed the configured per-service maximum; and the number of distinct active deployments does not exceed `maxActiveDeployments`. The system SHALL emit a warning when a declared network is referenced by no interface.

#### Scenario: Interface role without a matching network is rejected
- **WHEN** a service interface declares a `role` that no `networks[]` entry provides
- **THEN** validation fails identifying the service and unresolved role

#### Scenario: Duplicate service names are rejected
- **WHEN** two services share the same `name`
- **THEN** validation fails identifying the duplicate name

#### Scenario: dependsOn cycle is rejected
- **WHEN** service `dependsOn` references form a cycle or reference an unknown service
- **THEN** validation fails identifying the offending dependency

#### Scenario: Active deployment cap is enforced
- **WHEN** applying a new deployment would exceed `maxActiveDeployments` distinct active `metadata.name` values
- **THEN** validation fails with a cap-violation error

#### Scenario: Unused network produces a warning
- **WHEN** a declared network is referenced by no interface
- **THEN** validation succeeds with a warning identifying the unused network

## ADDED Requirements

### Requirement: Dual-stack network declarations
The system SHALL accept `spec.networks[]` entries with a `role`, an optional `bridge` name (defaulting to `<metadata.name>-<role>`), `nat` and `firewall` flags, and optional `ipv4` and `ipv6` blocks. Each address-family block MAY define a `cidr`, an optional `gateway` (defaulting to the first usable host in the CIDR), and an optional `pool` range. A network MAY declare either or both address families to support IPv4-only, IPv6-only, or dual-stack topologies.

#### Scenario: IPv6-only network is accepted
- **WHEN** a network declares only an `ipv6` block with a valid CIDR
- **THEN** validation succeeds and the network is planned as IPv6-only

#### Scenario: Default bridge name is derived
- **WHEN** a network omits `bridge`
- **THEN** the system uses `<metadata.name>-<role>` as the bridge name

### Requirement: Service interface declarations
The system SHALL accept `services[].interfaces[]` entries that bind to a network by `role`, with an optional `device` (defaulting to ordered `eth<n>`), an optional `mac`, and at most one `ipv4` and one `ipv6` address. The interface gateway is inherited from its network. At most one interface per service MAY set `defaultRoute: true`; if none does, the interface bound to the `wan` role is the default route.

#### Scenario: Interface inherits network gateway
- **WHEN** an interface binds to a network by `role`
- **THEN** the interface uses the network's gateway for its address family

#### Scenario: Multiple default routes are rejected
- **WHEN** more than one interface in a service sets `defaultRoute: true`
- **THEN** validation fails identifying the conflicting interfaces

### Requirement: Replicas and explicit addressing
The system SHALL allow explicit `mac`, `ipv4`, or `ipv6` values on an interface only when the service `replicas` is 1. When `replicas` is greater than 1, the system MUST allocate addresses from IPAM and assign deterministic MACs that include a per-replica index.

#### Scenario: Explicit address with multiple replicas is rejected
- **WHEN** a service with `replicas` greater than 1 declares an explicit interface address or MAC
- **THEN** validation fails identifying the service and interface

#### Scenario: Multiple replicas allocate per-replica identities
- **WHEN** a service with `replicas` greater than 1 is planned
- **THEN** each replica receives an IPAM-allocated address and a deterministic MAC that includes the replica index
