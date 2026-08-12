## Context

The current health surface is fragmented: BNG, Gateway, Routerd, and WebPA have image-local shell checks; event-sink exposes a simple HTTP endpoint; XB10, Oktupus, and generic containers do not share a health contract. `vcpe status` reads persisted state and does not query live services. The operator requirement is live, type-specific health without using Podman calls for health checks.

This design implements the architectural choices captured in [architecture.md](architecture.md) and [decisions.md](decisions.md). It introduces one response protocol, per-instance endpoint publication, and a collector that remains independent of Podman health state.

## Goals / Non-Goals

**Goals:**
- Expose one structured `/health` response contract from every curated image.
- Run type-specific probes in the service/container that owns their semantics.
- Publish a loopback-only endpoint for every deployed service instance and retain it with the deployment state.
- Honor operator-declared manifest `ports` consistently for every registered service type.
- Make `vcpe status` collect these endpoints over HTTP and preserve partial results when peers fail.
- Make Gateway report both WebPA reachability and an authoritative fresh registration condition.
- Reuse the endpoint for OCI `HEALTHCHECK` commands.

**Non-Goals:**
- Adding Sensu, a central monitoring server, alert routing, or long-term metric storage.
- Treating container process state as a readiness result.
- Polling health continuously from the control plane or failing `apply` until every instance is ready.
- Exposing health ports outside the local host.
- Inventing a default health assertion for a generic arbitrary image.

## Decisions

### Common response contract and aggregation

Health responses use a versioned JSON body with an overall `status`, observation timestamp, and named checks. The permitted overall values are `healthy`, `starting`, and `unhealthy`; `vcpe` owns the separate observation results `unknown` (transport/protocol failure) and `not-configured` (generic service without a probe). A check contains a stable name, status, and short non-sensitive message.

The endpoint returns HTTP 200 for a valid health response, including `starting` and `unhealthy`, so callers can preserve the detailed body. Invalid requests return normal HTTP errors. OCI healthcheck wrappers convert the overall JSON status into an exit code.

This avoids treating an HTTP transport result as proof of service readiness and lets the CLI distinguish a failed probe from a missing endpoint.

### Endpoint publication and state

Renderers add one loopback-only host-port mapping per service instance, targeting the container health port. Apply records the resulting endpoint by deployment, service, and replica index in persisted state. Teardown removes the mapping with the Compose project and removes the associated state.

The allocator must avoid collisions among active deployments and replicas and must make the same endpoint recoverable across a non-disruptive reconcile. A deterministic allocation based on a persisted reservation is preferred over a random port generated only in Compose output. The health port is never advertised on topology networks or wildcard host addresses.

### Manifest application-port parity

`services[].ports` remains the operator-controlled list of Compose port mappings. Every registered type's renderer SHALL copy these mappings into its generated Compose service definition. BNG and WebPA currently generate Compose without them, while event-sink uses a checked-in Compose file with an empty `ports` list; these paths must be changed to preserve the manifest list. Gateway, XB10, Oktupus, and generic-container already pass the list through and provide the reference behavior.

Manifest application ports are independent from health endpoint publication. The former retain the mapping exactly as declared by the operator; the latter are allocated by the control plane and bind only to loopback. The renderer must compose both sets without overwriting or exposing the health port through an operator-declared wildcard mapping.

Maintained example manifests declare application mappings only for services intentionally demonstrated from the host. The full examples cover BNG, WebPA, and event-sink with non-conflicting documented host ports. They never declare health mappings because those are runtime-owned endpoint reservations.

### In-container health implementation

Curated images gain a lightweight `vcpe-healthd` process or service-specific equivalent that owns the HTTP server and invokes the service's probe implementation. It must survive the runtime-init handoff: runtime-init currently replaces PID 1 with the workload, so images that use systemd supervise healthd as a unit and images using startup scripts launch it alongside the workload.

Existing shell checks become type probe inputs or are replaced by endpoint-backed checks. The OCI `HEALTHCHECK` command calls the loopback endpoint rather than duplicating checks.

