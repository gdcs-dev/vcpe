## Purpose
Define deterministic, idempotent reconciliation phases for Podman resource convergence and recovery.

## Requirements

### Requirement: Idempotent reconcile and apply pipeline
The system SHALL execute apply through deterministic phases and converge to desired state when the same manifest is applied repeatedly. The phases SHALL include service type planning, host-network preflight, image lifecycle decisions, typed rendering, replica delta computation, compose group application, health verification, status inspection, and generated artifact state recording.

#### Scenario: Repeated apply is convergent
- **WHEN** `apply` is run multiple times against unchanged desired state
- **THEN** the system reports no additional mutations after initial convergence

#### Scenario: Apply uses type-driven phases
- **WHEN** an operator applies a deployment through `vcpe up`
- **THEN** the plan includes service type ordering, required image actions, render artifacts, replica delta computation, compose groups, host-network intents, and health checks before runtime mutation begins

#### Scenario: Re-apply with increased replica count is convergent
- **WHEN** `vcpe up` is run with `replicas: 2` on a service that was previously applied with `replicas: 1`
- **THEN** exactly one new container is created and the original container is left running, producing a total of two running containers for that service

#### Scenario: Re-apply with decreased replica count is convergent
- **WHEN** `vcpe up` is run with `replicas: 1` on a service that was previously applied with `replicas: 2`
- **THEN** the excess container is removed and one container remains running, producing a total of one running container for that service

### Requirement: Fail-fast rollback with durable operation journal
The system MUST stop on terminal phase failure, record phase outcomes durably, and attempt bounded rollback for resources created in the current operation. Rollback order MUST follow the reverse of the planner's successfully applied service and resource order.

#### Scenario: Apply failure triggers bounded rollback
- **WHEN** a container lifecycle phase fails after network allocation succeeds
- **THEN** the system records failure details and executes compensating rollback for resources created by that operation

#### Scenario: Rollback follows reverse plan order
- **WHEN** a compose group fails after dependent resources were created
- **THEN** the system rolls back current-operation resources in reverse dependency order and records the rollback result in the operation journal

### Requirement: Typed service catalog planning
The system SHALL plan service lifecycle actions from the service type registry and the manifest, deriving service ordering, image policy, render contracts, compose groups, and health checks without per-customer catalog tables. Cross-service ordering across separate compose projects SHALL follow manifest `dependsOn`, applied in dependency order on startup and reverse dependency order on teardown.

#### Scenario: Service ordering is deterministic
- **WHEN** a deployment enables multiple services with `dependsOn` relationships
- **THEN** the planner emits deterministic start, health, stop, and rollback order across compose projects without relying on bash command order

#### Scenario: Cross-project dependsOn governs lifecycle order
- **WHEN** services in separate compose projects declare `dependsOn`
- **THEN** the planner starts dependencies first and tears them down in reverse, independent of any intra-project compose `depends_on`

### Requirement: Go-owned image lifecycle
The system SHALL manage build, pull, push, tag, and image existence decisions through Go image lifecycle components backed by typed Podman operations.

#### Scenario: Up builds missing image by policy
- **WHEN** `vcpe up` requires a missing local image and the selected image policy allows build-if-missing behavior
- **THEN** the system builds the image before applying the dependent compose group

### Requirement: Typed compose adapter
The system SHALL apply compose-backed services through a typed compose adapter that owns generated inputs, project names, command timeouts, operation journal entries, status inspection, and rollback bookkeeping.

#### Scenario: Compose command is journaled
- **WHEN** the operator applies a compose group
- **THEN** the system records the compose group identity, project name, generated input paths, phase result, and rollback eligibility in the operation journal

### Requirement: Host-network preflight
The system MUST preflight bridge, NAT, firewall, and required host capabilities before mutating runtime resources, and MUST fail before mutation if required capabilities are unavailable.

#### Scenario: Missing host capability blocks apply
- **WHEN** a deployment requires privileged host networking and the current environment lacks required capabilities
- **THEN** apply fails during preflight with an actionable error and no runtime resources are mutated

### Requirement: Versioned generated artifact state
The system SHALL store generated manifests, rendered files, operation journals, and compatibility snapshots under versioned control-plane state paths.

#### Scenario: Generated artifacts are state-scoped
- **WHEN** the operator renders artifacts for an apply operation
- **THEN** the artifacts are written under the selected control-plane state root with a versioned operation or deployment path

### Requirement: Schema-versioned state cutover
The system SHALL stamp the persisted state root with the `vcpe.dev/v1` schema version and MUST refuse to reconcile against a non-empty state root whose stamp is missing or mismatched, requiring an explicit operator-initiated state reset before applying v1 manifests.

#### Scenario: Mismatched state blocks reconcile
- **WHEN** the persisted state root holds data with a missing or non-`vcpe.dev/v1` schema stamp
- **THEN** apply fails before mutation with an actionable error directing the operator to reset state

#### Scenario: First v1 apply has no prior snapshot
- **WHEN** the operator applies a v1 manifest against a freshly reset, schema-stamped state root
- **THEN** the apply proceeds as a clean create without flagging a disruptive teardown

### Requirement: EnsureNetwork accepts NetworkSpec struct
The `networkProvisioner` interface and `podman.EnsureNetwork` implementation SHALL accept a `NetworkSpec` struct instead of positional string parameters. When `Driver` is non-empty, `podman network create` SHALL include `--driver <driver>`. Each entry in `DriverOptions` SHALL be passed as a separate `-o key=val` flag with keys in sorted order. When `IPAMDriver` is non-empty, `--ipam-driver <driver>` SHALL be included. When `Driver` is empty the behavior SHALL be identical to the previous implementation.

#### Scenario: macvlan network created with parent
- **WHEN** a deployment with a macvlan network is applied
- **THEN** the reconciler invokes `podman network create --driver macvlan -o parent=eth0 [--subnet ...] <name>`

#### Scenario: bridge network creation is unchanged
- **WHEN** a deployment with a standard bridge network (no driver field) is applied
- **THEN** `podman network create` is called without `--driver`, identical to existing behavior

#### Scenario: NAT/firewall intents not generated for non-bridge networks
- **WHEN** a deployment containing a macvlan network is applied
- **THEN** the hostnet phase does not attempt to configure NAT or firewall rules for that network

#### Scenario: network removed on down
- **WHEN** `vcpe down` tears down a deployment
- **THEN** `podman network rm <bridge>` is invoked for each network bridge after compose services are stopped

#### Scenario: ipv4_address omitted for ipamDriver none networks
- **WHEN** a service has an interface on a network with `ipamDriver: none`
- **THEN** the generated `compose.yaml` service network entry contains `mac_address` but NOT `ipv4_address`
