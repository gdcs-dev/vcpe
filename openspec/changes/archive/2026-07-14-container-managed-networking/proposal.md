## Why

The gateway client (`docker.io/library/alpine`) is not receiving DHCP leases because the gateway container's `brlan0` bridge setup is broken by systemd-networkd. The root cause is a conflict between two mechanisms: the gateway entrypoint enslaves `eth0`–`eth3` to `brlan0` as bridge ports, but systemd-networkd reads `10-netplan-eth0.network` (generated from `50-cloud-init.yaml`'s `eth0: dhcp4: yes`) and removes `eth0` from the bridge to run its own DHCP client. This was identified by tracing the original vCPE networking model, which uses `ipam_driver=none` for all container-managed networks (WAN, CM, LAN) so Podman never interferes with IPs on those segments.

## What Changes

- **`services/gateway/container/50-cloud-init.yaml`**: Remove `eth0: dhcp4: yes`. This stops systemd-networkd from generating a `.network` file that fights with the entrypoint's bridge setup. Without an ethernets entry for eth0, networkd leaves it alone and the entrypoint bridge setup persists correctly.
- **`controlplane/internal/ipam/store.go` (`Apply`)**: Skip CIDR conflict-checking for networks with `IPAMDriver: "none"` — these networks have no subnet managed by Podman.
- **`controlplane/internal/ipam/store.go` (`AllocateInterfaces`)**: Skip pool-based address allocation for interfaces on `IPAMDriver: "none"` networks. Explicit `interfaces[].ipv4` values are still honoured (containers apply these from `IFACE_*` env vars themselves).
- **`controlplane/internal/types/bng/bng.go`** and **`gateway/gateway.go`**: When the interface's network has `IPAMDriver: "none"`, omit `ipv4_address` from the generated `compose.yaml` service network entry. The MAC address is still set. The container reads `IFACE_*_IPV4` from compose.env and applies it internally via its entrypoint.
- **`manifests/example.yaml`**: Add `ipamDriver: none` to `wan`, `cm`, `lan-p1`, `lan-p2`, `lan-p3`, `lan-p4`. Management network (`mgmt`) retains Podman IPAM.
- **Rebuild gateway image** after updating `50-cloud-init.yaml`.

## Capabilities

### Modified Capabilities
- `network-driver-options`: When `ipamDriver: none` is set on a network, the vcpe IPAM SHALL skip pool allocation and CIDR conflict-checking for that network. Containers on `ipamDriver: none` networks SHALL apply their own IPs from `IFACE_*` env vars.
- `podman-reconciliation-engine`: When a container interface's network has `ipamDriver: none`, the generated `compose.yaml` SHALL omit `ipv4_address` for that network entry; `mac_address` SHALL still be set.
- `desired-state-manifests`: `ipamDriver: none` on `wan`, `cm`, and `lan-*` networks is the canonical pattern for BNG-owned and gateway-owned IP segments.

## Impact

- **`services/gateway/container/50-cloud-init.yaml`** — remove `eth0: dhcp4: yes`
- **`controlplane/internal/ipam/store.go`** — skip CIDR check and pool alloc for `ipamDriver: none` networks
- **`controlplane/internal/types/bng/bng.go`** — omit `ipv4_address` when network `ipamDriver: none`
- **`controlplane/internal/types/gateway/gateway.go`** — same
- **`controlplane/internal/types/genericcontainer/genericcontainer.go`** — same (client uses generic-container)
- **`manifests/example.yaml`** and **`manifests/example-macvlan.yaml`** — add `ipamDriver: none` to appropriate networks
- Requires `vcpe build` + `vcpe push` to publish updated gateway image
