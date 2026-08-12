#!/bin/sh
set -eu

case ${1:-} in
interfaces)
    for interface in brlan0 wan0 erouter0; do
        ip link show "$interface" >/dev/null
    done
    ;;
parodus)
    systemctl is-active --quiet parodus.service
    ;;
webpa-reachable)
    curl -fsS -u "${VCPE_TALARIA_BASIC_AUTH:-user:pass}" \
        "${VCPE_TALARIA_DEVICES_URL:-http://talaria:6200/api/v2/devices}" >/dev/null
    ;;
webpa-registration)
    talaria_url=${VCPE_TALARIA_DEVICES_URL:-http://talaria:6200/api/v2/devices}
    talaria_basic_auth=${VCPE_TALARIA_BASIC_AUTH:-user:pass}
    serial="mac:"${VCPE_HEALTH_SERIAL:-$(tr -d ':' </sys/class/net/erouter0/address)}
    curl -fsS -u "$talaria_basic_auth" "$talaria_url" \
        | jq -e --arg serial "$serial" '.devices[]? | select(.id == $serial)' >/dev/null
    ;;
*)
    echo "usage: $0 {interfaces|parodus|webpa-reachable|webpa-registration}" >&2
    exit 2
    ;;
esac