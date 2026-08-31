## 1. Characterize DNS Boundaries

- [x] 1.1 Add expected-failing BNG renderer tests proving DHCPv4 callback blocks and planned MAC-to-alias mappings are currently absent.
- [x] 1.2 Add expected-failing BNG renderer tests for primary-interface selection, role-qualified aliases, replicas, unknown clients, and duplicate generated alias rejection.
- [x] 1.3 Add expected-failing shared Compose builder tests for BNG-first/Aardvark-second DNS on management services, unchanged BNG DNS, and deployments without an eligible BNG.
- [x] 1.4 Add a shell test harness around `services/bng/assets/dhcpd-notify.sh` covering commit, address move, delayed release, expiry, unknown MAC, and management-host-file preservation.

## 2. Render Planned DHCP Identities

- [x] 2.1 Implement BNG access-role discovery and generate a deterministic MAC identity map for every planned DHCPv4 service interface.
- [x] 2.2 Implement canonical `<service>-<replica>`, bare first-replica, and `<service>-<replica>-<role>` alias assignment with default-route/manifest-order primary selection.
- [x] 2.3 Detect duplicate generated DNS aliases during BNG rendering and return a contextual error before artifacts are written.
- [x] 2.4 Render global ISC DHCP commit, release, and expiry callback blocks that pass event, lease address, and client MAC to the notify script.
- [x] 2.5 Split generated dnsmasq inputs into separately owned management-peer and DHCP-lease host files and update dnsmasq configuration to read both.
- [x] 2.6 Extend BNG golden tests to cover identity-map content, callback syntax, separate host files, and unchanged WebPA virtual CNAME behavior.

## 3. Reconcile Lease Events into DNS

- [x] 3.1 Refactor `dhcpd-notify.sh` to require a planned MAC mapping, ignore client hostnames as DNS identity, and publish every mapped alias for a committed lease.
- [x] 3.2 Serialize callback updates and atomically replace DHCP state and hosts files before signaling dnsmasq.
- [x] 3.3 Make release and expiry removal conditional on matching MAC and address so delayed events cannot delete a renewed lease.
- [x] 3.4 Update the BNG entrypoint to own only the management-peer hosts file and preserve DHCP-owned records across management refreshes and process restarts.
- [x] 3.5 Ensure the BNG image explicitly provides any locking/runtime utility used by the notify script and initializes both host files before dnsmasq starts.
- [x] 3.6 Run the shell harness and verify concurrent callbacks preserve all active records without partial output.

## 4. Route Management DNS through BNG

- [x] 4.1 Add a shared Compose helper that finds an allocated BNG IPv4 address on the same Podman-managed `mgmt` network as the rendered service.
- [x] 4.2 Render service DNS in BNG-first, Podman-gateway/Aardvark-second order for eligible non-BNG services while leaving BNG and BNG-less deployments unchanged.
- [x] 4.3 Apply the shared resolver behavior through `BuildComposeService` so built-in and generic service renderers receive the same policy without per-type duplication.
- [x] 4.4 Extend cross-type Compose tests to verify DNS ordering, same-network matching, BNG self-exclusion, and no behavior change on non-management attachments.

## 5. Integration Validation

- [x] 5.1 Run focused Go tests for `internal/types/bng`, `internal/render/servicetemplate`, and all built-in type renderers; fix only regressions caused by this change.
- [x] 5.2 Build the control-plane binary and BNG image, apply a Gateway deployment, and verify WebPA resolves the bare, instance, and WAN-role aliases to the active lease.
- [x] 5.3 Apply the XB10 deployment and verify WebPA resolves `xb10`, `xb10-1`, and each active role-qualified alias to the corresponding ISC DHCP lease.
- [x] 5.4 In the live deployment, verify management aliases and a public DNS name still resolve, BNG's upstream remains Aardvark, and a release/renew cycle updates records without stale answers.
- [x] 5.5 Add an automated Podman smoke test for cross-network DNS and include it in the release-gate smoke set.

## 6. Documentation and Final Gate

- [x] 6.1 Document the deployment DNS chain, canonical device names, role-qualified names, lease lifecycle, and BNG dependency in `docs/networking.md` and the runbook.
- [x] 6.2 Run `go test ./...` from `controlplane/` and the new DNS smoke test, recording any unrelated pre-existing failures separately.
- [x] 6.3 Run `make release-gate` when Podman integration prerequisites are available and reconcile the completed implementation with this change's spec and task checklist.

## 7. Route WebPA Access Traffic through BNG

- [x] 7.1 Add expected-failing WebPA renderer coverage for route metadata and BNG-less deployments.
- [x] 7.2 Render BNG management next-hop and attached IPv4 CIDRs, then install those routes before WebPA services start.
- [x] 7.3 Extend the cross-network smoke test with a WebPA-to-Gateway TCP probe and run focused validation.