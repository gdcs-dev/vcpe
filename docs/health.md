# Health Reporting

Each curated service publishes `GET /health` on container port `9878`. A valid
response uses the versioned common schema:

```json
{
  "schemaVersion": "vcpe.dev/health/v1",
  "status": "healthy",
  "observedAt": "2026-08-12T12:00:00Z",
  "checks": [{"name": "application", "status": "healthy", "message": "ready"}]
}
```

Endpoint statuses are `healthy`, `starting`, and `unhealthy`. A health endpoint
returns HTTP 200 for all three values so callers can retain its checks and
diagnostics. Diagnostics are intentionally short and never include credentials,
tokens, request data, or logs.

`vcpe` reserves one endpoint for every configured health-enabled service
instance. The resulting Compose mapping always binds `127.0.0.1` on the control
plane host and targets container port `9878`; it is not a manifest port and is
never published on a topology network or wildcard host address.

Publication is automatic and fully control-plane owned: the manifest declares
no interface-level health transport or upstream hint. Every instance of a
registered type with valid health behavior (and every `generic-container`
instance with a configured probe) is published directly from its own workload
network namespace. When none of an instance's topology interfaces already has
a Podman-managed network, the workload is instead attached to the
deployment's private, Podman-managed `aa-health` network so the reserved host
port can still forward to it. Either way, no per-instance health proxy
container is rendered — the workload itself always serves `9878`.

## Status

`vcpe status --name <deployment>` requests only the persisted loopback HTTP
endpoints. It does not use Podman inspect, exec, Compose, or OCI health state to
evaluate readiness. Human and JSON output identify the service and replica and
report one of:

- `healthy`: endpoint returned a valid healthy response.
- `starting`: endpoint returned a valid starting response.
- `unhealthy`: endpoint returned valid failed readiness checks.
- `unknown`: the endpoint was unreachable or returned an invalid response.
- `not-configured`: a generic container has no explicit health probe.

Individual endpoint failures do not stop collection for other replicas.

## Generic Containers

`generic-container` health is opt-in. Declare exactly one HTTP or command probe
and a bounded timeout from 1 to 30 seconds:

```yaml
services:
  - name: application
    type: generic-container
    replicas: 1
    image: { repository: example/application, tag: latest }
    config:
      health:
        http:
          url: http://127.0.0.1:8080/ready
          expectedStatus: 204
        timeoutSeconds: 3
```

For a command probe, replace `http` with:

```yaml
command:
  command: test -f /run/application-ready
```

Configured generic services receive a small health process in the same network
namespace as the workload. Unconfigured generic services have no generated
health endpoint and status reports `not-configured`.

## Gateway And WebPA

WebPA reports separate XMiDT checks for Talaria, Scytale, Tr1d1um, Argus,
Caduceus, and Themis. Gateway reports `interfaces`, `parodus`,
`webpa-reachable`, and `webpa-registration` checks.

Gateway registration uses Talaria's authenticated live device registry at
`GET /api/v2/devices`. It derives the device ID from the colon-stripped
`erouter0` MAC, matching the serial sent by Parodus. The registration is fresh
only while that ID is present in Talaria's current registry; a missing entry
makes Gateway unhealthy even when the WebPA endpoint itself remains reachable.

## Validation Prerequisites

Podman smoke tests require a running Podman machine, local image-pull access,
and health artifacts staged for the machine architecture with
`scripts/stage-runtime-init-binaries`. Generated binaries are stored under each
service's `container/platforms/<os>-<arch>/` directory. The full control-plane
test suite also includes release tests that require the checkout to be on the
`main` branch.