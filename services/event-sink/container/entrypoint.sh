#!/bin/bash
set -euo pipefail

# ---- Network configuration ------------------------------------------------
# addressing: dhcp (the default) acquires the management IP from BNG's
# dnsmasq DHCP server. Flush the Podman-assigned address first so only the
# DHCP-acquired IP is active. dnsmasq binds the container hostname to the
# assigned IP automatically, making this container resolvable by name from
# the WAN/CM side of the BNG. addressing: static keeps Podman's planned/pinned
# address untouched. IFACE_MGMT_NETWORK_MANAGED=1 means Podman itself assigns
# this network's addresses (e.g. mgmt); a DHCP client is not applicable there
# and dhcp is a no-op.
ip link set eth0 up
if [[ "${IFACE_MGMT_ADDRESSING:-dhcp}" != "static" && -z "${IFACE_MGMT_NETWORK_MANAGED:-}" ]]; then
	sleep 2
	ip addr flush dev eth0
	dhclient -v eth0
fi
# echo "nameserver ${BNG_DNS_SERVER}" > /etc/resolv.conf

exec /usr/local/bin/vcpe-healthd \
	--command /usr/local/bin/event-sink-health-probe \
	--run '/usr/local/bin/event-sink'
