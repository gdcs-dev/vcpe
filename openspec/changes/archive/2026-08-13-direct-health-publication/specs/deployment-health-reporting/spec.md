## MODIFIED Requirements

### Requirement: Per-instance loopback health publication
For every deployed service instance with health support, the system SHALL publish the container health port directly from the workload network namespace only on the control-plane host loopback interface and SHALL persist an endpoint record keyed by deployment name, service name, and replica index. When an instance has no Podman-managed topology attachment suitable for publication, the system SHALL attach the workload to a deployment-private Podman-managed health network and SHALL publish through that attachment without creating a per-instance transport proxy. The system SHALL allocate endpoints without collisions among active deployments and SHALL remove endpoint records when the deployment is torn down.

#### Scenario: Health endpoint is local-only
- **WHEN** the system renders a service instance with health support
- **THEN** its health port is published on a loopback-only host address and is not published on wildcard or topology-network addresses

#### Scenario: Replica endpoints are distinct
- **WHEN** a service has multiple replicas with health support
- **THEN** each replica has a distinct persisted loopback endpoint record

#### Scenario: Self-addressed workload publishes directly
- **WHEN** a health-capable workload has only topology networks whose addresses are container-managed
- **THEN** the workload is attached to the deployment-private managed health network and its own health port is published on the reserved host-loopback port

#### Scenario: Direct publication has no transport proxy
- **WHEN** a workload publishes health through the deployment-private managed health network
- **THEN** the rendered runtime contains no per-instance container whose purpose is to proxy that workload's health endpoint

#### Scenario: Publication does not require runtime discovery
- **WHEN** the control plane renders or collects health for a self-addressed workload
- **THEN** it does not query Podman or Compose to discover a container address or port