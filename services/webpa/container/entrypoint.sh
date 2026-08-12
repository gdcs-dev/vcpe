#!/bin/bash
set -euo pipefail

# ---- Network configuration ------------------------------------------------
# Keep Podman's planned management address. Replacing it with a DHCP lease
# invalidates the address selected for loopback-only published health ports.
ip link set eth0 up

exec /usr/local/bin/vcpe-healthd \
	--probe "talaria=/usr/local/bin/webpa-health-probe talaria" \
	--probe "scytale=/usr/local/bin/webpa-health-probe scytale" \
	--probe "tr1d1um=/usr/local/bin/webpa-health-probe tr1d1um" \
	--probe "argus=/usr/local/bin/webpa-health-probe argus" \
	--probe "caduceus=/usr/local/bin/webpa-health-probe caduceus" \
	--probe "themis=/usr/local/bin/webpa-health-probe themis" \
	--run /usr/local/bin/start-services.sh
