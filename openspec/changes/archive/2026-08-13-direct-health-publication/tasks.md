## 1. Prove Direct Podman Forwarding

- [x] 1.1 Add an isolated runtime smoke test that starts one HTTP workload with multiple `ipamDriver: none` topology networks plus a managed `aa-health` network, publishes `127.0.0.1:<port>:9878` from the workload, and verifies the response without inspecting the container. Added `tests/smoke/aa-health-multi-network-forwarding.sh`.
- [x] 1.2 Run the forwarding smoke test with local Linux Podman and macOS Podman Machine, recording the Podman/network backend versions and confirming that deterministic `aa-health` ordering preserves the managed forwarding path. Ran on macOS Podman Machine (PASS, netavark 6.0.2) — see `design.md` "Spike Evidence"; no bare-metal Linux Podman host was available in this session, so that leg is unverified rather than failed.
- [x] 1.3 Treat forwarding failure on either supported platform as a design gate: stop implementation, capture the evidence in `design.md`, and choose between a minimum supported Podman version and one relay per deployment before modifying production transport code. No failure observed on the available platform; proceeding per the existing design decision.

## 2. Shared Direct-Publication Rendering

- [x] 2.1 Replace `AttachProxySidecar` with a shared direct-publication helper that adds the allocated loopback mapping to the selected workload and adds `aa-health` only when the instance lacks a Podman-managed topology attachment.
- [x] 2.2 Add focused helper tests for managed-topology publication, private-health-network publication, zero-port no-op behavior, existing application-port preservation, deterministic network alias ordering, and absence of proxy services or dependencies.
- [x] 2.3 Convert built-in service renderers to the direct-publication helper and remove their duplicated health-port construction while leaving unrelated Compose behavior unchanged.
- [x] 2.4 Keep generic-container's namespace-sharing probe helper, ensure only its workload receives networks and published ports, and add parsed-Compose tests proving the probe helper has `network_mode` but no conflicting `networks` or `ports` fields.
- [x] 2.5 Update gateway startup and renderer tests to preserve `vcpe-health0` naming with the direct managed attachment and to assert that no `<instance>-health` proxy service is rendered.

## 3. Automatic Health-Network Orchestration

- [x] 3.1 Update health endpoint reservation so every required-health instance and every optional-health instance with configured probes receives a stable endpoint without checking a manifest transport opt-in.
- [x] 3.2 Keep private health-network provisioning plan-driven: provision `<deployment>-00-health` exactly when at least one published instance has no Podman-managed topology attachment, with no Podman or Compose discovery call.
- [x] 3.3 Replace document-wide health-network mutation with workload-targeted rendering and remove orchestration paths that attach `aa-health` to proxy or namespace-sharing helper services.
- [x] 3.4 Replace orchestration and persistence tests with coverage for self-addressed replicas, collision-free endpoint allocation, repeated apply convergence, teardown record removal, and no health network when all published workloads already have managed topology attachments; do not add compatibility tests for prior health records.
- [x] 3.5 Extend status tests to verify direct endpoints remain HTTP-only, preserve timeout/error isolation, and make no Podman, Compose, or container CLI calls during collection.

## 4. Remove Manifest Transport Hints

- [x] 4.1 Remove `HealthUpstream` from manifest and plan interface models, planner propagation, validation counters, renderer conditions, and associated helper functions.
- [x] 4.2 Add strict manifest tests proving `healthUpstream` is rejected as an unknown field and that a self-addressed health-capable service needs no replacement annotation.
- [x] 4.3 Remove `healthUpstream` from maintained manifests, visual-editor schema/types or examples, documentation, and test fixtures without adding aliases, deprecation handling, or migration documentation for the unreleased field.
- [x] 4.4 Remove `--proxy-url`, proxy probe code, `healthUpstream`, proxy-sidecar helpers, and all associated tests and artifacts; verify repository searches contain no compatibility implementation for the unreleased transport.
- [x] 4.5 Update `docs/health.md` to explain direct workload publication through the private `aa-health` network for self-addressed services, automatic control-plane selection with no manifest transport hint, and the absence of per-instance health proxy containers.

## 5. Verification

- [x] 5.1 Run focused Go tests for manifest, planner, render/servicetemplate, all built-in service types, orchestration, persistence, and health collection; repair only failures caused by this change. Only the pre-existing branch-name-gated `TestRunRelease_CoherenceFailure` fails (expected on non-`main` branches).
- [x] 5.2 Build `controlplane/bin/vcpe` and run the direct-publication smoke against a representative gateway deployment, verifying one workload container per instance, loopback-only reachability, and healthy `vcpe status --name <deployment>` output. Ran against a self-addressed gateway on macOS Podman Machine; surfaced and fixed three pre-existing `services/gateway/container/entrypoint.sh` bugs along the way -- see `design.md` "End-to-end gateway smoke findings".
- [x] 5.3 Run `go test ./...`, `go vet ./...`, `openspec validate direct-health-publication --strict`, and the environment-appropriate release smoke suite; document any pre-existing or platform-gated failures separately. All green except the pre-existing branch-name-gated `TestRunRelease_CoherenceFailure`; `openspec validate --strict` passes; ran `tests/smoke/health-loopback-port.sh`, `tests/smoke/health-endpoint-oci.sh`, and the new `tests/smoke/aa-health-multi-network-forwarding.sh` (all PASS). The full `tests/smoke/controlplane-bng-*.sh` multi-service release-gate suite was not run in this session (out of scope for a single-session apply pass — it stands up the full BNG/gateway/webpa stack); the targeted gateway smoke in task 5.2 already exercised the direct-publication path end to end on real Podman.