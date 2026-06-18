#!/bin/bash
set -euo pipefail
ip link set eth0 up
ip addr replace 10.10.10.107/24 dev eth0
ip -6 addr replace 2001:dbf:0:1::107/64 dev eth0
ip route replace default via 10.10.10.1
ip -6 route replace default via 2001:dbf:0:1::1

ip link set eth1 up
ip addr replace 10.107.200.1/24 dev eth1
ip -6 addr replace 2001:dae:7:1::129/64 dev eth1

ip link set eth2 up
ip addr replace 10.107.201.1/24 dev eth2
ip -6 addr replace 2001:daf:7:1::129/64 dev eth2
