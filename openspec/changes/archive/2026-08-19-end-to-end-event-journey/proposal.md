## Why

The CPE-to-WebPA and webhook callback diagnostics prove adjacent portions of an event path, but an operator still cannot determine whether one representative event crossed both boundaries. Manual correlation across independently generated diagnostic evidence is slow and can misattribute a failure. A single bounded journey is needed to answer whether a selected CPE application's event reached the cloud callback.

## What Changes

- Add an `end-to-end-event-journey` diagnostic that composes the existing CPE-to-WebPA and webhook-registration/callback checks around one explicitly requested, correlation-marked representative event.
- Add `vcpe diagnose --name <deployment> --from <cpe-service> --to callback` with required client-service, subscriber, event, and device selection, plus an explicit active-event consent flag.
- Correlate source-owned CPE/Parodus evidence, WebPA/Talaria processing, Argus registration selection, Caduceus routing, and the subscriber callback receipt into one ordered diagnostic graph with a single earliest confirmed failure.
- Reuse persisted loopback diagnostic endpoints and bounded HTTP protocols; do not inspect or execute containers, parse unbounded logs, expose secrets, or accept arbitrary endpoints or event payloads.
- Generate only a reserved, bounded diagnostic event after prerequisite connectivity and registration checks pass. The event must be distinguishable from normal application traffic and must not be processed as a normal subscriber event.
- Report inconclusive outcomes when a required observation cannot be established, rather than claiming end-to-end delivery from partial success.
- Exclude tracing arbitrary production events, arbitrary callback payload validation, continuous telemetry, delivery performance testing, and visual-editor overlays.

## Capabilities

### New Capabilities
- `end-to-end-event-journey`: Correlated, bounded diagnosis of a representative event from a supported CPE application through WebPA event routing to a registered cloud callback.

### Modified Capabilities
- `local-control-plane-cli`: Extend `vcpe diagnose` with the callback journey, its required selection flags, and explicit active-event consent semantics.

## Impact

- Extends `controlplane/internal/diagnostic` with a composed multi-participant journey, correlation model, causal graph stages, and orchestration across persisted loopback endpoints.
- Extends `controlplane/internal/app` CLI parsing, validation, dispatch, help, and human/JSON output for the callback journey.
- Extends diagnostic-only event handling in Gateway, WebPA, and event-sink while preserving normal health checks, application events, webhook registration, and callback processing.
- Requires the completed `cpe-webpa-connectivity-diagnostics` and `webhook-registration-callback-diagnostics` foundations to be available; does not change the manifest schema, normal event-routing contracts, or container-runtime independence of diagnostics.
- Adds unit, protocol, integration, security, and opt-in deployed smoke coverage for successful correlation and failures at application, Talaria, registration, routing, callback, and receipt boundaries.