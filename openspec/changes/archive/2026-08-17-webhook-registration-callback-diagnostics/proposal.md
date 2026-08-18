## Why

Cloud application developers can see that an event callback did not arrive, but currently must correlate event-sink state, Argus records, Caduceus behavior, DNS, authentication, and callback logs manually. vCPE should show the expected webhook path, identify the first confirmed registration or delivery failure, and provide bounded remediation without exposing webhook secrets or parsing unbounded logs.

## What Changes

- Add `vcpe diagnose --name <deployment> --from <subscriber-service> --to webhook` for diagnosing a deployed subscriber's Argus registration and Caduceus callback path.
- Model the expected path as subscriber intent to Argus reachability/authentication, stored registration presence and freshness, Caduceus registration loading, callback DNS and transport, signature validation, and callback HTTP acceptance.
- Reuse the versioned diagnostic graph and human/JSON rendering contracts established by CPE-to-WebPA diagnostics, including passed, failed, unknown, skipped, and first-failure semantics.
- Collect evidence exclusively through persisted loopback HTTP diagnostic endpoints; the control plane does not inspect or execute containers.
- Coordinate evidence from the subscriber endpoint and the deployment's WebPA endpoint because callback delivery must be observed from the Caduceus network namespace rather than inferred from subscriber-local reachability.
- Run a bounded synthetic signed callback from the WebPA/Caduceus side using the stored Argus registration, with an explicit diagnostic marker that the subscriber acknowledges without treating it as an application event.
- Compare intended subscriber registration with the authoritative Argus item, including callback URL, event filter, device matcher, and TTL freshness, while never returning the stored secret.
- Require `--allow-active-callback` before sending a synthetic callback; without it, registration stages run and callback-delivery stages are reported as not exercised rather than inferred.
- Require representative `--event <destination>` and `--device-id <id>` values with active mode so Caduceus filter and matcher selection are tested rather than guessed.
- Emit deterministic ASCII and stable `vcpe.dev/diagnostic/v1` JSON from one validated graph model.
- Exclude CPE-origin event generation, end-to-end event correlation, webhook payload correctness for arbitrary application schemas, callback performance testing, and VS Code graph overlays. Those remain follow-up capabilities.

## Capabilities

### New Capabilities
- `webhook-registration-callback-diagnostics`: Multi-participant HTTP diagnosis of subscriber intent, authoritative Argus registration, Caduceus loading, and bounded signed callback delivery.

### Modified Capabilities
- `local-control-plane-cli`: Extend `vcpe diagnose` with the `--to webhook` journey and the explicit `--allow-active-callback` safety flag.

## Impact

- Extends `controlplane/internal/diagnostic` with a webhook journey provider, multi-endpoint collection, journey-specific invocation, graph stages, and remediation mappings.
- Extends `controlplane/internal/app` parsing, help, dispatch, output, and exit behavior for the webhook journey.
- Extends the common loopback diagnostic HTTP protocol while preserving `GET /health` and passive capability discovery.
- Adds subscriber-owned diagnostic state to event-sink for intended registration and receipt of marked diagnostic callbacks.
- Adds WebPA-owned diagnostic handling for bounded Argus inspection, Caduceus registration evidence, and active signed callback delivery from the correct network namespace.
- Adds focused unit, HTTP contract, security, integration, and deployed smoke coverage for healthy, missing, stale, mismatched, unreachable, authentication, signature, and callback-status failures.
- Does not change the manifest schema, normal webhook registration format, normal event handling, common health schema, or container-runtime independence of diagnostics.