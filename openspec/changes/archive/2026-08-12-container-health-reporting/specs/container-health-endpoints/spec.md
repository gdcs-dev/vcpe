## ADDED Requirements

### Requirement: Common container health endpoint
Every curated vCPE container image SHALL expose an HTTP `GET /health` endpoint on a container-local health port. The endpoint SHALL return a versioned JSON document containing an overall `status`, an observation timestamp, and named check results. Each check result SHALL include a stable name, status, and a non-sensitive diagnostic message. The endpoint SHALL NOT include credentials, tokens, configuration secrets, request contents, or unbounded logs.

#### Scenario: Healthy response is structured
- **WHEN** all required checks for a curated service pass
- **THEN** `GET /health` returns HTTP 200 with `status: "healthy"`, an observation timestamp, and the passed named checks

#### Scenario: Failed check retains diagnostics
- **WHEN** a required check fails
- **THEN** `GET /health` returns HTTP 200 with `status: "unhealthy"` and identifies the failed check without exposing sensitive data

### Requirement: Type-owned readiness checks
Each curated service type SHALL implement health checks that verify its own application readiness rather than only its container process existence. BNG SHALL verify required service processes and listeners; WebPA SHALL verify its configured XMiDT health endpoints; Routerd SHALL verify its control socket, `routerctl` status, and LAN bridge; and event-sink SHALL verify completed Argus registration in addition to HTTP serving. XB10 and Oktupus SHALL define their own application-specific readiness checks before reporting `healthy`.

#### Scenario: Process liveness alone is insufficient
- **WHEN** a service container is running but one of its required application checks fails
- **THEN** its health endpoint reports `unhealthy`

#### Scenario: Existing event-sink endpoint adopts common contract
- **WHEN** event-sink has started its HTTP server but has not completed Argus registration
- **THEN** its health endpoint reports `starting` or `unhealthy` according to its registration state rather than reporting `healthy` solely because HTTP is listening

### Requirement: Gateway WebPA registration readiness
Gateway health SHALL report distinct `webpa-reachable` and `webpa-registration` checks. Gateway overall health SHALL NOT be `healthy` unless WebPA has authoritatively observed the Gateway's deployment, service, and replica identity as registered/connected within the configured freshness interval.

#### Scenario: Reachable but unregistered Gateway
- **WHEN** Gateway can reach WebPA but WebPA has no current registration observation for the Gateway identity
- **THEN** Gateway reports `webpa-reachable` as passing, `webpa-registration` as failing or starting, and an overall status other than `healthy`

#### Scenario: Fresh Gateway registration
- **WHEN** WebPA has a current registration observation for the Gateway identity
- **THEN** Gateway reports `webpa-registration` as passing

#### Scenario: Expired Gateway registration
- **WHEN** the most recent WebPA registration observation exceeds the configured freshness interval
- **THEN** Gateway reports `webpa-registration` as failing and an overall status other than `healthy`

### Requirement: OCI healthcheck parity
Every curated image's OCI `HEALTHCHECK` SHALL query its container-local `/health` endpoint and SHALL use the endpoint's overall status to determine its exit status. Image healthcheck scripts SHALL NOT maintain an independent set of readiness criteria.

#### Scenario: OCI healthcheck uses endpoint result
- **WHEN** a curated image's `/health` endpoint reports `unhealthy`
- **THEN** its OCI `HEALTHCHECK` exits non-zero

### Requirement: Generic container health opt-in
A `generic-container` service SHALL be reported as `not-configured` unless its manifest declares a valid health probe. The supported probe forms SHALL be an HTTP probe or a command probe with a bounded timeout. A valid configured probe SHALL be exposed through the common health response contract.

#### Scenario: Generic container without probe
- **WHEN** a deployed generic-container declares no health probe
- **THEN** it is reported as `not-configured` and is not represented as healthy based only on container liveness

#### Scenario: Generic container configured HTTP probe
- **WHEN** a deployed generic-container declares a valid HTTP health probe and the probe succeeds
- **THEN** its health endpoint reports the configured named check as passing