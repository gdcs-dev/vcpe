#!/bin/sh
set -eu

case ${1:-} in
interfaces)
    for interface in brlan0 wan0 erouter0; do
        ip link show "$interface" >/dev/null
    done
    ;;
vaps)
    contract=${VCPE_STARTUP_CONTRACT:-/etc/vcpe/startup-contract.json}
    control_dir=${VCPE_HOSTAPD_CTRL_DIR:-/run/vcpe/hostapd/control}
    rows=$(jq -r '.radios[]? | select(.mode == "ap") | .name as $radio | .vaps[]? | [$radio, (.slot | tostring), .device, .bssid, .bridge] | @tsv' "$contract") || exit 1
    if [ -n "$rows" ]; then
        tab=$(printf '\t')
        printf '%s\n' "$rows" | {
            failed=0
            while IFS="$tab" read -r radio slot device expected_bssid expected_bridge; do
                report=$(timeout 2 hostapd_cli -p "$control_dir" -i "$device" status 2>/dev/null) || report=
                state=$(printf '%s\n' "$report" | awk -F= '$1 == "state" {print $2; exit}')
                config=$(timeout 2 hostapd_cli -p "$control_dir" -i "$device" get_config 2>/dev/null) || config=
                bssid=$(printf '%s\n' "$config" | awk -F= '$1 == "bssid" {print $2; exit}')
                link=$(ip -o link show dev "$device" 2>/dev/null) || link=
                bridge=$(printf '%s\n' "$link" | awk '{for (field = 1; field <= NF; field++) if ($field == "master") {print $(field + 1); exit}}')
                enabled=false
                [ "$state" = ENABLED ] && enabled=true
                [ -n "$bssid" ] || bssid=-
                [ -n "$bridge" ] || bridge=-
                jq -nc --arg radio "$radio" --argjson slot "$slot" --arg device "$device" --arg bssid "$bssid" --argjson enabled "$enabled" --arg bridge "$bridge" \
                    '{radio: $radio, slot: $slot, interface: $device, bssid: $bssid, enabled: $enabled, bridge: $bridge}'
                if [ "$enabled" != true ] || [ "$bssid" != "$expected_bssid" ] || [ "$bridge" != "$expected_bridge" ]; then
                    failed=1
                fi
            done
            exit "$failed"
        }
    fi
    ;;
mesh)
    contract=${VCPE_STARTUP_CONTRACT:-/etc/vcpe/startup-contract.json}
    control_dir=${VCPE_MESH_CTRL_DIR:-/run/vcpe/mesh/control}
    rows=$(jq -r '.radios[]? | select(.mode == "mesh") | [.name, .device, .bridge] | @tsv' "$contract") || exit 1
    if [ -n "$rows" ]; then
        tab=$(printf '\t')
        printf '%s\n' "$rows" | {
            failed=0
            while IFS="$tab" read -r radio device expected_bridge; do
                report=$(timeout 2 wpa_cli -p "$control_dir" -i "$device" status 2>/dev/null) || report=
                state=$(printf '%s\n' "$report" | awk -F= '$1 == "wpa_state" {print $2; exit}')
                link=$(ip -o link show dev "$device" 2>/dev/null) || link=
                bridge=$(printf '%s\n' "$link" | awk '{for (field = 1; field <= NF; field++) if ($field == "master") {print $(field + 1); exit}}')
                station_dump=$(iw dev "$device" station dump 2>/dev/null) || station_dump=
                peers=$(printf '%s\n' "$station_dump" | awk '
                    /^Station / {if (established && authorized) count++; established=0; authorized=0}
                    /mesh plink:[[:space:]]+ESTAB([[:space:]]|$)/ {established=1}
                    /authorized:[[:space:]]+yes([[:space:]]|$)/ {authorized=1}
                    END {if (established && authorized) count++; print count+0}
                ')
                joined=false
                [ "$state" = COMPLETED ] && joined=true
                [ -n "$bridge" ] || bridge=-
                jq -nc --arg radio "$radio" --arg interface "$device" --arg bridge "$bridge" --argjson joined "$joined" --argjson peers "$peers" \
                    '{radio: $radio, interface: $interface, bridge: $bridge, joined: $joined, peers: $peers}'
                if [ "$joined" != true ] || [ "$bridge" != "$expected_bridge" ] || [ "$peers" -lt 1 ]; then
                    failed=1
                fi
            done
            exit "$failed"
        }
    fi
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
    echo "usage: $0 {interfaces|vaps|mesh|parodus|webpa-reachable|webpa-registration}" >&2
    exit 2
    ;;
esac