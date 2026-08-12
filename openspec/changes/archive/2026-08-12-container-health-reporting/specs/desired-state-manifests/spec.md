## ADDED Requirements

### Requirement: Service port forwarding parity
The manifest `services[].ports` entry SHALL accept Compose-compatible port mapping strings and SHALL be preserved unchanged in the rendered Compose service definition for every registered service type: `bng`, `event-sink`, `gateway`, `generic-container`, `oktopus`, `webpa`, and `xb10`. Operator-declared port mappings SHALL remain distinct from control-plane-allocated health endpoint mappings.

#### Scenario: WebPA manifest port is rendered
- **WHEN** a WebPA service declares `ports: ["8080:8080"]` in a deployment manifest
- **THEN** the rendered WebPA Compose service contains the same `8080:8080` mapping

#### Scenario: BNG manifest port is rendered
- **WHEN** a BNG service declares a port mapping in a deployment manifest
- **THEN** the rendered BNG Compose service contains that mapping unchanged

#### Scenario: Event-sink manifest port is rendered
- **WHEN** an event-sink service declares a port mapping in a deployment manifest
- **THEN** the Compose service used for event-sink contains that mapping unchanged rather than an empty fixed port list

#### Scenario: All registered types preserve declared ports
- **WHEN** each registered service type is rendered with a valid manifest `ports` list
- **THEN** each rendered Compose service contains the declared mappings unchanged

#### Scenario: Health mappings remain separate
- **WHEN** a health-supported service declares application ports in the manifest
- **THEN** its rendered Compose service retains those declared mappings and also includes its separate loopback-only control-plane health mapping

### Requirement: Representative manifests declare demonstrated application ports
The maintained development and release example manifests that demonstrate host access to BNG, WebPA, or event-sink SHALL declare the required application port mappings in the owning service's `ports` entry. The mappings SHALL be documented by service and SHALL use non-conflicting host ports within each example deployment. Control-plane health endpoint mappings SHALL NOT be added to manifests because they are allocated and rendered by `vcpe`.

#### Scenario: Full example exposes demonstrated services
- **WHEN** an operator applies a maintained full example manifest containing BNG, WebPA, and event-sink
- **THEN** the manifest declares the documented application port mappings for each service intended to be accessed from the host

#### Scenario: Generated health mapping is absent from manifest
- **WHEN** a maintained example manifest is inspected
- **THEN** it contains no hard-coded control-plane health port mapping