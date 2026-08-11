#!/bin/sh
set -eu

log() { echo "[xb10-init] $*" >&2; }

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
    #
    # Two-pass rename to avoid "RTNETLINK: File exists" when interfaces need
    # to swap names (e.g. eth2 → eth1 while eth1 still exists):
    #   Pass 1 — stage each interface to a tmp-<target> name
    #   Pass 2 — finalize each tmp-<target> to the real target name
    local var_name mac_val role_key device_var device iface temp_name

    log "renaming interfaces by MAC (pass 1: stage to temp names)"
    for var_name in $(env | grep '^IFACE_.*_MAC=' | cut -d= -f1); do
        mac_val=$(eval "echo \"\${$var_name}\"")
        [ -n "$mac_val" ] || continue
        role_key="${var_name%_MAC}"
        role_key="${role_key#IFACE_}"
        device_var="IFACE_${role_key}_DEVICE"
        device=$(eval "echo \"\${${device_var}:-}\"")
        [ -n "$device" ] || continue

        iface=$(current_iface_for_mac "$mac_val" || true)
        [ -n "$iface" ] || { log "  no interface found for MAC $mac_val (role $role_key) — skipping"; continue; }
        if [ "$iface" = "$device" ]; then
            log "  $iface already named correctly (role $role_key)"
            continue
        fi

        temp_name="tmp-$device"
        log "  staging $iface → $temp_name (role $role_key, final target: $device)"
        ip link set "$iface" down || true
        ip link set "$iface" name "$temp_name"
    done

    log "renaming interfaces by MAC (pass 2: finalize temp names)"
    for var_name in $(env | grep '^IFACE_.*_MAC=' | cut -d= -f1); do
        mac_val=$(eval "echo \"\${$var_name}\"")
        [ -n "$mac_val" ] || continue
        role_key="${var_name%_MAC}"
        role_key="${role_key#IFACE_}"
        device_var="IFACE_${role_key}_DEVICE"
        device=$(eval "echo \"\${${device_var}:-}\"")
        [ -n "$device" ] || continue

        temp_name="tmp-$device"
        [ -d "/sys/class/net/$temp_name" ] || continue
        log "  finalizing $temp_name → $device"
        ip link set "$temp_name" name "$device"
    done
}

start_dhcp_client() {
    iface=$1
    log "starting DHCP client on $iface"
    ip link set "$iface" up || true
    # udhcpc -q -b -i "$iface" -p "/tmp/udhcpc.${iface}.pid" -s /etc/udhcpc.script -x "hostname:$(hostname)"
    # Write BNG gateway as dnsmasq's upstream; keep /etc/resolv.conf on local dnsmasq.
    log "writing upstream resolver: nameserver ${EROUTER0_IPV4_GATEWAY}"
    printf 'nameserver %s\n' "${EROUTER0_IPV4_GATEWAY}" > /etc/upstream-resolv.conf
    printf 'nameserver %s\n' "${LAN_DNS:-10.0.0.1}" > /etc/resolv.conf
}

log "xb10 entrypoint starting (CM device: ${IFACE_CM_DEVICE:-cm0})"
rename_interfaces_by_mac
# Use the CM interface device name from the manifest env var.
start_dhcp_client "${IFACE_CM_DEVICE:-cm0}"
log "xb10 init complete, exec: $*"
exec "$@"