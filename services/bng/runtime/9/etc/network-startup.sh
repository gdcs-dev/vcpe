#!/bin/bash
set -euo pipefail
ip link set eth0 up
ip addr replace 10.10.10.109/24 dev eth0
ip -6 addr replace 2001:dbf:0:1::109/64 dev eth0
ip route replace default via 10.10.10.1
ip -6 route replace default via 2001:dbf:0:1::1

ip link set eth1 up
ip addr flush dev eth1 || true

ip link set eth2 up

ip link add link eth1 name eth1.1081 type vlan id 1081 || true
ip link set eth1.1081 up
ip addr replace 10.178.200.1/24 dev eth1.1081
ip -6 addr replace 2001:dbe:0:1::129/64 dev eth1.1081

ip link add link eth1 name eth1.881 type vlan id 881 || true
ip link set eth1.881 up
ip addr replace 10.177.200.1/24 dev eth1.881
ip -6 addr replace 2001:dbd:0:1::129/64 dev eth1.881

ip link add link eth1 name eth1.981 type vlan id 981 || true
ip link set eth1.981 up
ip addr replace 10.179.200.1/24 dev eth1.981
ip -6 addr replace 2001:dbc:0:1::129/64 dev eth1.981
