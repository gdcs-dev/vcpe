## Why

Podman supports multiple network drivers (`bridge`, `macvlan`, `ipvlan`) and custom IPAM drivers, but the vcpe manifest only ever creates `bridge`-mode networks via hardcoded `podman network create` arguments. There is no way to declare a `macvlan` network with a physical parent interface, which is required to let a BNG container serve DHCP to physical devices on the lab bench (the device connects to a real NIC, gets an IP from the BNG's DHCP server, and behaves exactly as it would against production infrastructure). The `EnsureNetwork` interface has also accumulated a growing positional parameter list that makes extension difficult.

## What Changes

- **`manifest.Network`** gains three optional fields: `driver` (default `""` = bridge), `driverOptions` (map of key/value flags passed as `-o` to `podman network create`), and `ipamDriver` (passthrough, not vcpe-IPAM-integrated).
- **Validation** rejects `nat: true` or `firewall: true` on non-bridge drivers, and requires `parent` in `driverOptions` for `macvlan` and `ipvlan`.
- **`plan.Network`** carries the same three fields through from the manifest.
- **`planner.resolveNetworks`** skips `HostBridgeGateway` and `PodmanDNS` derivation for non-bridge drivers.
- **`networkProvisioner` interface + `podman.EnsureNetwork`**: replace the five-argument positional signature with a `NetworkSpec` struct; add driver, options, and ipam-driver args to the `podman network create` invocation.
- **`hostnet`**: skip NAT/firewall intent generation for non-bridge-driver networks (macvlan/ipvlan bypass the host network stack).
- No changes to IPAM, compose, or service rendering — vcpe assigns container IPs from the manifest pool regardless of driver.

## Capabilities

### New Capabilities
- `network-driver-options`: The manifest `networks[]` entries SHALL accept optional `driver`, `driverOptions`, and `ipamDriver` fields. When `driver` is `macvlan` or `ipvlan`, the network is created with the specified driver and options; `nat` and `firewall` are rejected. The default behavior (no `driver` field) is unchanged.

### Modified Capabilities
- `desired-state-manifests`: The `Network` schema gains `driver`, `driverOptions`, and `ipamDriver` fields; `bridge` is the implicit default.
- `podman-reconciliation-engine`: `EnsureNetwork` accepts a struct (`NetworkSpec`) instead of positional parameters, and passes `--driver`, `-o key=val`, and `--ipam-driver` to `podman network create` when set.

## Impact

- **`internal/manifest/model.go`** — `Network` struct: add `Driver`, `DriverOptions`, `IPAMDriver`
- **`internal/manifest/validate.go`** — driver-specific validation
- **`internal/plan/model.go`** — `Network` struct: add same three fields
- **`internal/planner/planner.go`** — pass-through; skip bridge-specific derivations for non-bridge drivers
- **`internal/app/orchestrator.go`** — build `NetworkSpec`; skip NAT/firewall intent for non-bridge networks
- **`internal/backend/podman/adapter.go`** — `EnsureNetwork` takes `NetworkSpec`; builds `--driver`, `-o`, `--ipam-driver` args
- **Tests** — adapter tests, planner tests, validate tests updated for new fields and struct
- **No changes** to IPAM, compose, service types, runtime-init, or CLI
