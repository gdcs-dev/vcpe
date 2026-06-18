#!/bin/bash
set -euo pipefail
ip link set eth0 up
ip addr replace 10.10.10.120/24 dev eth0
ip -6 addr replace 2001:dbf:0:1::120/64 dev eth0
ip route replace default via 10.10.10.1
ip -6 route replace default via 2001:dbf:0:1::1

ip link set eth1 up
ip addr flush dev eth1 || true

ip link set eth2 up

ip link add link eth1 name eth1.100 type vlan id 100 || true
ip link set eth1.100 up
ip addr replace 10.120.200.1/24 dev eth1.100
ip -6 addr replace 2001:dae:20:1::129/64 dev eth1.100
