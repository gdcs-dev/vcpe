# Networking

## Bridges

Phase 1 creates and verifies these host bridges:

- `mgmt`
- `wan`
- `cm`
- `lan-p1` .. `lan-p4`
- `wlan0`, `wlan1`
- `wanoe`

`mgmt` replaces the old `lxdbr1` and owns:

- IPv4: `10.10.10.1/24`
- IPv6: `2001:dbf:0:1::1/64`

## VLAN Filtering

These bridges are created as VLAN-aware bridges:

- `lan-p1` .. `lan-p4`
- `wlan0`, `wlan1`
- `wanoe`

## NAT

The phase-1 setup installs the same management-side IPv4 NAT rule shape that
the LXD workflow used for `lxdbr1`:

```bash
iptables -t nat -A POSTROUTING -s 10.10.10.0/24 ! -o wan ! -d 10.10.10.0/24 -j MASQUERADE
```

## Privileges

Host bridge and firewall setup require root privileges. These scripts use
`sudo` when not already running as root.