### Type-specific checks

- BNG verifies its required service processes and listeners.
- WebPA verifies all configured internal XMiDT health endpoints and supplies the registration authority needed by Gateway.
- Gateway verifies required interfaces, Parodus readiness, WebPA reachability, and an active/recent WebPA registration for its deployment/service/replica identity.
- Event-sink upgrades its existing endpoint to the common response schema and includes completed Argus registration readiness.
- Routerd verifies its socket, `routerctl status`, and LAN bridge.
- XB10 and Oktupus define probes based on their actual main-process readiness before being reported as healthy.
- Generic containers only expose health when a manifest-provided command or HTTP probe is configured.

### Gateway registration source of truth

Talaria's authenticated `GET /api/v3/devices` endpoint is the registration authority. It serializes Talaria's live device registry, so an entry is present only while the WebPA connection is managed by Talaria. Gateway health queries this deployment-local endpoint with the existing Talaria service Basic authentication and reports `webpa-reachable` separately from `webpa-registration`.

Gateway derives the candidate device identity from the same colon-stripped `erouter0` MAC passed as Parodus's `--hw-serial-number`. The integration test against the pinned Talaria image version MUST confirm the exact registry ID representation and use that representation for matching. The registration result is valid only while the identity remains present in the registry; Talaria's device-list cache refresh interval defines the initial freshness bound. Credentials remain inside the Gateway health runtime and are never returned by `/health`.

### Status collection

`vcpe status --name` loads expected endpoints from persisted state and requests each endpoint with a bounded timeout, ideally in parallel with a bounded concurrency limit. It validates response shape and returns an observation for every expected instance even if other requests fail. Human output summarizes per-service/per-replica status; JSON includes endpoint-independent identifiers, observation state, endpoint response, and transport error where applicable.

No status health path invokes Podman, Compose, or container CLI commands.

## Risks / Trade-offs

- [Loopback port forwarding differs under Podman machine] -> Add an integration test on macOS/Podman machine and retain endpoint publication behind a small adapter that can report actionable forwarding failures.
- [Healthd process is not supervised] -> Start it using the service's existing supervisor or startup lifecycle, and test that it survives runtime-init handoff.
- [Gateway identity is ambiguous across replicas] -> Define and persist the identity as deployment, service, and replica index before implementing the ledger.
- [A stale WebPA registration masks a lost connection] -> Require a short, documented freshness interval and test expiry transitions.
- [Probe changes break image-local checks] -> Make OCI checks consume the endpoint only after endpoint tests cover each curated image.
- [Generic probes can execute arbitrary commands] -> Treat generic health configuration as manifest-controlled input, validate it, use explicit timeouts, and do not expose command text or output containing secrets.

## Migration Plan

1. Introduce the common response model, endpoint reservation/state model, and status collector with unit tests.
2. Add healthd and endpoint-backed OCI checks to BNG, WebPA, Gateway, Routerd, and event-sink; restage affected runtime-init binaries.
3. Implement WebPA registration authority and Gateway registration readiness.
4. Add XB10 and Oktupus probes and generic-container opt-in validation/rendering.
5. Make BNG, WebPA, and event-sink preserve manifest application ports; add parity tests for all registered types.
6. Add the documented BNG, WebPA, and event-sink application mappings to maintained example manifests without adding generated health mappings.
7. Update status human/JSON fixtures, service documentation, and Podman integration smoke coverage.

The change is additive to manifests except for optional generic health configuration. Rollback consists of reverting the new images and control-plane binary together; a health-endpoint state record absent from an older state root is treated as `unknown` rather than a migration blocker.

## Open Questions

- What loopback port publication behavior is guaranteed by the supported Podman machine version on macOS?
- Which concrete main-process signals define XB10 and Oktupus readiness?
- Should generic HTTP probes support headers or only URL/expected-status initially? The initial implementation should prefer URL plus expected status unless authentication is required.