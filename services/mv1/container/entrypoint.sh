#!/bin/bash
set -euo pipefail

rename_interfaces_by_mac() {
    declare -A current_by_mac=()
    declare -A target_by_mac=(
        ["${LAN1_MAC,,}"]=eth0
        ["${LAN2_MAC,,}"]=eth1
        ["${LAN3_MAC,,}"]=eth2
        ["${LAN4_MAC,,}"]=eth3
        ["${WAN0_MAC,,}"]=wan0
        ["${EROUTER0_MAC,,}"]=erouter0
    )
    declare -A temp_by_target=()
    local name
    local mac
    local target
    local temp_name

    for path in /sys/class/net/*; do
        name=$(basename "$path")
        [[ "$name" == lo ]] && continue
        mac=$(cat "$path/address")
        current_by_mac["${mac,,}"]=$name
    done

    for mac in "${!target_by_mac[@]}"; do
        target=${target_by_mac[$mac]}
        [[ -n "${current_by_mac[$mac]:-}" ]] || continue
        if [[ "${current_by_mac[$mac]}" == "$target" ]]; then
            continue
        fi
        temp_name="tmp-${target}"
        ip link set "${current_by_mac[$mac]}" down
        ip link set "${current_by_mac[$mac]}" name "$temp_name"
        temp_by_target[$target]=$temp_name
    done

    for target in eth0 eth1 eth2 eth3 wan0 erouter0; do
        [[ -n "${temp_by_target[$target]:-}" ]] || continue
        ip link set "${temp_by_target[$target]}" name "$target"
    done
}

configure_networking() {
    local erouter_iface=erouter0

    ip link set lo up

    ip link add brlan0 type bridge
    ip link set brlan0 up

    for lan_if in eth0 eth1 eth2 eth3; do
        ip link set "$lan_if" up
        ip link set "$lan_if" master brlan0
    done

    ip addr add "$BRLAN0_IPV4" dev brlan0
    ip -6 addr add "$BRLAN0_IPV6" dev brlan0

    ip link set wan0 up

    if [[ -n "${WAN0_IPV4:-}" ]]; then
        ip addr add "$WAN0_IPV4" dev wan0
    fi

    if [[ -n "${WAN0_IPV6:-}" ]]; then
        ip -6 addr add "$WAN0_IPV6" dev wan0
    fi

    ip link set erouter0 up

    if [[ -n "${EROUTER0_VLAN:-}" ]]; then
        erouter_iface="erouter0.${EROUTER0_VLAN}"
        ip link add link erouter0 name "$erouter_iface" type vlan id "$EROUTER0_VLAN"
        ip link set "$erouter_iface" up
    fi

    ip addr add "$EROUTER0_IPV4" dev "$erouter_iface"
    ip -6 addr add "$EROUTER0_IPV6" dev "$erouter_iface"

    ip route replace default via "$EROUTER0_IPV4_GATEWAY" dev "$erouter_iface"
    ip -6 route replace default via "$EROUTER0_IPV6_GATEWAY" dev "$erouter_iface"
}

main() {
    rename_interfaces_by_mac
    configure_networking
    exec tail -f /dev/null
}

main "$@"