## Why

Operators can verify one named receive-enabled Parodus client, but cannot inspect the bounded set of registered clients exposed by the patched Parodus runtime. This makes discovering the correct client name unnecessarily dependent on container-level troubleshooting.

## What Changes

- Add `vcpe diagnose --from <service> --to parodus` to retrieve a bounded, deterministic list of registered Parodus clients from a selected Gateway replica.
- Add a source-local diagnostic journey that queries Parodus through Scytale and returns only validated client names and truncation state through the persisted loopback endpoint.
- Reject incompatible cross-journey diagnostic options, including `--client-service`, before state access or network activity.

## Capabilities

### New Capabilities
- `parodus-client-enumeration`: Bounded, deployment-targeted retrieval of registered Parodus client names through source-owned diagnostic endpoints.

### Modified Capabilities
- `local-control-plane-cli`: Extend `vcpe diagnose` target selection with the Parodus client enumeration journey.

## Impact

- Affects the diagnostic model, provider registry, deployment resolver, CLI validation and help, and `vcpe-healthd` journey wiring.
- Requires the Gateway image's patched Parodus `client-list` route; unsupported or malformed responses remain diagnostic unknown results.