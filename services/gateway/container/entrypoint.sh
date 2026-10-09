#!/bin/bash
set -Eeuo pipefail
trap 'echo "gateway-entrypoint: failed at line $LINENO" >&2' ERR

rename_interfaces_by_mac() {
    declare -A current_by_mac=()
    declare -A target_by_mac=()
    declare -A temp_by_target=()
    local name mac role_key device_var device stripped prefix target health_device=eth0
    local net_root="${VCPE_SYS_CLASS_NET_ROOT:-/sys/class/net}"

    # Build one rename table for wired interfaces and late-attached radios.
    while IFS='=' read -r var_name mac_val; do
        case "$var_name" in
            IFACE_*_MAC) prefix="IFACE" ;;
            RADIO_*_MAC) prefix="RADIO" ;;
            *) continue ;;
        esac
        [[ -n "$mac_val" ]] || continue
        role_key="${var_name%_MAC}"
        role_key="${role_key#${prefix}_}"
        device_var="${prefix}_${role_key}_DEVICE"
        device="${!device_var:-}"
        [[ -n "$device" ]] || continue
        target_by_mac["${mac_val,,}"]="$device"
    done < <(env)

    for path in "$net_root"/*; do
        name=$(basename "$path")
        [[ "$name" == lo ]] && continue
        mac=$(cat "$path/address")
        # Kernel tunnel pseudo-devices (gre0, gretap0, erspan0, sit0, ...) are
        # not real network attachments and report an all-zero address (some
        # in 6-octet MAC form, some in a shorter 4-octet form); excluding
        # them keeps this map limited to actual veth/ethernet interfaces so
        # the unmatched-interface search below can't grab one of them
        # instead of the real managed health attachment.
        stripped="${mac//[:.]/}"
        [[ "$stripped" =~ ^0+$ ]] && continue
        local mac_key="${mac,,}"
        local desired_target="${target_by_mac[$mac_key]:-}"
        local existing="${current_by_mac[$mac_key]:-}"
        if [[ -z "$existing" || "$name" == "$desired_target" || ( "$existing" != "$desired_target" && "$name" == "tmp-${desired_target}" ) ]]; then
            current_by_mac[$mac_key]=$name
        fi
    done

    for target in "${target_by_mac[@]}"; do
        [[ "$target" == eth0 ]] && health_device=vcpe-health0
    done
    # Keep Podman's eth0 when no declared interface needs its name so netavark
    # can tear the network down on restart.
    for mac in "${!current_by_mac[@]}"; do
        [[ -n "${target_by_mac[$mac]:-}" ]] && continue
        name=${current_by_mac[$mac]}
        if [[ "$name" != "$health_device" ]]; then
            ip link set "$name" name "$health_device"
        fi
        unset 'current_by_mac[$mac]'
        break
    done

    for mac in "${!target_by_mac[@]}"; do
        local target=${target_by_mac[$mac]}
        [[ -n "${current_by_mac[$mac]:-}" ]] || continue
        if [[ "${current_by_mac[$mac]}" == "$target" ]]; then
            continue
        fi
        local temp_name="tmp-${target}"
        if [[ "${current_by_mac[$mac]}" == "$temp_name" ]]; then
            temp_by_target[$target]=$temp_name
            continue
        fi
        ip link set "${current_by_mac[$mac]}" down
        if ! ip link set "${current_by_mac[$mac]}" name "$temp_name"; then
            echo "gateway-entrypoint: cannot stage interface ${current_by_mac[$mac]} as $temp_name for $target" >&2
            return 1
        fi
        temp_by_target[$target]=$temp_name
    done

    for target in "${!temp_by_target[@]}"; do
        ip link set "${temp_by_target[$target]}" name "$target"
    done

    preserve_health_default_route "$health_device"
}

# preserve_health_default_route keeps the managed aa-health attachment
# reachable for connections that terminate
# locally on its own address, independent of whatever global default route
# configure_networking later installs for erouter0 (gateway's own WAN
# uplink). Without this, a health-check reply whose destination address
# isn't in any locally-attached subnet — e.g. a request forwarded through
# Podman Machine's host<->VM tunnel — falls through to the global default
# route and is black-holed via erouter0 instead of returning via the health
# attachment. It is a no-op when the attachment has no default route.
preserve_health_default_route() {
    local health_device=$1 health_default health_gw health_ip
    health_default=$(ip route show default dev "$health_device" 2>/dev/null | head -1 || true)
    [[ -n "$health_default" ]] || return 0
    health_gw=$(awk '{print $3}' <<<"$health_default")
    health_ip=$(ip -4 -o addr show "$health_device" | awk '{print $4}' | cut -d/ -f1)
    [[ -n "$health_gw" && -n "$health_ip" ]] || return 0
    ip rule add from "$health_ip" table 100 priority 100 2>/dev/null || true
    ip route add default via "$health_gw" dev "$health_device" table 100 2>/dev/null || true
}

configure_networking() {
    # Read interface names from manifest-driven env vars.
    # Use :- defaults so the function works even when wan/cm aren't declared.
    local wan_dev="${IFACE_WAN_DEVICE:-}"
    local cm_dev="${IFACE_CM_DEVICE:-}"
    local lan_bridge="${LAN_BRIDGE:-brlan0}"
    local ready_file="${VCPE_NETWORK_READY_FILE:-/etc/vcpe/gateway-network-ready}"
    local erouter_iface="$wan_dev"
    local var bridge_name attachment_key device_var mode_var dev prefix

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
        local bridge_mac_var="${var%_NAME}_MAC"
        if [[ -n "${!bridge_mac_var:-}" ]]; then
            ip link set "$bridge_name" address "${!bridge_mac_var}"
        fi
        ip link set "$bridge_name" up || true
        bridge_done[$bridge_name]=1
    done < <(env)
    if (( ${#bridge_done[@]} > 1 )); then
        sysctl -q -w net.ipv4.conf.all.arp_ignore=1
    fi
    # Enslave wired interfaces and AP radios to declared bridges.
    while IFS='=' read -r var bridge_name; do
        case "$var" in
            IFACE_*_BRIDGE) prefix="IFACE" ;;
            RADIO_*_VAP_*_BRIDGE) continue ;;
            RADIO_*_BRIDGE) prefix="RADIO" ;;
            *) continue ;;
        esac
        [[ -n "$bridge_name" ]] || continue
        attachment_key="${var%_BRIDGE}"
        if [[ "$prefix" == RADIO ]]; then
            mode_var="${attachment_key}_MODE"
            [[ "${!mode_var:-}" == mesh ]] && continue
        fi
        attachment_key="${attachment_key#${prefix}_}"
        device_var="${prefix}_${attachment_key}_DEVICE"
        dev="${!device_var:-}"
        [[ -n "$dev" ]] || continue
        if [[ "$prefix" == "RADIO" ]]; then
            ip link set "$dev" up
            ip link set "$dev" nomaster 2>/dev/null || true
            ip link set "$dev" master "$bridge_name"
            ip addr flush dev "$dev"
        else
            ip link set "$dev" up || true
            ip link set "$dev" master "$bridge_name" || true
            ip addr flush dev "$dev" 2>/dev/null || true
        fi
    done < <(env)
    while IFS='=' read -r var medium_name; do
        [[ "$var" == RADIO_*_MEDIUM && -n "$medium_name" ]] || continue
        attachment_key="${var%_MEDIUM}"
        mode_var="${attachment_key}_MODE"
        [[ "${!mode_var:-}" == mesh ]] && continue
        device_var="${attachment_key}_DEVICE"
        dev="${!device_var:-}"
        [[ -n "$dev" ]] || continue
        ip link set "$dev" up
        ip link set "$dev" nomaster 2>/dev/null || true
        ip addr flush dev "$dev"
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
        if [[ "${IFACE_CM_ADDRESSING:-dhcp}" == "static" ]]; then
            [[ -n "${IFACE_CM_IPV4:-}" ]] && ip addr add "$IFACE_CM_IPV4" dev "$cm_dev" || true
            [[ -n "${IFACE_CM_IPV6:-}" ]] && ip -6 addr add "$IFACE_CM_IPV6" dev "$cm_dev" || true
        elif [[ -z "${IFACE_CM_NETWORK_MANAGED:-}" ]]; then
            # CM never holds the default route: only WAN/erouter0 is the
            # uplink. Strip any default route the CM lease's router option
            # installs so it can't race with (and win over) WAN's.
            dhclient -v "$cm_dev" || true
            ip route del default dev "$cm_dev" 2>/dev/null || true
        fi
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

        if [[ "${IFACE_WAN_ADDRESSING:-dhcp}" == "static" ]]; then
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
        elif [[ -z "${IFACE_WAN_NETWORK_MANAGED:-}" ]]; then
            dhclient -v "$erouter_iface" || true
        fi
    fi

    mkdir -p "$(dirname "$ready_file")"
    touch "$ready_file"
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
            echo "dhcp-option=tag:${bridge_name},6,${gw}"
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
            echo "dhcp-option=6,${gw}"
        } >> "$conf"
        has_config=1
    fi

    # Note: this must not be a bare `[[ ]] && cmd` statement — under set -e, a
    # false test here would make this the function's (and thus main's) exit
    # status, silently terminating the script before it reaches `exec
    # /sbin/init` whenever no bridge has DHCP configured.
    if [[ $has_config -eq 1 ]]; then
        dnsmasq --conf-file="$conf"
    fi
}

# configure_rdk_otel writes the per-device identity and a dev bearer token
# consumed by otel-relay.service/telemetry-agent-otlp.service's
# EnvironmentFile=/etc/rdk-otel/device.env. Must run after configure_networking
# has renamed the WAN interface to erouter0. The token is generated once and
# then left alone so it stays stable across container restarts.
configure_rdk_otel() {
    mkdir -p /etc/rdk-otel
    local mac
    mac=$(cat /sys/class/net/erouter0/address 2>/dev/null || echo 00:00:00:00:00:00)
    echo "DEVICE_ID=${mac}" > /etc/rdk-otel/device.env
    if [[ ! -s /etc/rdk-otel/otel-token ]]; then
        head -c32 /dev/urandom | od -An -tx1 | tr -d ' \n' > /etc/rdk-otel/otel-token
        echo >> /etc/rdk-otel/otel-token
        chmod 0600 /etc/rdk-otel/otel-token
    fi
}

main() {
    rename_interfaces_by_mac
    configure_networking
    configure_rdk_otel
    # NAT all LAN bridge traffic going out via the WAN (erouter) interface so
    # clients can reach the internet and management hosts through the BNG.
    if command -v iptables >/dev/null 2>&1 && [[ -n "${IFACE_WAN_DEVICE:-}" ]]; then
        iptables -t nat -A POSTROUTING -o "${IFACE_WAN_DEVICE}" -j MASQUERADE || true
    fi
    
    if [[ -n "${LAN_DNS:-}" ]]; then
        echo "nameserver ${LAN_DNS}" > /etc/resolv.conf
    elif [[ -n "${IFACE_LAN_P1_GATEWAY4:-}" ]]; then
        echo "nameserver ${IFACE_LAN_P1_GATEWAY4}" > /etc/resolv.conf
    elif [[ -n "${BNG_DNS_SERVER:-}" ]]; then
        echo "nameserver ${BNG_DNS_SERVER}" > /etc/resolv.conf
    fi
    start_lan_dhcp
    exec /sbin/init
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi