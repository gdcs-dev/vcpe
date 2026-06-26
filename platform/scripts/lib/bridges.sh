#!/bin/bash

MGMT_BRIDGE=mgmt
MGMT_IPV4_CIDR=10.10.10.1/24
MGMT_IPV6_CIDR=2001:dbf:0:1::1/64
CUSTOMER_NETWORK_IDS=(7 9 20)

# LAN access ports are bridged together inside each gateway router (brlan0). They
# must therefore be per-customer: a shared LAN bridge would tie every router's
# internal bridge into a single layer-2 loop and cause a broadcast storm.
LAN_PORTS=(lan-p1 lan-p2 lan-p3 lan-p4)

# Host bridges that are not scoped per customer.
STATIC_BRIDGES=(mgmt wan cm wlan0 wlan1 wanoe)
STATIC_VLAN_BRIDGES=(wlan0 wlan1 wanoe)
BASE_PODMAN_NETWORKS=(mgmt)

customer_wan_network() {
    local customer_id=$1
    printf 'wan-%s\n' "$customer_id"
}

customer_cm_network() {
    local customer_id=$1
    printf 'cm-%s\n' "$customer_id"
}

customer_lan_network() {
    local lan_port=$1
    local customer_id=$2
    printf '%s-%s\n' "$lan_port" "$customer_id"
}

# Every host bridge to create: the static set plus a per-customer bridge for
# each LAN access port.
host_bridges() {
    local bridge_name
    for bridge_name in "${STATIC_BRIDGES[@]}"; do
        printf '%s\n' "$bridge_name"
    done

    local customer_id
    local lan_port
    for customer_id in "${CUSTOMER_NETWORK_IDS[@]}"; do
        for lan_port in "${LAN_PORTS[@]}"; do
            customer_lan_network "$lan_port" "$customer_id"
        done
    done
}

podman_networks() {
    local network_name
    for network_name in "${BASE_PODMAN_NETWORKS[@]}"; do
        printf '%s\n' "$network_name"
    done

    local customer_id
    local lan_port
    for customer_id in "${CUSTOMER_NETWORK_IDS[@]}"; do
        customer_wan_network "$customer_id"
        customer_cm_network "$customer_id"
        for lan_port in "${LAN_PORTS[@]}"; do
            customer_lan_network "$lan_port" "$customer_id"
        done
    done
}

