#!/bin/bash
set -euo pipefail

# ---- Network configuration ------------------------------------------------
# addressing: static (IFACE_MGMT_ADDRESSING) keeps Podman's planned/pinned
# address untouched. addressing: dhcp (the default) acquires the address from
# BNG's dnsmasq DHCP server, same as event-sink. IFACE_MGMT_NETWORK_MANAGED=1
# means Podman itself assigns this network's addresses (e.g. mgmt); a DHCP
# client is not applicable there and dhcp is a no-op.
ip link set eth0 up
if [[ "${IFACE_MGMT_ADDRESSING:-dhcp}" != "static" && -z "${IFACE_MGMT_NETWORK_MANAGED:-}" ]]; then
	sleep 2
	ip addr flush dev eth0
	dhclient -v eth0
fi

exec /usr/local/bin/vcpe-healthd \
	--probe "talaria=/usr/local/bin/webpa-health-probe talaria" \
	--probe "scytale=/usr/local/bin/webpa-health-probe scytale" \
	--probe "tr1d1um=/usr/local/bin/webpa-health-probe tr1d1um" \
	--probe "argus=/usr/local/bin/webpa-health-probe argus" \
	--probe "caduceus=/usr/local/bin/webpa-health-probe caduceus" \
	--probe "themis=/usr/local/bin/webpa-health-probe themis" \
	--run /usr/local/bin/start-services.sh
