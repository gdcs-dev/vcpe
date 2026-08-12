#!/bin/bash
set -euo pipefail

# addressing: static (IFACE_MGMT_ADDRESSING) keeps Podman's planned/pinned
# address untouched. addressing: dhcp (the default) acquires the address from
# BNG's dnsmasq DHCP server, same as event-sink/webpa. IFACE_MGMT_NETWORK_MANAGED=1
# means Podman itself assigns this network's addresses (e.g. mgmt); a DHCP
# client is not applicable there and dhcp is a no-op.
ip link set eth0 up || true
if [[ "${IFACE_MGMT_ADDRESSING:-dhcp}" != "static" && -z "${IFACE_MGMT_NETWORK_MANAGED:-}" ]]; then
	sleep 2
	ip addr flush dev eth0
	dhclient -v eth0
fi
