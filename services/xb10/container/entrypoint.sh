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

mac_is_manifest_interface() {
    wanted=$(normalize_mac "$1")

    for var_name in $(env | grep '^IFACE_.*_MAC=' | cut -d= -f1); do
        mac_val=$(eval "echo \"\${$var_name}\"")
        [ "$(normalize_mac "$mac_val")" = "$wanted" ] && return 0
    done

    return 1
}

move_health_interface() {
    [ ! -d /sys/class/net/vcpe-health0 ] || return 0

    for path in /sys/class/net/*; do
        name=$(basename "$path")
        [ "$name" = "lo" ] && continue
        mac=$(tr '[:upper:]' '[:lower:]' < "$path/address")
        stripped=$(printf '%s' "$mac" | tr -d ':.' | tr -d '0')
        [ -n "$stripped" ] || continue
        mac_is_manifest_interface "$mac" && continue

        log "  moving unmatched interface $name → vcpe-health0"
        ip link set "$name" name vcpe-health0
        return 0
    done
}

preserve_health_default_route() {
    health_default=$(ip route show default dev vcpe-health0 2>/dev/null | head -1)
    [ -n "$health_default" ] || return 0
    health_gateway=$(printf '%s\n' "$health_default" | awk '{print $3}')
    health_ip=$(ip -4 -o addr show vcpe-health0 | awk '{print $4}' | cut -d/ -f1)
    [ -n "$health_gateway" ] && [ -n "$health_ip" ] || return 0
    ip rule add from "$health_ip" table 100 priority 100 2>/dev/null || true
    ip route add default via "$health_gateway" dev vcpe-health0 table 100 2>/dev/null || true
}

rename_interfaces_by_mac() {
    # Build rename table from IFACE_*_MAC + IFACE_*_DEVICE env vars.
    # No legacy aliases (LAN1_MAC, EROUTER0_MAC, WAN0_MAC, etc.) are used.
    #
    # Two-pass rename to avoid "RTNETLINK: File exists" when interfaces need
    # to swap names (e.g. eth2 → eth1 while eth1 still exists):
    #   Pass 1 — stage each interface to a tmp-<target> name
    #   Pass 2 — finalize each tmp-<target> to the real target name
    move_health_interface

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
        ip link set "$iface" down || true
        #ip link set "$device" up || true
    done

    preserve_health_default_route
}

update_resolver() {
    log "writing local resolver: nameserver ${LAN_DNS:-10.0.0.1}"
    printf 'nameserver %s\n' "${LAN_DNS:-10.0.0.1}" > /etc/resolv.conf
}

log "xb10 entrypoint starting (CM device: ${IFACE_CM_DEVICE:-cm0})"
rename_interfaces_by_mac
update_resolver
log "xb10 init complete, exec: $*"
exec "$@"