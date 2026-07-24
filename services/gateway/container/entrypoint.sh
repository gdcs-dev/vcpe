#!/bin/bash
set -euo pipefail

rename_interfaces_by_mac() {
    declare -A current_by_mac=()
    declare -A target_by_mac=()
    declare -A temp_by_target=()
    local name mac role_key device_var device

    # Build rename table from IFACE_*_MAC + IFACE_*_DEVICE env vars.
    # No legacy aliases (LAN1_MAC, EROUTER0_MAC, WAN0_MAC) are used.
    while IFS='=' read -r var_name mac_val; do
        [[ "$var_name" == IFACE_*_MAC ]] || continue
        [[ -n "$mac_val" ]] || continue
        role_key="${var_name%_MAC}"
        role_key="${role_key#IFACE_}"
        device_var="IFACE_${role_key}_DEVICE"
        device="${!device_var:-}"
        [[ -n "$device" ]] || continue
        target_by_mac["${mac_val,,}"]="$device"
    done < <(env)

    for path in /sys/class/net/*; do
        name=$(basename "$path")
        [[ "$name" == lo ]] && continue
        mac=$(cat "$path/address")
        current_by_mac["${mac,,}"]=$name
    done

    for mac in "${!target_by_mac[@]}"; do
        local target=${target_by_mac[$mac]}
        [[ -n "${current_by_mac[$mac]:-}" ]] || continue
        if [[ "${current_by_mac[$mac]}" == "$target" ]]; then
            continue
        fi
        local temp_name="tmp-${target}"
        ip link set "${current_by_mac[$mac]}" down
        ip link set "${current_by_mac[$mac]}" name "$temp_name"
        temp_by_target[$target]=$temp_name
    done

    for target in "${!temp_by_target[@]}"; do
        ip link set "${temp_by_target[$target]}" name "$target"
    done
}

configure_networking() {
    # Read interface names from manifest-driven env vars.
    local wan_dev="${IFACE_WAN_DEVICE}"
    local cm_dev="${IFACE_CM_DEVICE}"
    local lan_bridge="${LAN_BRIDGE:-brlan0}"
    local erouter_iface="$wan_dev"

    ip link set lo up

    ip link add "$lan_bridge" type bridge
    ip link set "$lan_bridge" up

    for lan_if in ${LAN_DEVICES:-}; do
        ip link set "$lan_if" up
        ip link set "$lan_if" master "$lan_bridge"
        ip addr flush dev "$lan_if"   # remove Podman IPAM address; only the bridge needs an IP
    done

    ip addr add "$BRLAN0_IPV4" dev "$lan_bridge"
    if [[ -n "${BRLAN0_IPV6:-}" ]]; then
        ip -6 addr add "$BRLAN0_IPV6" dev "$lan_bridge"
    fi

    ip link set "$cm_dev" up

    if [[ -n "${WAN0_IPV4:-}" ]]; then
        ip addr add "$WAN0_IPV4" dev "$cm_dev"
    fi

    if [[ -n "${WAN0_IPV6:-}" ]]; then
        ip -6 addr add "$WAN0_IPV6" dev "$cm_dev"
    fi

    ip link set "$wan_dev" up

    if [[ -n "${EROUTER0_VLAN:-}" ]]; then
        erouter_iface="${wan_dev}.${EROUTER0_VLAN}"
        ip link add link "$wan_dev" name "$erouter_iface" type vlan id "$EROUTER0_VLAN"
        ip link set "$erouter_iface" up
    fi

    ip addr add "$EROUTER0_IPV4" dev "$erouter_iface"
    if [[ -n "${EROUTER0_IPV6:-}" ]]; then
        ip -6 addr add "$EROUTER0_IPV6" dev "$erouter_iface"
    fi

    ip route replace default via "$EROUTER0_IPV4_GATEWAY" dev "$erouter_iface"
    if [[ -n "${EROUTER0_IPV6_GATEWAY:-}" ]]; then
        ip -6 route replace default via "$EROUTER0_IPV6_GATEWAY" dev "$erouter_iface"
    fi
}

start_lan_dhcp() {
    [[ -n "${BRLAN0_DHCP_START:-}" && -n "${BRLAN0_DHCP_END:-}" ]] || return 0
    local gw=${BRLAN0_IPV4%%/*}
    cat > /tmp/dnsmasq-brlan0.conf <<EOF
interface=brlan0
dhcp-range=${BRLAN0_DHCP_START},${BRLAN0_DHCP_END},12h
dhcp-option=3,${gw}
no-resolv
bind-dynamic
EOF
    # Propagate BNG's DNS server to LAN clients so they resolve container
    # hostnames (webpa, etc.) via BNG dnsmasq instead of the Podman bridge.
    if [[ -n "${BNG_DNS_SERVER:-}" ]]; then
        echo "dhcp-option=6,${BNG_DNS_SERVER}" >> /tmp/dnsmasq-brlan0.conf
        # Also configure dnsmasq to forward DNS queries to BNG so that
        # clients with the gateway brlan0 IP as their nameserver resolve
        # container hostnames correctly.
        echo "server=${BNG_DNS_SERVER}" >> /tmp/dnsmasq-brlan0.conf
    fi
    dnsmasq --conf-file=/tmp/dnsmasq-brlan0.conf
}

main() {
    rename_interfaces_by_mac
    configure_networking
    # NAT all LAN bridge traffic going out via the WAN (erouter) interface so
    # clients can reach the internet and management hosts through the BNG.
    if command -v iptables >/dev/null 2>&1; then
        iptables -t nat -A POSTROUTING -o "${IFACE_WAN_DEVICE}" -j MASQUERADE || true
    fi
    # Point resolv.conf at BNG dnsmasq so gateway can resolve peer hostnames.
    if [[ -n "${BNG_DNS_SERVER:-}" ]]; then
        echo "nameserver ${BNG_DNS_SERVER}" > /etc/resolv.conf
    fi
    start_lan_dhcp
    exec /sbin/init
}

main "$@"