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
    # Use :- defaults so the function works even when wan/cm aren't declared.
    local wan_dev="${IFACE_WAN_DEVICE:-}"
    local cm_dev="${IFACE_CM_DEVICE:-}"
    local lan_bridge="${LAN_BRIDGE:-brlan0}"
    local erouter_iface="$wan_dev"

    ip link set lo up

    # ── Create and configure manifest-declared bridges ──────────────────────
    # BRIDGE_*_NAME/IPV4 are emitted by the renderer from the manifest's
    # 'bridges' section. Each bridge is created and its members enslaved via
    # IFACE_*_BRIDGE. The IP is assigned from BRIDGE_*_IPV4.
    declare -A bridge_done=()
    while IFS='=' read -r var bridge_name; do
        [[ "$var" == BRIDGE_*_NAME ]] || continue
        [[ -n "$bridge_name" ]] || continue
        ip link add "$bridge_name" type bridge 2>/dev/null || true
        ip link set "$bridge_name" up || true
        bridge_done[$bridge_name]=1
    done < <(env)
    # Enslave interfaces to bridges using IFACE_*_BRIDGE env vars.
    while IFS='=' read -r var bridge_name; do
        [[ "$var" == IFACE_*_BRIDGE ]] || continue
        [[ -n "$bridge_name" ]] || continue
        role_key="${var%_BRIDGE}"; role_key="${role_key#IFACE_}"
        dev_var="IFACE_${role_key}_DEVICE"
        dev="${!dev_var:-}"
        [[ -n "$dev" ]] || continue
        ip link set "$dev" up || true
        ip link set "$dev" master "$bridge_name" || true
        ip addr flush dev "$dev" 2>/dev/null || true
    done < <(env)
    # Configure bridge IPs from BRIDGE_*_IPV4.
    while IFS='=' read -r var cidr; do
        [[ "$var" == BRIDGE_*_IPV4 ]] || continue
        [[ -n "$cidr" ]] || continue
        key="${var%_IPV4}"; name_var="${key}_NAME"
        bridge_name="${!name_var:-}"
        [[ -n "$bridge_name" ]] || continue
        ip addr add "$cidr" dev "$bridge_name" || true
    done < <(env)
    # Also add BRLAN0_IPV4 from config (backward compat when bridges: not set).
    if [[ -z "${bridge_done[$lan_bridge]:-}" && -n "${BRLAN0_IPV4:-}" ]]; then
        ip link add "$lan_bridge" type bridge 2>/dev/null || true
        ip link set "$lan_bridge" up || true
        for lan_if in ${LAN_DEVICES:-}; do
            ip link set "$lan_if" up || true
            ip link set "$lan_if" master "$lan_bridge" || true
            ip addr flush dev "$lan_if" 2>/dev/null || true
        done
        ip addr add "$BRLAN0_IPV4" dev "$lan_bridge" || true
    fi

    # ── CM (cable-modem physical line) — only if declared ──────────────────
    if [[ -n "$cm_dev" ]]; then
        ip link set "$cm_dev" up || true
    fi

    # ── WAN (erouter) — only if declared ───────────────────────────────────
    if [[ -n "$wan_dev" ]]; then
        if [[ -n "${EROUTER0_VLAN:-}" ]]; then
            erouter_iface="${wan_dev}.${EROUTER0_VLAN}"
            ip link add link "$wan_dev" name "$erouter_iface" type vlan id "$EROUTER0_VLAN"
            ip link set "$erouter_iface" up
        else
            ip link set "$wan_dev" up
        fi

        if [[ -n "${EROUTER0_IPV4:-}" ]]; then
            ip addr add "$EROUTER0_IPV4" dev "$erouter_iface" || true
        fi
        if [[ -n "${EROUTER0_IPV6:-}" ]]; then
            ip -6 addr add "$EROUTER0_IPV6" dev "$erouter_iface" || true
        fi
        if [[ -n "${EROUTER0_IPV4_GATEWAY:-}" ]]; then
            ip route replace default via "$EROUTER0_IPV4_GATEWAY" dev "$erouter_iface" || true
        fi
        if [[ -n "${EROUTER0_IPV6_GATEWAY:-}" ]]; then
            ip -6 route replace default via "$EROUTER0_IPV6_GATEWAY" dev "$erouter_iface" || true
        fi
    fi
}

