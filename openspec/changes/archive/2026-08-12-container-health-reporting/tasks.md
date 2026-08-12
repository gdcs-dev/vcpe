## 1. Discovery and Contract Foundation

- [x] 1.1 Verify the supported Podman-machine behavior for loopback-only published ports on macOS and add a focused integration probe documenting the result.
- [x] 1.2 Inspect Talaria/WebPA for an existing authoritative Gateway/Parodus registration or session signal; document the identity fields and freshness semantics, or define the minimal explicit registration-observation API when none exists.
- [x] 1.3 Add a versioned Go health-response model with overall/check states, schema validation, bounded diagnostics, and unit tests.
- [x] 1.4 Add health behavior metadata to `typeregistry.ServiceType`, update every built-in registration, and cover registry lookup behavior with tests.

## 2. Endpoint Publication and Persistence

- [x] 2.1 Add a collision-safe persisted health-endpoint reservation keyed by deployment, service, and replica, including cleanup on deployment teardown and backward-compatible handling of deployments with no endpoint record.
- [x] 2.2 Extend plan/render/Compose inputs to map each health-supported instance's container health port to its reserved loopback-only host port; add renderer tests for single and replicated services.
- [x] 2.3 Ensure apply persists expected instance endpoint records before Compose startup and preserves them across non-disruptive reconciles.
- [x] 2.4 Add lifecycle tests proving endpoint reservations are distinct across replicas, are removed on down, and do not expose wildcard/topology-network host ports.
- [x] 2.5 Ensure internally allocated loopback health mappings and operator-declared manifest application mappings coexist without either replacing the other.

## 3. Control-Plane Health Collection

- [x] 3.1 Implement an HTTP-only health collector with endpoint/JSON validation, short bounded timeouts, bounded concurrent requests, and no Podman or Compose command dependency.
- [x] 3.2 Extend `vcpe status --name` human output with per-service/per-replica health observations and distinct `healthy`, `starting`, `unhealthy`, `unknown`, and `not-configured` states.
- [x] 3.3 Extend `vcpe status --name --json` with stable per-instance identifiers, observation timestamps, endpoint checks, and transport/protocol errors.
- [x] 3.4 Add status tests for healthy, starting, unhealthy, unreachable, malformed/unsupported responses, generic not-configured services, and partial collection failures.

## 4. Shared In-Container Health Runtime

- [x] 4.1 Implement a lightweight in-container health HTTP server/probe runner that returns the common `/health` response and safely executes named type-owned checks.
- [x] 4.2 Integrate the health runtime with systemd-backed and startup-script-backed images so it remains supervised after runtime-init transfers control to the workload.
- [x] 4.3 Replace or adapt existing OCI healthcheck scripts so they query container-local `/health` and map non-healthy responses to non-zero exits.
- [x] 4.4 Add unit and image-level tests for response semantics, startup behavior, and OCI-healthcheck parity.

## 5. Curated Service Probes

- [x] 5.1 Add BNG checks for required processes and listeners; update its Containerfile, startup lifecycle, healthcheck script, and tests.
- [x] 5.2 Add WebPA checks for all configured XMiDT endpoints and implement/adapt the authoritative Gateway registration observation API with freshness expiry tests.
- [x] 5.3 Add Gateway checks for required interfaces, Parodus readiness, WebPA reachability, and current WebPA registration; make overall Gateway health require registration success.
- [x] 5.4 Upgrade event-sink `/health` to the common response contract and make successful Argus registration a readiness requirement.
- [x] 5.5 Add Routerd checks for socket, `routerctl status`, and LAN bridge, including its endpoint-backed OCI healthcheck.
- [x] 5.6 Define and implement application-specific readiness probes for XB10 and Oktupus before allowing either to report healthy.
- [x] 5.7 Restage every affected committed `services/*/container/runtime-init-*` Linux/amd64 binary with `scripts/stage-runtime-init-binaries` and verify staged artifacts.
- [x] 5.8 Update BNG and WebPA generated Compose renderers to preserve `input.Service.Ports`, with focused renderer tests.
- [x] 5.9 Replace event-sink's fixed curated Compose port list with a generated or parameterized Compose artifact that preserves `input.Service.Ports`, with focused renderer tests.
- [x] 5.10 Add a table-driven renderer parity test covering BNG, event-sink, gateway, generic-container, oktopus, WebPA, and XB10, proving each carries manifest `ports` into Compose unchanged.
- [x] 5.11 Update `manifests/dev/example.yaml`, `manifests/dev/example-full.yaml`, `manifests/example.yaml`, and `manifests/example-full.yaml` with documented non-conflicting BNG, WebPA, and event-sink application port entries where those services are demonstrated; do not add control-plane health ports to manifests.

## 6. Generic Container and Documentation

- [x] 6.1 Add strict `generic-container` health configuration for HTTP and command probes, including bounded timeouts and validation tests.
- [x] 6.2 Render configured generic probes through the common health endpoint and report services without a probe as `not-configured`.
- [x] 6.3 Document the health response schema, generic probe configuration, status states, loopback-only exposure, and Gateway WebPA registration semantics.

## 7. End-to-End Validation

- [x] 7.1 Add control-plane integration tests that apply a representative deployment and verify `vcpe status` obtains health entirely over HTTP without Podman health inspection/execution.
- [x] 7.2 Add Podman smoke coverage for loopback endpoint publication, endpoint-backed OCI healthchecks, partial health failure reporting, Gateway WebPA registration loss/expiry, and manifest application-port forwarding for each renderer family.
- [x] 7.3 Run targeted Go tests, all control-plane tests, service tests, and the applicable Podman smoke suite; record any environment-dependent prerequisites.