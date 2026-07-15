## Context

The immediate bug: `services/gateway/container/50-cloud-init.yaml` has `eth0: dhcp4: yes`. When the gateway container starts, the entrypoint enslaves eth0–eth3 to brlan0 (a Linux bridge). Systemd-networkd then reads the cloud-init-generated `10-netplan-eth0.network` file (`DHCP=ipv4`) and removes eth0 from the bridge to run its own DHCP client, breaking the L2 path between the client container and brlan0 dnsmasq.

The architectural fix: the original vCPE used `ipam_driver=none` on WAN, CM, and LAN networks. Podman creates the network (for L2 adjacency) but assigns no IPs — containers configure their own IPs via entrypoint scripts reading `IFACE_*` env vars from compose.env. This eliminates the Podman-vs-container IP conflict entirely.

vcpe already supports `IPAMDriver` on `manifest.Network` (from `network-driver-options`). The missing pieces are: IPAM skip logic, compose.yaml `ipv4_address` omission, and the gateway container image fix.

## Goals / Non-Goals

**Goals:**
- Fix gateway brlan0 setup so the client receives DHCP leases from dnsmasq.
- `ipamDriver: none` on `wan`, `cm`, `lan-*`: Podman creates the network but assigns no IPs.
- vcpe IPAM still resolves explicit `interfaces[].ipv4` values (BNG/gateway still get their IPs in `IFACE_*` env vars and apply them via entrypoint).
- Generated `compose.yaml` omits `ipv4_address` for `ipamDriver: none` networks; `mac_address` still set.
- `mgmt` network retains Podman IPAM (management interfaces get Podman-assigned IPs).

**Non-Goals:**
- Gateway `erouter0` getting a real DHCP lease from BNG (it still uses a static IP from vcpe IPAM).
- Changing the IPAM database schema.
- Changing any container entrypoint scripts.

## Decisions

**`ipamDriver: none` skips both `CheckConflicts` and `AllocateInterfaces` for that network**
`CheckConflicts` checks CIDR overlaps for networks Podman manages. Networks with `ipamDriver: none` have no Podman-managed CIDR, so they should be excluded from this check (the CIDR is still meaningful to vcpe for env var generation, but Podman doesn't know about it). `AllocateInterfaces` skips pool allocation for interfaces on `ipamDriver: none` networks — the container manages these IPs itself.

**`ipv4_address` omitted; `mac_address` kept**
Podman rejects `ipv4_address` on a network with `--ipam-driver=none`. The MAC is still needed for deterministic interface naming inside the container (MAC → eth0 via the rename function in entrypoints).

**`ipamDriver: none` check uses `plan.Network.IPAMDriver`**
The renderer receives a `plan.Deployment` which carries `IPAMDriver` from the manifest via the planner. Renderers look up the network by role to check if they should omit `ipv4_address`.

**50-cloud-init.yaml fix: remove ethernets section entirely**
With no `[eth0]` entry in cloud-init, netplan generates no `.network` file for eth0, networkd leaves it alone, and the entrypoint bridge setup persists through the init handoff.
