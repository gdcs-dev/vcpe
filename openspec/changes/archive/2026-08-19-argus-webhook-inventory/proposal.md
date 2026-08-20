## Why

An operator can diagnose one event-sink registration today, but cannot inspect the authoritative set of webhook registrations Argus holds for a deployment. That makes registrations created by other services, stale deployments, or external tooling invisible without direct Argus access.

## What Changes

- Add a passive `vcpe diagnose --name <deployment> --from <webpa-service> --to webhooks` journey that inventories registrations in the WebPA-local Argus `webhooks` bucket.
- Return a deterministic, bounded, non-secret representation of every safely decoded Argus registration, irrespective of the service or tool that created it.
- Add structured JSON and human-readable inventory output while preserving the existing subscriber-specific `--to webhook` registration and callback diagnostic.
- Reject incompatible subscriber, client-service, replica, and active-event flags before state access or diagnostic HTTP requests.

## Capabilities

### New Capabilities
- `argus-webhook-inventory`: Deployment-targeted, WebPA-owned inventory of authoritative Argus webhook registrations.

### Modified Capabilities
- `local-control-plane-cli`: Add the `webhooks` diagnose target and its target-specific command grammar and help.

## Impact

- `controlplane/internal/app`: CLI parsing, help, and diagnostic invocation wiring.
- `controlplane/internal/diagnostic`: journey registry, source-local protocol, result model, rendering, validation, and tests.
- `controlplane/cmd/vcpe-healthd`: WebPA diagnostic journey registration and bounded Argus inventory execution.
- Existing Argus/chrysom compatibility code is reused; no new external service, credential, container-runtime, or callback traffic is introduced.