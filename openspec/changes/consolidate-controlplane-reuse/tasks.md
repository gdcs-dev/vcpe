## 1. Baseline Characterization

- [x] 1.1 Add a cross-type artifact inventory test covering renderer names, root artifacts, one-based instance paths, and multi-replica output for every registered built-in type; run `go test ./internal/types/...`.
- [x] 1.2 Extend cross-type Compose tests to parse YAML and capture service names, container names, hostnames, ports, health publication, external networks, MAC attachment, and current IPv4 pinning behavior; run `go test ./internal/types/...`.
- [x] 1.3 Add package-local XB10 tests for environment artifacts, replica paths, manifest ports, network attachment maps, volumes, and its addressing exemption; run `go test ./internal/types/xb10`.
- [x] 1.4 Add registry metadata table tests asserting effective health behavior, image policy, interface validation, description, default image, expected roles, and renderer identity for every built-in type; run `go test ./internal/typeregistry ./internal/types/...`.
- [x] 1.5 Add wizard tests that expose the current default-image catalog drift for registered types without changing production behavior; run `go test ./internal/app/wizard` and record the expected failing cases that the registry migration will resolve.
- [x] 1.6 Add image-reference tests that explicitly cover empty repository, omitted tag, whitespace-only tag, and explicit tag at both current call surfaces; run `go test ./internal/render ./internal/image` and record the expected empty-repository mismatch.
- [x] 1.7 Confirm adapter request field parity and add or retain argument tests for Docker and Podman build, pull, push, tag, multi-platform, no-cache, Containerfile, and validation behavior; run `go test ./internal/backend/docker ./internal/backend/podman`.

## 2. Shared Leaf Primitives

- [x] 2.1 Add a dependency-neutral canonical image-reference package with the specified empty-repository and `latest` rules plus focused unit tests; run its package tests.
- [x] 2.2 Delegate `render.ImageRef` and `image.ImageReference` to the canonical formatter without changing their public internal signatures; run `go test ./internal/render ./internal/image`.
- [x] 2.3 Update or remove the characterization case that expected `:latest` for an empty repository and verify all image reference consumers now agree; run `go test ./internal/render ./internal/image ./internal/types/...`.
- [x] 2.4 Add `render.SortedEnv` with deterministic key ordering, input immutability, empty-map behavior, and value-preservation tests; run `go test ./internal/render`.
- [x] 2.5 Add `render.InstanceEnvArtifacts` with root mirror, one-based path, plan-order, trailing-newline, zero-instance, and callback-content tests; run `go test ./internal/render`.
- [x] 2.6 Migrate repeated service environment-map formatting and conventional environment artifact placement one service package at a time, running that package's tests plus `go test ./internal/render` after each edit.

## 3. Service Registry Defaults

- [x] 3.1 Add zero-value `typeregistry.BaseServiceType` defaults for curated health port `9878`, build image policy, and no-op interface validation; run `go test ./internal/typeregistry`.
- [x] 3.2 Migrate BNG, event-sink, gateway, Oktopus, WebPA, and XB10 to the shared defaults one package at a time, running registry metadata tests and the migrated package tests after each edit.
- [x] 3.3 Migrate generic-container to shared defaults while retaining its explicit optional-health override; run `go test ./internal/types/genericcontainer ./internal/typeregistry`.
- [x] 3.4 Add compile-time and runtime completeness assertions proving all built-in types still satisfy the unchanged `ServiceType` contract; run `go test ./internal/typeregistry ./internal/types/...`.
- [x] 3.5 Replace the wizard's hardcoded image repository switch with registry lookup and remove concrete imports used only for image defaults; run `go test ./internal/app/wizard`.
- [x] 3.6 Expand wizard default tests to every registered built-in type, an empty-default type, and an unknown type; run `go test ./internal/app/wizard ./internal/typeregistry`.

## 4. Shared Renderer Lifecycle

- [x] 4.1 Create `internal/render/servicetemplate` with typed hooks, explicit per-instance/interpolated modes, construction validation, single config decode, and renderer identity normalization.
- [x] 4.2 Add template tests for missing name/decoder/hook, unsupported mode, no instances, decode errors, hook errors, one invocation per resolved instance, and one invocation for interpolated mode; run `go test ./internal/render/servicetemplate`.
- [x] 4.3 Implement relative artifact-key validation, rejecting empty keys, absolute paths, and parent traversal; add focused valid/invalid path tests and run the template tests.
- [x] 4.4 Implement root and per-instance artifact placement with first-instance mirroring and duplicate-output conflict detection; add exact artifact tests and run the template tests.
- [x] 4.5 Implement structured Compose fragment parsing and aggregation for `services` and `networks`; test distinct keys, semantically equal duplicate networks, conflicting duplicate services/networks, missing Compose, multiple Compose artifacts, and malformed YAML.
- [x] 4.6 Verify the complete shared renderer package with `go test ./internal/render/...` before migrating any production renderer.

## 5. Simple Renderer Migrations

- [x] 5.1 Migrate WebPA to the shared per-instance renderer lifecycle while keeping config, environment, network attachment, ports, and health behavior local; run `go test ./internal/types/webpa ./internal/render/... ./internal/types`.
- [x] 5.2 Compare WebPA artifact inventory, exact environment bytes, and parsed Compose output against characterization tests; resolve any mismatch before proceeding.
- [x] 5.3 Migrate event-sink to the shared per-instance lifecycle without changing its service policy; run `go test ./internal/types/eventsink ./internal/render/... ./internal/types`.
- [x] 5.4 Compare event-sink artifact inventory and parsed Compose output against characterization tests; resolve any mismatch before proceeding.
- [x] 5.5 Migrate Oktopus to the shared per-instance lifecycle while retaining volume and attachment behavior; run `go test ./internal/types/oktopus ./internal/render/... ./internal/types`.
- [x] 5.6 Compare Oktopus artifact inventory and parsed Compose output against characterization tests; resolve any mismatch before proceeding.