start_lan_dhcp() {
    # Build a single dnsmasq config covering all bridges that have DHCP vars set.
    # Running one process avoids the "Address already in use" conflict that occurs
    # when multiple instances each try to bind 127.0.0.1:53 for DNS.
    local conf="/tmp/dnsmasq-lan.conf"
    local has_config=0

    # Header: shared options
    cat > "$conf" <<'EOF'
no-resolv
bind-dynamic
EOF
    if [[ -n "${BNG_DNS_SERVER:-}" ]]; then
        echo "dhcp-option=6,${BNG_DNS_SERVER}" >> "$conf"
        echo "server=${BNG_DNS_SERVER}" >> "$conf"
    fi

    # Per-bridge DHCP blocks from BRIDGE_*_{NAME,IPV4,DHCP_START,DHCP_END} vars.
    while IFS='=' read -r var bridge_name; do
        [[ "$var" == BRIDGE_*_NAME ]] || continue
        [[ -n "$bridge_name" ]] || continue
        local key="${var%_NAME}"; key="${key#BRIDGE_}"
        local v_start="BRIDGE_${key}_DHCP_START"
        local v_end="BRIDGE_${key}_DHCP_END"
        local v_ip="BRIDGE_${key}_IPV4"
        local dhcp_start="${!v_start:-}"
        local dhcp_end="${!v_end:-}"
        local bridge_ip="${!v_ip:-}"
        [[ -n "$dhcp_start" && -n "$dhcp_end" ]] || continue
        local gw="${bridge_ip%%/*}"
        {
            echo "interface=${bridge_name}"
            echo "dhcp-range=${dhcp_start},${dhcp_end},12h"
            echo "dhcp-option=tag:${bridge_name},3,${gw}"
        } >> "$conf"
        has_config=1
    done < <(env)

    # Legacy fallback: BRLAN0_DHCP_START/END for manifests without bridges: section.
    if [[ $has_config -eq 0 && -n "${BRLAN0_DHCP_START:-}" && -n "${BRLAN0_DHCP_END:-}" ]]; then
        local lan_bridge="${LAN_BRIDGE:-brlan0}"
        local bridge_ip="${BRLAN0_IPV4:-}"
        local gw="${bridge_ip%%/*}"
        {
            echo "interface=${lan_bridge}"
            echo "dhcp-range=${BRLAN0_DHCP_START},${BRLAN0_DHCP_END},12h"
            echo "dhcp-option=3,${gw}"
        } >> "$conf"
        has_config=1
    fi

    [[ $has_config -eq 1 ]] && dnsmasq --conf-file="$conf"
}

main() {
    rename_interfaces_by_mac
    configure_networking
    # NAT all LAN bridge traffic going out via the WAN (erouter) interface so
    # clients can reach the internet and management hosts through the BNG.
    if command -v iptables >/dev/null 2>&1 && [[ -n "${IFACE_WAN_DEVICE:-}" ]]; then
        iptables -t nat -A POSTROUTING -o "${IFACE_WAN_DEVICE}" -j MASQUERADE || true
    fi
    
    if [[ -n "${IFACE_LAN_P1_GATEWAY4:-}" ]]; then
        echo "nameserver ${IFACE_LAN_P1_GATEWAY4}" > /etc/resolv.conf
    elif [[ -n "${BNG_DNS_SERVER:-}" ]]; then
        echo "nameserver ${BNG_DNS_SERVER}" > /etc/resolv.conf
    fi
    start_lan_dhcp
    exec /sbin/init
}

main "$@"