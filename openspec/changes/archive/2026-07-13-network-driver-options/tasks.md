## 1. Manifest Schema

- [x] 1.1 Add `Driver string`, `DriverOptions map[string]string`, and `IPAMDriver string` to `manifest.Network` in `internal/manifest/model.go` with YAML/JSON tags `driver`, `driverOptions`, `ipamDriver` (all `omitempty`)

## 2. Validation

- [x] 2.1 In `validateNetworks()` (`internal/manifest/validate.go`): reject `nat: true` or `firewall: true` when `driver` is non-empty and not `bridge`
- [x] 2.2 In `validateNetworks()`: require `parent` key in `driverOptions` when `driver` is `macvlan` or `ipvlan`
- [x] 2.3 Add validate tests covering: macvlan with parent (valid), macvlan without parent (error), macvlan+nat (error), ipvlan+firewall (error), no driver (unchanged valid)

## 3. Plan Model

- [x] 3.1 Add `Driver string`, `DriverOptions map[string]string`, and `IPAMDriver string` to `plan.Network` in `internal/plan/model.go`

## 4. Planner

- [x] 4.1 In `resolveNetworks()` (`internal/planner/planner.go`): copy `Driver`, `DriverOptions`, `IPAMDriver` from manifest network to plan network
- [x] 4.2 In the `HostBridgeGateway`/`PodmanDNS` derivation block: skip when `n.Driver != "" && n.Driver != "bridge"` (non-bridge drivers have no host bridge to configure)

## 5. NetworkSpec Struct and Interface

- [x] 5.1 Define `NetworkSpec` struct in `internal/backend/podman/adapter.go` with fields: `Name`, `Subnet`, `HostGateway`, `DNS`, `Driver`, `DriverOptions map[string]string`, `IPAMDriver`
- [x] 5.2 Update `EnsureNetwork` signature to `EnsureNetwork(ctx context.Context, spec NetworkSpec) error`
- [x] 5.3 Update `networkProvisioner` interface in `internal/app/orchestrator.go` to match new signature
- [x] 5.4 Update the recording test double in `internal/app/lifecycle_test.go` to match new signature

## 6. Podman Adapter Implementation

- [x] 6.1 In `EnsureNetwork`: add `--driver <spec.Driver>` to args when `spec.Driver` is non-empty
- [x] 6.2 Sort `spec.DriverOptions` keys, then append `-o key=val` for each entry
- [x] 6.3 Add `--ipam-driver <spec.IPAMDriver>` to args when `spec.IPAMDriver` is non-empty
- [x] 6.4 Add adapter unit tests for: no driver (bridge-compatible args unchanged), macvlan+parent, ipvlan+parent+mode, ipamDriver set

## 7. Orchestrator

- [x] 7.1 In the network provisioning loop (`internal/app/orchestrator.go`): build `NetworkSpec` from `plan.Network` and call `EnsureNetwork(ctx, spec)`
- [x] 7.2 In the hostnet intent loop: set `RequiresNAT = false` and `RequiresFirewall = false` for networks where `net.Driver != "" && net.Driver != "bridge"`

## 8. Spec Sync

- [x] 8.1 Apply ADDED requirements from `specs/desired-state-manifests/spec.md` to `openspec/specs/desired-state-manifests/spec.md`
- [x] 8.2 Apply ADDED requirements from `specs/podman-reconciliation-engine/spec.md` to `openspec/specs/podman-reconciliation-engine/spec.md`

## 9. Verification

- [x] 9.1 Run `cd controlplane && go build ./...` — must succeed
- [x] 9.2 Run `cd controlplane && go test ./...` — all tests pass
- [x] 9.3 Run `VCPE_SKIP_IMAGE=1 controlplane/bin/vcpe plan --manifest manifests/example.yaml` — must succeed (bridge networks unchanged)
- [x] 9.4 Verify a manifest with `driver: macvlan` and no `parent` is rejected at preflight with a clear error
- [x] 9.5 Verify a manifest with `driver: macvlan` and `nat: true` is rejected at preflight
