## Overview

This change makes service health a first-class runtime contract. Each curated container hosts a typed health endpoint that evaluates its own process and dependency state. `vcpe` obtains live health only through HTTP requests to deployment-owned loopback endpoints; it does not use Podman inspect, exec, or OCI health state to evaluate a service.

## Components

- **In-container health server**: A small supervised HTTP server exposing `/health` with the common response schema.
- **Type-owned health probe**: Service-specific logic that reports named local and dependency checks.
- **Health endpoint publisher**: Compose rendering allocates and publishes a stable loopback host port for every service instance, then records the endpoint with deployment state.
- **Health registry metadata**: Built-in types declare the health endpoint/probe behavior required for their image and renderer.
- **Status health collector**: `vcpe status` reads expected instances and endpoints from persisted state, invokes them over HTTP with a bounded timeout, and presents the results.
- **WebPA registration authority**: WebPA publishes an authoritative, freshness-bounded registration state that Gateway uses to distinguish reachability from an active connection.

## Key Architectural Decisions

### HTTP health is the control-plane transport
**Choice**: `vcpe` queries per-instance `/health` endpoints over loopback-published HTTP ports.
**Rationale**: It avoids Podman calls for health evaluation, works across the macOS Podman-machine boundary, and gives operators a stable protocol independent of runtime internals.
**Alternatives considered**: Podman inspect or exec was rejected because it couples status to container-runtime commands and provides poor typed diagnostics. A collector container was rejected because it must join each topology and creates another runtime dependency.

### Health semantics are owned by service types
**Choice**: Each registered curated type provides named checks behind the shared response schema; generic containers require explicit opt-in health configuration.
**Rationale**: Readiness is application-specific, and a running generic process is not evidence of readiness.
**Alternatives considered**: A universal TCP/process probe was rejected because it cannot express service dependencies or application registration.

### Gateway registration is verified by WebPA
**Choice**: Gateway reports separate WebPA reachability and registration checks, and its overall readiness requires a recent registration observation from WebPA.
**Rationale**: A reachable Talaria endpoint does not prove that Parodus has an active upstream session.
**Alternatives considered**: Checking Parodus process existence or parsing logs was rejected because neither proves a live registration and both are brittle.

### OCI checks reuse the HTTP contract
**Choice**: Existing and new OCI `HEALTHCHECK` commands request the container-local `/health` endpoint.
**Rationale**: Podman status and `vcpe status` apply identical health semantics without `vcpe` depending on Podman to retrieve them.
**Alternatives considered**: Maintaining shell-only OCI checks in parallel was rejected because health implementations would drift.

## Data Flow

```text
service workload / dependency state
                |
                v
       type-owned health probe
                |
                v
  container-local GET /health (JSON)
                |
                +--> OCI HEALTHCHECK
                |
                +--> loopback-published host port
                              |
                              v
                  persisted deployment endpoint map
                              |
                              v
                  vcpe status --name <deployment>
```

For Gateway:

```text
Gateway health probe ---- HTTP ----> Talaria reachability
          |
          +---- registration query ----> WebPA registration authority
                                               |
                                               v
                              active/recent Gateway identity observation
```

## Integration Points

- `typeregistry.ServiceType` gains health behavior metadata for built-in types.
- Renderers and Compose artifacts publish per-instance loopback health ports.
- Apply persists the expected health endpoint with each resolved instance.
- `runStatus` combines persisted expected instances with HTTP health observations.
- Containerfiles and service supervisors start the health server and direct OCI checks to it.
- WebPA and Gateway exchange a documented registration-state identity and freshness contract.

## Security Model

- Health ports bind to loopback only and are not exposed on deployment networks or external interfaces.
- The endpoint returns operational status only; it MUST NOT return credentials, configuration secrets, tokens, request payloads, or full error logs.
- `vcpe` accepts endpoints only from its own persisted deployment state and validates host/port syntax before requests.
- The WebPA registration query uses the deployment-local trust boundary; any credential required for it stays inside containers and is not surfaced in status output.

## Error Handling Strategy

- An unreachable endpoint is reported as `unknown` with a transport reason, not as a synthetic healthy result.
- A valid response with failed checks is reported as `unhealthy`; a starting response is reported as `starting`.
- Status collection uses a short per-endpoint timeout and continues collecting other instances after failures.
- Apply remains successful when a container needs time to become ready; readiness is observed through `status`, not conflated with reconciliation success.
- Missing health configuration for a generic container is reported as `not-configured`.

## Observability Strategy

- The health server logs probe failures with service and check names, excluding secrets.
- `vcpe status --json` includes overall state, individual named checks, observation time, and bounded diagnostic messages.
- The collector reports endpoint transport errors distinctly from application-reported failures.
- Tests cover response-schema compatibility, endpoint publication, timeout handling, and Gateway registration freshness transitions.

## Constraints

- `vcpe` MUST NOT use `podman inspect`, `podman exec`, or other Podman calls to evaluate health.
- The implementation must support the macOS Podman-machine development path.
- Existing image healthchecks must converge on the common HTTP contract.
- Generic containers cannot be claimed healthy without an explicit configured probe.
- Changes to runtime-init source require restaging the committed Linux/amd64 runtime-init binaries.

## Diagrams

```text
                         +---------------------+
                         |  vcpe status        |
                         |  HTTP collector     |
                         +----------+----------+
                                    |
                     127.0.0.1:allocated-port
                                    |
          +-------------------------+-------------------------+
          |                         |                         |
          v                         v                         v
  +---------------+         +---------------+         +---------------+
  | BNG /health   |         | Gateway       |         | WebPA /health |
  | local checks  |         | /health       |         | local checks  |
  +---------------+         | local + peer  |         +-------+-------+
                            +-------+-------+                 |
                                    |                         |
                                    +---- registration -------+
```