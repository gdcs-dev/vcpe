# Architecture

## Goal

Recreate the current BNG lifecycle from `meta-lxd-master/gen/bng-base.sh` and
`meta-lxd-master/gen/bng.sh` using Podman instead of LXD.

## Mapping

- `lxdbr1` becomes `mgmt`
- LXD profile NIC attachments become Podman external networks on host bridges
- `lxc file push` and `lxc exec` become render-time config generation into
  `services/bng/runtime/<customer-id>`
- `bng-base` becomes a Podman image built from `services/bng/Containerfile`

## Phase-1 Runtime Contract

- One multi-service BNG container
- Three attached networks: `mgmt`, `wan`, `cm`
- Customer-specific IPv4 and IPv6 config for `7`, `9`, and `20`
- DHCPv4, DHCPv6, `radvd`, Apache, Mosquitto, and NTP available in-container
- Host-managed bridge and NAT setup

## Known Risk

Deterministic interface naming inside a multi-network Podman container remains a
runtime risk area. The current implementation assigns fixed per-network MAC
addresses in `services/bng/compose.yaml` and renames attached interfaces by
MAC-to-role mapping at container startup, with the older order-based rename kept
only as fallback.