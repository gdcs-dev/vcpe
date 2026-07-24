#!/bin/sh
set -eu

normalize_mac() {
    printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

current_iface_for_mac() {
    wanted=$(normalize_mac "$1")

    for path in /sys/class/net/*; do
        name=$(basename "$path")
        [ "$name" = "lo" ] && continue
        mac=$(tr '[:upper:]' '[:lower:]' < "$path/address")
        if [ "$mac" = "$wanted" ]; then
            printf '%s\n' "$name"
            return 0
        fi
    done

    return 1
}

stage_rename() {
    mac=$1
    target=$2
    iface=$(current_iface_for_mac "$mac" || true)

    [ -n "$iface" ] || return 0
    [ "$iface" = "$target" ] && return 0

    temp="tmp-$target"
    ip link set "$iface" down || true
    ip link set "$iface" name "$temp"
    printf '%s\n' "$temp"
}

finalize_rename() {
    temp_name=${1:-}
    target=$2

    [ -n "$temp_name" ] || return 0
    ip link set "$temp_name" name "$target"
    #ip link set "$target" up || true
}

rename_interfaces_by_mac() {
    # Build rename table from IFACE_*_MAC + IFACE_*_DEVICE env vars.
    # No legacy aliases (LAN1_MAC, EROUTER0_MAC, WAN0_MAC, etc.) are used.
    local var_name mac_val role_key device_var device iface temp_name

    for var_name in $(env | grep '^IFACE_.*_MAC=' | cut -d= -f1); do
        mac_val=$(eval "echo \"\${$var_name}\"")
        [ -n "$mac_val" ] || continue
        role_key="${var_name%_MAC}"
        role_key="${role_key#IFACE_}"
        device_var="IFACE_${role_key}_DEVICE"
        device=$(eval "echo \"\${${device_var}:-}\"")
        [ -n "$device" ] || continue

        iface=$(current_iface_for_mac "$mac_val" || true)
        [ -n "$iface" ] || continue
        [ "$iface" = "$device" ] && continue

        temp_name="tmp-$device"
        ip link set "$iface" down || true
        ip link set "$iface" name "$temp_name"
        ip link set "$temp_name" name "$device"
    done
}

start_dhcp_client() {
    iface=$1
    ip link set "$iface" up || true
    # udhcpc -q -b -i "$iface" -p "/tmp/udhcpc.${iface}.pid" -s /etc/udhcpc.script -x "hostname:$(hostname)"
    cat > /etc/resolv.conf <<EOF
nameserver ${EROUTER0_IPV4_GATEWAY}
EOF
}

rename_interfaces_by_mac
# Use the CM interface device name from the manifest env var.
start_dhcp_client "${IFACE_CM_DEVICE:-cm0}"
exec "$@"