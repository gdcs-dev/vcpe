# Networking

## Manifest-declared networks

Topology comes from `spec.networks[]`. Each network has a `role`, an optional
`bridge` (default `<metadata.name>-<role>`), `nat`/`firewall` flags, and optional
`ipv4` and `ipv6` blocks. Each address-family block may define a `cidr`, an
optional `gateway` (default: first usable host in the CIDR), and an optional
`pool` range. A network may declare either or both families, supporting
IPv4-only, IPv6-only, or dual-stack:

```yaml
spec:
  networks:
    - role: wan
      bridge: edge-wan        # optional; default <metadata.name>-<role>
      nat: true
      firewall: true
      ipv4: { cidr: 10.7.200.0/24, gateway: 10.7.200.1, pool: { start: 10.7.200.10, end: 10.7.200.250 } }
      ipv6: { cidr: "2001:dae:7:1::/64" }
```

## Interfaces

Services attach to networks through `services[].interfaces[]`. An interface binds
to a network by `role`, with an optional `device` (default: ordered `eth<n>`), an
optional `mac`, and at most one `ipv4` and one `ipv6` address. The gateway is
inherited from the network. Explicit `mac`/`ipv4`/`ipv6` are valid only when
`replicas: 1`; with `replicas > 1`, IPAM allocates per replica and MACs are
indexed.

At most one interface per service may set `defaultRoute: true`; if none does, the
interface bound to the `wan` role is the default route.

## Deterministic MAC and bridge naming

- **MAC**: an interface without an explicit `mac` gets a deterministic,
  locally-administered MAC derived from `metadata.name/service/role[/index]`
  (`02:` prefix). The planner and runtime-init contract derive it identically.
- **Bridge/interface names**: derived names are capped at the 15-character kernel
  limit (IFNAMSIZ). When `<metadata.name>-<role>` overflows, the name is
  truncated with a deterministic hash suffix; an explicit name over 15 characters
  produces a warning.

## NAT

Networks with `nat: true` are masqueraded via the host default uplink. The
masquerade source CIDR is the network's CIDR; egress uplink is an environment
concern, not a manifest field.

## Canonical interface environment contract

For each interface role, the renderer emits a canonical environment contract that
curated compose files consume (the role is upper-cased with `-` replaced by `_`,
e.g. `lan-p1` -> `LAN_P1`):

```
IFACE_<ROLE>_NETWORK
IFACE_<ROLE>_DEVICE
IFACE_<ROLE>_MAC
IFACE_<ROLE>_IPV4
IFACE_<ROLE>_IPV6
IFACE_<ROLE>_GATEWAY4
IFACE_<ROLE>_GATEWAY6
```

plus the deployment-level `DEPLOYMENT_NAME`, `SERVICE_NAME`, and `IMAGE`. Curated
`services/{bng,gateway,webpa}/compose.yaml` read these variables; `generic-container`
services have their compose document generated from the manifest.

## Privileges

Host bridge and firewall setup require root privileges. On macOS, `vcpe`
delegates these to the Podman machine Linux host automatically.
