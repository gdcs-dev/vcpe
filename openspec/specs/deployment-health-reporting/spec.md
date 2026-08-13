## Purpose
Define how the control plane publishes, collects, and reports per-instance container health for a deployment, independently of apply success.

## Requirements

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

### Requirement: HTTP-only health collection
`vcpe status --name <deployment>` SHALL request persisted health endpoints over HTTP and SHALL NOT invoke Podman, Compose, or container CLI commands to evaluate service health. The collector SHALL use bounded request timeouts and continue collecting remaining instances after an individual request fails.

#### Scenario: Status performs no runtime command health check
- **WHEN** an operator runs `vcpe status --name <deployment>`
- **THEN** live health is collected solely through HTTP requests to persisted endpoints

#### Scenario: One endpoint is unreachable
- **WHEN** one expected health endpoint cannot be reached before its timeout
- **THEN** status records that instance as `unknown` with a transport reason and returns observations for the remaining instances

### Requirement: Live health status output
`vcpe status --name <deployment>` SHALL display a health observation for every expected service instance. Human-readable output SHALL identify service, replica, and overall health observation. `vcpe status --name <deployment> --json` SHALL include machine-readable per-instance identifiers, observation state, named checks supplied by reachable endpoints, observation timestamp, and transport/protocol errors where applicable.

#### Scenario: Healthy deployment status JSON
- **WHEN** all expected health endpoints return valid healthy responses
- **THEN** JSON status includes a healthy observation and named checks for every expected instance

#### Scenario: Invalid endpoint response
- **WHEN** an endpoint returns malformed JSON or an unsupported health response version
- **THEN** status records that instance as `unknown` with a protocol reason rather than treating it as healthy

### Requirement: Apply and health are independently observable
Applying a valid deployment SHALL NOT fail solely because a newly started instance is still reporting `starting` or has not yet reached healthy status. The subsequent `vcpe status` result SHALL expose that readiness state.

#### Scenario: Slow-starting service
- **WHEN** `vcpe apply` successfully starts a service whose health endpoint reports `starting`
- **THEN** apply succeeds and a subsequent status command reports the instance as `starting`