## 6. Complex Curated Renderer Migrations

- [x] 6.1 Migrate gateway to the shared per-instance lifecycle while retaining generated artifacts, computed environment values, health-sidecar topology, network attachments, ports, volumes, and privileges; run `go test ./internal/types/gateway ./internal/render/... ./internal/types`.
- [x] 6.2 Verify gateway first-instance mirrors, per-instance generated artifacts, health behavior, and parsed Compose output against characterization tests before proceeding.
- [x] 6.3 Migrate BNG to the shared per-instance lifecycle while retaining all DHCP, RADVD, DNS, sysctl, firewall, environment, and network-derived artifact content; run `go test ./internal/types/bng ./internal/render/... ./internal/types`.
- [x] 6.4 Verify BNG root mirrors, every per-instance auxiliary artifact, exact generated configuration assertions, and parsed Compose output against characterization tests before proceeding.
- [x] 6.5 Migrate XB10 to the shared per-instance lifecycle while retaining its addressing exemption, network attachment behavior, computed environment, ports, volumes, and privileges; run `go test ./internal/types/xb10 ./internal/render/... ./internal/types`.
- [x] 6.6 Verify all new XB10 package-local compatibility tests and cross-type invariants before proceeding.

## 7. Interpolated Renderer Migration

- [x] 7.1 Add shared lifecycle tests that characterize interpolated-mode pass-through, artifact validation, renderer naming, replica counts, and error propagation; run `go test ./internal/render/servicetemplate`.
- [x] 7.2 Migrate generic-container to interpolated mode without changing `${...}` references, replica naming, MAC behavior, ports, volumes, DNS behavior, entrypoint artifact, or optional health sidecars; run `go test ./internal/types/genericcontainer ./internal/render/... ./internal/types`.
- [x] 7.3 Verify generic-container exact environment and entrypoint expectations plus parsed multi-replica Compose output against characterization tests.
- [x] 7.4 Run all renderer and registry suites together and resolve only consolidation regressions: `go test ./internal/render/... ./internal/typeregistry ./internal/types/...`.

## 8. Image Backend Contract Cleanup

- [x] 8.1 Change Docker image adapter method signatures and argument helpers to consume canonical `image` request types; add a compile-time `image.Backend` assertion and run `go test ./internal/backend/docker ./internal/image`.
- [x] 8.2 Change Podman image adapter method signatures and argument helpers to consume canonical `image` request types; add a compile-time `image.Backend` assertion and run `go test ./internal/backend/podman ./internal/image`.
- [x] 8.3 Update application backend selection to return concrete Docker or Podman adapters directly while preserving the skip-image no-op backend; run focused application image lifecycle tests.
- [x] 8.4 Remove adapter-local image request structs and forwarding-only application backend wrappers after all callers compile; run `go test ./internal/backend/... ./internal/image ./internal/app`.
- [x] 8.5 Verify Docker and Podman command argument arrays and backend-specific diagnostics remain unchanged for all characterized operations.

## 9. Conservative Utility Cleanup

- [x] 9.1 Add `render.IPWithPrefix` with exact tests for empty IP, empty CIDR, invalid CIDR, IPv4, and IPv6; run `go test ./internal/render`.
- [x] 9.2 Migrate gateway and XB10 to `render.IPWithPrefix`, delete their duplicate functions, and run both service package suites.
- [x] 9.3 Establish one IPAM-domain owner for primary-CIDR selection without introducing an app dependency inversion; migrate callers and run `go test ./internal/ipam ./internal/app`.
- [x] 9.4 Reconfirm the Podman `lastUsableIP` implementation has no production callers, delete it, and run `go test ./internal/backend/podman ./internal/planner`.
- [x] 9.5 Search production Go code for superseded renderer loops, adapter request structs, hardcoded default-image catalogs, and exact helper copies; remove only confirmed dead duplicates and rerun affected package tests.

## 10. Test Scaffolding and Documentation

- [x] 10.1 Evaluate repeated test fixture and artifact lookup code after migrations; introduce `internal/types/testsupport` only for exact helpers shared by at least three packages.
- [x] 10.2 Keep service-specific expectations package-local and rerun `go test ./internal/types/...` after any test-helper migration.
- [x] 10.3 Update package comments and developer documentation that describe renderer extension, service registration defaults, image backend implementation, and artifact conventions.
- [x] 10.4 Verify documentation does not claim a universal Compose policy or changed external behavior.

## 11. Final Verification

- [x] 11.1 Run `gofmt` on all changed Go files and obtain clean diagnostics for every touched package.
- [x] 11.2 Run focused subsystem gates: `go test ./internal/render/... ./internal/typeregistry ./internal/types/... ./internal/image ./internal/backend/... ./internal/app/wizard`.
- [x] 11.3 Run `go test ./...` from `controlplane` and document only pre-existing or environment-gated failures with reproduction details.
- [x] 11.4 Run `go build ./...` from `controlplane` and verify all command binaries link.
- [x] 11.5 Run available non-destructive smoke tests; run Podman-backed smoke tests only when a working runtime is available and report skipped gates explicitly.
- [x] 11.6 Review the final diff for generated artifact compatibility, unchanged external APIs, dependency direction, accidental behavior fixes, unrelated edits, and residual duplicate ownership.
- [x] 11.7 Run `openspec validate consolidate-controlplane-reuse` and confirm every implemented requirement has corresponding automated coverage or a documented environment-gated verification.