purge_netavark_isolation() {
    local -a handles=()
    local index
    local handle

    while IFS= read -r handle; do
        handles+=("$handle")
    done < <(
        run_linux_host_root nft -a list chain inet netavark NETAVARK-ISOLATION-3 2>/dev/null |
            sed -n 's/.* drop # handle \([0-9][0-9]*\)$/\1/p'
    )

    (( ${#handles[@]} > 0 )) || return 0

    for (( index=${#handles[@]} - 1; index >= 0; index-- )); do
        run_linux_host_root nft delete rule inet netavark NETAVARK-ISOLATION-3 handle "${handles[index]}"
    done
}

network_is_internal() {
    local network_name=$1
    [[ "$network_name" == "$MGMT_BRIDGE" ]]
}

network_disable_isolation() {
    local network_name=$1
    [[ "$network_name" != "$MGMT_BRIDGE" ]]
}

bridge_has_vlan_filtering() {
    local bridge_name=$1
    if [[ " ${STATIC_VLAN_BRIDGES[*]} " == *" $bridge_name "* ]]; then
        return 0
    fi
    # Per-customer LAN bridges (lan-pN-<id>) keep VLAN filtering enabled.
    [[ "$bridge_name" == lan-p[1-4]-* ]]
}

create_bridge() {
    local bridge_name=$1
    if ! run_linux_host ip link show "$bridge_name" >/dev/null 2>&1; then
        if bridge_has_vlan_filtering "$bridge_name"; then
            run_linux_host_root ip link add name "$bridge_name" type bridge vlan_filtering 1 vlan_default_pvid 1
        else
            run_linux_host_root ip link add name "$bridge_name" type bridge
        fi
    fi
    run_linux_host_root ip link set "$bridge_name" up
}

configure_mgmt_bridge() {
    run_linux_host_root ip addr replace "$MGMT_IPV4_CIDR" dev "$MGMT_BRIDGE"
    run_linux_host_root ip -6 addr replace "$MGMT_IPV6_CIDR" dev "$MGMT_BRIDGE"
    run_linux_host_root sysctl -w "net.ipv4.conf.${MGMT_BRIDGE}.forwarding=1" >/dev/null
    run_linux_host_root sysctl -w "net.ipv6.conf.${MGMT_BRIDGE}.accept_ra=0" >/dev/null
}

configure_non_mgmt_bridge() {
    local bridge_name=$1
    run_linux_host_root ip addr flush dev "$bridge_name" || true
    run_linux_host_root sysctl -w "net.ipv6.conf.${bridge_name}.accept_ra=0" >/dev/null || true
}

ensure_podman_network() {
    local network_name=$1

    if podman network exists "$network_name"; then
        return 0
    fi

    if network_is_internal "$network_name"; then
        if network_disable_isolation "$network_name"; then
            podman network create \
                --driver bridge \
                --disable-dns \
                --internal \
                -o isolate=false \
                --ipam-driver none \
                --interface-name "$network_name" \
                "$network_name" >/dev/null
        else
            podman network create \
                --driver bridge \
                --disable-dns \
                --internal \
                --ipam-driver none \
                --interface-name "$network_name" \
                "$network_name" >/dev/null
        fi
        return 0
    fi

    if network_disable_isolation "$network_name"; then
        podman network create \
            --driver bridge \
            --disable-dns \
            -o isolate=false \
            --ipam-driver none \
            --interface-name "$network_name" \
            "$network_name" >/dev/null
        return 0
    fi

    podman network create \
        --driver bridge \
        --disable-dns \
        --ipam-driver none \
        --interface-name "$network_name" \
        "$network_name" >/dev/null
}

cleanup_podman_network() {
    local network_name=$1
    if podman network exists "$network_name"; then
        podman network rm -f "$network_name" >/dev/null || true
    fi
}

setup_bridges() {
    require_cmd podman
    local bridge_name
    while read -r bridge_name; do
        create_bridge "$bridge_name"
        if [[ "$bridge_name" == "$MGMT_BRIDGE" ]]; then
            configure_mgmt_bridge
        else
            configure_non_mgmt_bridge "$bridge_name"
        fi
    done < <(host_bridges)
    while read -r network_name; do
        ensure_podman_network "$network_name"
    done < <(podman_networks)

    purge_netavark_isolation
}

verify_bridges() {
    local missing=0
    local bridge_name
    while read -r bridge_name; do
        if ! run_linux_host ip link show "$bridge_name" >/dev/null 2>&1; then
            warn "missing bridge: $bridge_name"
            missing=1
            continue
        fi
        if bridge_has_vlan_filtering "$bridge_name"; then
            local vlan_filtering=0
            if vlan_filtering=$(run_linux_host cat "/sys/class/net/${bridge_name}/bridge/vlan_filtering" 2>/dev/null); then
                :
            else
                vlan_filtering=0
            fi
            if [[ "$vlan_filtering" != 1 ]]; then
                warn "vlan filtering disabled on: $bridge_name"
                missing=1
            fi
        fi
    done < <(host_bridges)
    run_linux_host ip addr show "$MGMT_BRIDGE" | grep -q '10.10.10.1/24' || { warn "mgmt IPv4 missing"; missing=1; }
    run_linux_host ip -6 addr show "$MGMT_BRIDGE" | grep -q '2001:dbf:0:1::1/64' || { warn "mgmt IPv6 missing"; missing=1; }
    while read -r network_name; do
        podman network exists "$network_name" || { warn "missing podman network: $network_name"; missing=1; }
    done < <(podman_networks)
    (( missing == 0 )) || die "bridge verification failed"
}

cleanup_bridges() {
    local bridge_name
    local network_name
    while read -r network_name; do
        cleanup_podman_network "$network_name"
    done < <(podman_networks)
    while read -r bridge_name; do
        if run_linux_host ip link show "$bridge_name" >/dev/null 2>&1; then
            run_linux_host_root ip link set "$bridge_name" down || true
            run_linux_host_root ip link delete "$bridge_name" type bridge || true
        fi
    done < <(host_bridges)
}