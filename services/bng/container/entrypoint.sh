#!/bin/bash
set -euo pipefail

normalize_mac() {
    printf '%s\n' "$1" | tr '[:upper:]' '[:lower:]'
}

rename_interfaces_by_order() {
    local index=0
    local iface
    local -a ordered=(eth0 eth1 eth2)

    while read -r iface; do
        [[ "$iface" == lo ]] && continue
        [[ $index -lt ${#ordered[@]} ]] || break
        if [[ "$iface" != "${ordered[$index]}" ]]; then
            ip link set "$iface" down || true
            ip link set "$iface" name "${ordered[$index]}" || true
        fi
        index=$((index + 1))
    done < <(ls /sys/class/net | sort)
}

rename_interfaces_by_mac() {
    local iface mac target tmp index=0
    local -A target_by_mac=()
    local -A current_by_target=()
    local -A temp_by_target=()
    local -a ordered=(eth0 eth1 eth2)

    [[ -n "${MGMT_MAC:-}" ]] && target_by_mac["$(normalize_mac "$MGMT_MAC")"]=eth0
    [[ -n "${WAN_MAC:-}" ]] && target_by_mac["$(normalize_mac "$WAN_MAC")"]=eth1
    [[ -n "${CM_MAC:-}" ]] && target_by_mac["$(normalize_mac "$CM_MAC")"]=eth2
    (( ${#target_by_mac[@]} == 3 )) || return 1

    while read -r iface; do
        [[ "$iface" == lo ]] && continue
        mac=$(normalize_mac "$(cat "/sys/class/net/$iface/address")")
        target=${target_by_mac[$mac]:-}
        [[ -n "$target" ]] || continue
        current_by_target["$target"]=$iface
    done < <(ls /sys/class/net | sort)

    for target in "${ordered[@]}"; do
        [[ -n "${current_by_target[$target]:-}" ]] || return 1
    done

    for target in "${ordered[@]}"; do
        iface=${current_by_target[$target]}
        tmp="podtmp${index}"
        index=$((index + 1))
        if [[ "$iface" != "$tmp" ]]; then
            ip link set "$iface" down || true
            ip link set "$iface" name "$tmp"
        fi
        temp_by_target["$target"]=$tmp
    done

    for target in "${ordered[@]}"; do
        ip link set "${temp_by_target[$target]}" name "$target"
    done
}

rename_interfaces() {
    rename_interfaces_by_mac || rename_interfaces_by_order
}

wait_for_ipv6_ready() {
    local attempts=0
    local max_attempts=20

    while ip -6 -o addr show scope global tentative | grep -q .; do
        attempts=$((attempts + 1))
        if (( attempts >= max_attempts )); then
            echo "timed out waiting for IPv6 addresses to leave tentative state" >&2
            ip -6 addr show >&2
            exit 1
        fi
        sleep 0.25
    done
}

apply_runtime_config() {
    mkdir -p /etc/dhcp /var/www/html /usr/local/libexec
    cp /etc/resolv.conf /etc/dnsmasq.upstream-resolv.conf
    cp /runtime-config/etc/dhcp/dhcpd.conf /etc/dhcp/dhcpd.conf
    cp /runtime-config/etc/dhcp/dhcpd6.conf /etc/dhcp/dhcpd6.conf
    cp /runtime-config/etc/dnsmasq.conf /etc/dnsmasq.conf
    cp /runtime-config/etc/dnsmasq.hosts /etc/dnsmasq.hosts
    cp /runtime-config/etc/dnsmasq.dynamic.hosts /etc/dnsmasq.dynamic.hosts
    cp /runtime-config/etc/dnsmasq.dhcp-hosts.map /etc/dnsmasq.dhcp-hosts.map
    cp /runtime-config/etc/dnsmasq.dhcp-subnets.map /etc/dnsmasq.dhcp-subnets.map
    cp /runtime-config/etc/service-interfaces.env /etc/service-interfaces.env
    cp /runtime-config/etc/ntp.conf /etc/ntp.conf
    cp /runtime-config/etc/ports.conf /etc/apache2/ports.conf
    cp /runtime-config/etc/radvd.conf /etc/radvd.conf
    cp /runtime-config/etc/sysctl.conf /etc/sysctl.d/99-bng.conf
    cp /runtime-config/etc/iptables.rules.v4 /etc/iptables.rules.v4
    cp /runtime-config/etc/iptables.rules.v6 /etc/iptables.rules.v6
    cp /runtime-config/etc/network-startup.sh /usr/local/libexec/network-startup.sh
    cp /runtime-config/var/www/html/DCMresponse.txt /var/www/html/DCMresponse.txt
    chmod +x /usr/local/libexec/network-startup.sh
    /usr/local/libexec/network-startup.sh
    wait_for_ipv6_ready
    cat >/etc/resolv.conf <<'EOF'
nameserver 127.0.0.1
options timeout:1 attempts:2
EOF
    sysctl --system >/dev/null
    iptables-restore < /etc/iptables.rules.v4 || true
}

main() {
    [[ -d /runtime-config ]] || { echo "missing /runtime-config" >&2; exit 1; }
    rename_interfaces
    apply_runtime_config
    exec /usr/local/bin/start-services.sh
}

main "$@"