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
    ip link set "$target" up || true
}

rename_interfaces_by_mac() {
    temp_eth0=$(stage_rename "$LAN1_MAC" eth0)
    temp_eth1=$(stage_rename "$EROUTER0_MAC" eth1)
    temp_eth2=$(stage_rename "$LAN2_MAC" eth2)
    temp_eth3=$(stage_rename "$LAN3_MAC" eth3)
    temp_eth4=$(stage_rename "$LAN4_MAC" eth4)
    temp_eth5=$(stage_rename "$WAN0_MAC" eth5)

    finalize_rename "$temp_eth0" eth0
    finalize_rename "$temp_eth1" eth1
    finalize_rename "$temp_eth2" eth2
    finalize_rename "$temp_eth3" eth3
    finalize_rename "$temp_eth4" eth4
    finalize_rename "$temp_eth5" eth5
}

start_eth1_dhcp_client() {
    ip link set eth1 up || true
    udhcpc -q -b -i eth1 -p /tmp/udhcpc.eth1.pid -s /etc/udhcpc.script -x "hostname:$(hostname)"
    cat > /etc/resolv.conf <<EOF
nameserver ${EROUTER0_IPV4_GATEWAY}
EOF
}

rename_interfaces_by_mac
start_eth1_dhcp_client
exec "$@"