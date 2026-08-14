#!/bin/sh

add_dummy_interface() {
    ip link add "$1" type dummy
    ip link set "$1" up
}

psm_set() {
    local key="$1"
    local value="$2"
    local max_attempts=30
    local attempt=0
    echo "Setting PSM key $key to $value"
    while [ $attempt -lt $max_attempts ]; do
        psmcli set "$key" "$value"
        local readback
        readback=$(psmcli get "$key")
        if [ "$readback" = "$value" ]; then
            return 0
        fi
        attempt=$((attempt + 1))
        echo "Retry $attempt/$max_attempts: psmcli set $key=$value (got '$readback')" >&2
        sleep 1
    done
    echo "ERROR: failed to set psmcli $key=$value after $max_attempts attempts" >&2
    return 1
}

sysevent_set() {
    local key="$1"
    local value="$2"
    local max_attempts=10
    local attempt=0
    echo "Setting sysevent key $key to $value"
    while [ $attempt -lt $max_attempts ]; do
        sysevent set "$key" "$value"
        local readback
        readback=$(sysevent get "$key")
        if [ "$readback" = "$value" ]; then
            return 0
        fi
        attempt=$((attempt + 1))
        echo "Retry $attempt/$max_attempts: sysevent set $key=$value (got '$readback')" >&2
        sleep 1
    done
    echo "ERROR: failed to set sysevent $key=$value after $max_attempts attempts" >&2
    return 1
}

syscfg_set() {
    local key="$1"
    local value="$2"
    local max_attempts=10
    local attempt=0
    echo "Setting syscfg key $key to $value"

    while [ $attempt -lt $max_attempts ]; do
        syscfg set "$key" "$value"
        syscfg commit
        local readback
        readback=$(syscfg get "$key")
        if [ "$readback" = "$value" ]; then
            return 0
        fi
        attempt=$((attempt + 1))
        echo "Retry $attempt/$max_attempts: syscfg set $key=$value (got '$readback')" >&2
        sleep 1
    done
    echo "ERROR: failed to set syscfg $key=$value after $max_attempts attempts" >&2
    return 1
}

wait_for_erouter0_and_start_dhcp() {
    echo "Waiting for erouter0 to be available and starting DHCP client"
    while ! ip link show erouter0 > /dev/null 2>&1; do
        sleep 1
    done
    echo "erouter0 is available, starting DHCP client"
    udhcpc -b -i erouter0 -p /tmp/udhcpc.erouter0.pid -s /etc/udhcpc.script -x "hostname:$(hostname)"
}

check_for_cm0_and_start_dhcp() {
    if ip link show cm0 > /dev/null 2>&1; then
        echo "cm0 is available, starting DHCP client"
        udhcpc -q -b -i cm0 -p /tmp/udhcpc.cm0.pid -s /etc/udhcpc.script -x "hostname:$(hostname)"
    fi
}

restore_wan_state() {
    sysevent_set current_wan_ifname erouter0
    sysevent_set wan_ifname erouter0
    sysevent_set ethwan-initialized 1
    sysevent_set current_ipv4_link_state up
    sysevent_set desired_ipv4_link_state up
    sysevent_set desired_ipv4_wan_state up
    sysevent_set mesh_wan_linkstatus up
    sysevent_set current_ipv4_wan_state up
    sysevent_set current_wan_state up
    sysevent_set phylink_wan_state up
    sysevent_set wan-status started
    sysevent_set wan_service-status started
}

monitor_onewifi() {
    echo "Waiting for onewifi service to become active"
    while ! systemctl is-active --quiet onewifi; do
        echo "onewifi is not yet active, waiting..."
        sleep 5
        rm -f /tmp/.brcm_wifi_ready
        touch /tmp/.brcm_wifi_ready
    done
    echo "onewifi is active"
}

monitor_default_route() {
    echo "Starting default route watchdog for erouter0"
    while true; do
        sleep 10
        if ! ip route show default dev erouter0 2>/dev/null | grep -q default; then
            gw=$(sysevent get default_router 2>/dev/null)
            echo "Default route via erouter0 missing (gw=${gw:-unknown}), restoring"
            restore_wan_state
            if [ -n "$gw" ]; then
                echo "ip route add default via $gw dev erouter0"
                ip route add default via "$gw" dev erouter0 2>/dev/null \
                && echo "Default route restored via $gw" \
                || echo "Failed to add default route via $gw"
            fi
            pid_file=/tmp/udhcpc.erouter0.pid
            [ -f "$pid_file" ] && kill -USR1 "$(cat "$pid_file")" 2>/dev/null || true
        fi
    done
}

add_dummy_interface wl0
add_dummy_interface wl1
add_dummy_interface wl2

check_for_cm0_and_start_dhcp
wait_for_erouter0_and_start_dhcp
sysevent_set current_wan_ifname erouter0
restore_wan_state
sysevent_set ntpd-status started
sysevent_set ntp_time_sync 1
sysevent_set wan_start_time "$(/usr/bin/cut -d. -f1 /proc/uptime)"
#wan_dhcp_dns=$(awk '/^nameserver/{print $2; exit}' /etc/resolv.conf)
#sysevent_set wan_dhcp_dns "$wan_dhcp_dns"
#syscfg_set dhcp_nameserver_enabled 1
#syscfg_set dhcp_nameserver_1 "$wan_dhcp_dns"

touch /var/wan_started
touch /tmp/wanmanager_initialized

monitor_default_route &
monitor_onewifi &

echo "Exiting interface_config.sh"
exit 0
