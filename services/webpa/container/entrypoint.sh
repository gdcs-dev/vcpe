#!/bin/bash
set -euo pipefail

WEBPA_IPV4=10.10.10.210/24
WEBPA_IPV6=2001:dbf:0:1::210/64
WEBPA_GW4=10.10.10.1
WEBPA_GW6=2001:dbf:0:1::1

# ---- Network configuration ------------------------------------------------
ip link set eth0 up
ip addr replace "$WEBPA_IPV4" dev eth0
ip -6 addr replace "$WEBPA_IPV6" dev eth0
ip route replace default via "$WEBPA_GW4"
ip -6 route replace default via "$WEBPA_GW6"

# Routes to per-customer erouter subnets via their respective BNG mgmt IPs.
# Customer 7  (BNG mgmt 10.10.10.107)
ip route replace 10.107.200.0/24 via 10.10.10.107 || true
ip route replace 10.107.201.0/24 via 10.10.10.107 || true
# Customer 9  (BNG mgmt 10.10.10.109)
ip route replace 10.177.200.0/24 via 10.10.10.109 || true
ip route replace 10.178.200.0/24 via 10.10.10.109 || true
ip route replace 10.179.200.0/24 via 10.10.10.109 || true
# Customer 20 (BNG mgmt 10.10.10.120)
ip route replace 10.120.200.0/24 via 10.10.10.120 || true

exec /usr/local/bin/start-services.sh
