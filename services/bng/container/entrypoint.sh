#!/bin/bash
set -euo pipefail

normalize_mac() {
    printf '%s\n' "$1" | tr '[:upper:]' '[:lower:]'
}

rename_interfaces_by_order() {
    # Fallback: rename in interface index order using IFACE_*_DEVICE values.
    # Collect device names in sorted role order as a stable sequence.
    local -a ordered=()
    while IFS='=' read -r var_name device_val; do
        [[ "$var_name" == IFACE_*_DEVICE ]] || continue
        [[ -n "$device_val" ]] || continue
        ordered+=("$device_val")
    done < <(env | sort)

    # If no IFACE_*_DEVICE vars found, fall back to eth0 eth1 eth2.
    if (( ${#ordered[@]} == 0 )); then
        ordered=(eth0 eth1 eth2)
    fi

    local index=0
    local iface
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
    local iface mac role_key device_var device
    local -A target_by_mac=()    # mac → desired device name
    local -A current_by_mac=()   # mac → current kernel name
    local -A temp_by_target=()   # desired device name → temp name

    # Build rename table from IFACE_*_MAC + IFACE_*_DEVICE env vars.
    while IFS='=' read -r var_name mac_val; do
        [[ "$var_name" == IFACE_*_MAC ]] || continue
        [[ -n "$mac_val" ]] || continue
        role_key="${var_name%_MAC}"
        role_key="${role_key#IFACE_}"
        device_var="IFACE_${role_key}_DEVICE"
        device="${!device_var:-}"
        [[ -n "$device" ]] || continue
        target_by_mac["$(normalize_mac "$mac_val")"]="$device"
    done < <(env)

    (( ${#target_by_mac[@]} > 0 )) || return 1

    # Map current kernel interface names by their MAC address.
    while read -r iface; do
        [[ "$iface" == lo ]] && continue
        mac=$(normalize_mac "$(cat "/sys/class/net/$iface/address")")
        current_by_mac["$mac"]=$iface
    done < <(ls /sys/class/net | sort)

    # Verify all target MACs are visible on the system.
    for mac in "${!target_by_mac[@]}"; do
        [[ -n "${current_by_mac[$mac]:-}" ]] || return 1
    done

    # Two-phase rename to avoid name collisions (e.g. eth0→eth1, eth1→eth0).
    local idx=0
    for mac in "${!target_by_mac[@]}"; do
        local target="${target_by_mac[$mac]}"
        iface="${current_by_mac[$mac]}"
        [[ "$iface" == "$target" ]] && continue  # already correct
        local tmp="podtmp${idx}"
        idx=$((idx + 1))
        ip link set "$iface" down || true
        ip link set "$iface" name "$tmp"
        temp_by_target["$target"]=$tmp
    done

    for target in "${!temp_by_target[@]}"; do
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

    # Before dnsmasq overwrites resolv.conf, resolve each peer hostname from
    # dnsmasq.hosts via Podman's aardvark-dns (which knows each container's
    # actual runtime IP). Write the resolved entries to dnsmasq.dynamic.hosts
    # and clear the static file so dnsmasq never serves stale planned IPs.
    : > /etc/dnsmasq.dynamic.hosts
    while IFS= read -r line || [[ -n "$line" ]]; do
        [[ -z "$line" || "$line" == '#'* ]] && continue
        first_hostname=$(printf '%s' "$line" | awk '{print $2}')
        [[ -z "$first_hostname" ]] && continue
        actual_ip=$(getent hosts "$first_hostname" 2>/dev/null | awk '{print $1}' | head -1 || true)
        if [[ -n "$actual_ip" ]]; then
            rest=$(printf '%s' "$line" | cut -d' ' -f2-)
            printf '%s %s\n' "$actual_ip" "$rest" >> /etc/dnsmasq.dynamic.hosts
        fi
    done < /etc/dnsmasq.hosts
    : > /etc/dnsmasq.hosts

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