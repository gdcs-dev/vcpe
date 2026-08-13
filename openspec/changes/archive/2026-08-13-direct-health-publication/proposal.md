## Why

Services whose topology interfaces use container-managed addressing cannot currently publish health directly, so `vcpe` deploys a proxy container per affected instance. The proxy duplicates transport that Podman can provide once the workload is attached to the existing private managed health network, increasing resource use and adding another failure mode.

## What Changes

- Attach health-capable workloads without a usable Podman-managed topology attachment to the deployment's shared private health network and publish their own health endpoint on their allocated host-loopback port.
- Remove the per-instance health transport proxy container for services that already provide the standard health endpoint.
- Keep namespace-sharing probe helpers for generic containers that need `vcpe-healthd` to execute a configured command or HTTP probe; these helpers provide probe execution rather than proxy transport.
- Make publication automatic for every health-capable instance instead of requiring an interface-level transport opt-in.
- Remove `services[].interfaces[].healthUpstream` from the unreleased manifest schema because health transport no longer depends on a topology interface or its runtime address.
- Remove proxy-only command behavior, renderer helpers, tests, and health-specific state assumptions rather than retaining compatibility with the unreleased implementation.
- Provide HTTP-only status collection with collision-free loopback endpoints on Linux and macOS Podman Machine environments.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `deployment-health-reporting`: Require direct loopback publication through a private managed health attachment when topology attachments cannot support publication, without a per-instance transport proxy.
- `desired-state-manifests`: Remove the `healthUpstream` interface transport hint and keep health endpoint publication control-plane owned.
- `controlplane-code-reuse`: Replace the shared proxy-sidecar renderer shape with shared direct health-network attachment and port-publication behavior while retaining probe-delegation helpers.

## Impact

- Affected control-plane areas: orchestration health-network decisions, health-port reservation, Compose service rendering, manifest model and validation, gateway rendering, and renderer tests.
- Affected runtime behavior: health-capable workloads may receive one additional private Podman-managed interface; no application or topology ports are exposed beyond host loopback.
- Affected development artifacts: remove `healthUpstream` and proxy-sidecar shapes; no released manifest or health-state compatibility is required.
- No new runtime dependency or Podman inspection path is introduced.