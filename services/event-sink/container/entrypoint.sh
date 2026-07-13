#!/bin/bash
set -euo pipefail

# ---- Network configuration ------------------------------------------------
# Acquire the management IP from BNG's dnsmasq DHCP server. Flush the
# Podman-assigned address first so only the DHCP-acquired IP is active.
# dnsmasq binds the container hostname to the assigned IP automatically,
# making this container resolvable by name from the WAN/CM side of the BNG.
sleep 2
ip link set eth0 up
ip addr flush dev eth0
dhclient -v eth0
# echo "nameserver ${BNG_DNS_SERVER}" > /etc/resolv.conf

exec /usr/local/bin/event-sink "$@"
