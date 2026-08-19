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

## Connectivity Diagnostics

Gateway and XB10 expose diagnostics on the same loopback-published endpoint as
health. The routes are separate so status remains passive:

- `GET /health` returns the existing readiness document and never runs an
  active diagnostic journey.
- `GET /diagnostics` passively lists supported journey IDs.
- `POST /diagnostics/cpe-webpa` runs bounded source-local checks for the CPE to
  WebPA journey.

Run the journey with:

```bash
vcpe diagnose --name example-full --from gateway --to webpa --client-service apparmor-simulator
vcpe diagnose --name example-full --from gateway --to webpa --client-service my-test-app --json
```

The source service must be a Gateway or XB10. When it has multiple replicas,
add `--replica <zero-based-index>`. vCPE reads the source instance's persisted
loopback endpoint and communicates only over HTTP; it does not use Podman,
Docker, Compose, container discovery, or container exec to collect evidence.

The graph reports `passed`, `failed`, `unknown`, and `skipped` edges. A failed
or unknown blocking prerequisite skips dependent checks. The application to
Parodus edge is non-blocking and queries Parodus's existing client list through
a direct Scytale WRP Retrieve. `--client-service <name>` is required and must
match the application's libparodus `service_name`. There is no environment or
image default. The value must be a stable identifier and cannot
contain a slash or whitespace; vCPE supplies only this final service-name
segment to the source endpoint. `online` passes, `offline` fails, and a failed or
invalid Scytale query is unknown while DNS, transport, authentication, and
Talaria registration checks continue. Send-only libparodus clients configured
with `receive=false` do not register in this list and cannot be proven by this
check.

The command exits zero only when all edges pass. A completed graph containing
a failure or unknown edge is still printed but exits non-zero. Transport,
protocol, topology, and invocation errors also exit non-zero but do not present
a partial graph as complete.

Diagnostic requests cannot provide commands, credentials, device IDs, WRP
destination prefixes, probe definitions, or target URLs. Responses are
size-limited, strictly decoded, redacted, and
exclude raw logs and credentials.

### Parodus Client Enumeration

Gateway and XB10 sources can inventory the receive-enabled application clients
registered with their own Parodus instance without a WebPA service:

```bash
vcpe diagnose --name example-full --from gateway --to parodus --json
vcpe diagnose --name example-full --from xb10 --to parodus --replica 0 --json
```

The command uses the selected source instance's persisted loopback diagnostic
endpoint. That source owns the derived device identity, Scytale configuration,
and credentials used to retrieve its bounded `<device>/parodus/client-list`.
The result lists at most 64 sorted registered client-service identifiers and
reports whether Parodus truncated the list. It is an inventory of
receive-enabled registrations, not an active callback delivery diagnostic;
the Gateway and XB10 active callback journey remains a separate capability.

### Webhook Registration And Callback Diagnostics

The `event-sink` subscriber type supports a separate WebPA-owned webhook
journey. It reads the subscriber's intended registration and authoritative
Argus state through the two persisted loopback endpoints; the control plane
does not inspect containers or run Podman during collection.

Passive mode is the default and sends no callback traffic:

```bash
vcpe diagnose --name example-full --from event-sink --to webhook
vcpe diagnose --name example-full --from event-sink --to webhook --json
```

It verifies subscriber intent, Argus reachability and authentication, one
matching registration, freshness, and conformance. The direct-callback and
Caduceus stages remain visible as `unknown` with
`active-callback-not-requested`, so a healthy passive result is intentionally
inconclusive and exits nonzero.

Active mode is explicit because it generates one signed direct callback and
one synthetic Caduceus event. Supply a representative destination and device
identity that match the configured event filter and device matcher:

```bash
vcpe diagnose --name example-full --from event-sink --to webhook \
  --allow-active-callback --event apparmor/diagnostic \
  --device-id mac:001122334455 --json
```

`--event` and `--device-id` are required only with
`--allow-active-callback`; the command rejects either active-only field without
consent. `--client-service` belongs to the CPE-to-WebPA journey and is rejected
for `--to webhook`. A fully healthy active graph passes the registration,
direct callback DNS/transport/acceptance, Caduceus ingestion, and correlated
subscriber-receipt stages.

Diagnostic data is bounded: the WebPA participant considers at most eight
Argus candidates, requests and responses have fixed size limits, and receipt
polling has a fixed attempt bound. Output contains only safe registration
fingerprints, HTTP statuses, correlation state, and timestamps. It never
emits webhook secrets, HMAC values, Basic authentication, raw Argus items,
callback bodies, or unbounded logs. Event-sink recognizes correctly signed
diagnostic markers, records an expiring in-memory receipt, returns HTTP 204,
and does not log or process the marker as an ordinary application event.

### CPE-To-Subscriber Callback Diagnosis

`--to callback` correlates one bounded, reserved diagnostic event from a
supported Gateway or XB10 application through its existing Parodus path,
WebPA/Caduceus routing, and one selected `event-sink` subscriber receipt. Each
source exposes the fixed root-only AppArmor simulator diagnostic socket; the
subscriber must be an `event-sink` service.

This journey generates traffic only after explicit consent and all required
selections are validated before vCPE reads state or invokes a diagnostic
endpoint:

```bash
vcpe diagnose --name example-full --from gateway --to callback \
  --client-service apparmor-simulator --subscriber event-sink \
  --allow-active-event --event apparmor/diagnostic \
  --device-id mac:001122334455 --json

vcpe diagnose --name xb10 --from xb10 --to callback \
  --client-service apparmor-simulator --subscriber event-sink \
  --allow-active-event --event devices/diagnostic \
  --device-id mac:001122334455 --json
```

Use `--replica <zero-based-index>` for a multi-replica Gateway or XB10 and
`--subscriber-replica <zero-based-index>` for a multi-replica event-sink. The
command rejects missing or cross-journey fields, arbitrary callback URLs,
credentials, event bodies, endpoints, and executable commands. It generates at
most one marked event for an invocation.

The ordered stages are CPE application/Parodus evidence, Talaria DNS,
transport, authentication, and registration, subscriber intent and Argus
registration validation, CPE event acceptance, Caduceus routing observation,
and the matching signed callback receipt. The receipt is the only proof of
end-to-end delivery. A missing routing record or receipt, including receipt
state lost during an event-sink restart, is `unknown` and therefore
inconclusive; it is not a confirmed callback rejection.

The control plane uses only the selected participants' persisted loopback
diagnostic endpoints. Correlation identifiers, WRP bodies, webhook secrets,
signatures, and credentials remain local to workloads and are never emitted in
ASCII or JSON output. This is a one-event diagnostic probe, not continuous
telemetry or tracing of arbitrary production events.

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