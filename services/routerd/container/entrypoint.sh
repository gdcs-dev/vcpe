#!/bin/bash
set -euo pipefail

ROUTERD_SOCKET=${ROUTERD_SOCKET:-/run/routerd/routerd.sock}
ROUTERD_STATE_DIR=${ROUTERD_STATE_DIR:-/var/lib/routerd}
ROUTERD_CONFIG=${ROUTERD_CONFIG:-/etc/routerd/config.json}
ROUTERD_BIN_DIR=${ROUTERD_BIN_DIR:-/workspace/target/release}
ROUTERD_BIN=${ROUTERD_BIN:-$ROUTERD_BIN_DIR/routerd}
ROUTERCTL_BIN=${ROUTERCTL_BIN:-$ROUTERD_BIN_DIR/routerctl}

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

render_config() {
    mkdir -p "$(dirname "$ROUTERD_CONFIG")"
    /usr/local/bin/render-config.sh >"$ROUTERD_CONFIG"
}

wait_for_socket() {
    local i
    for i in $(seq 1 40); do
        [[ -S "$ROUTERD_SOCKET" ]] && return 0
        sleep 0.25
    done
    return 1
}

main() {
    rename_interfaces_by_mac
    render_config

    mkdir -p "$ROUTERD_STATE_DIR" "$(dirname "$ROUTERD_SOCKET")"
    chmod 0700 "$(dirname "$ROUTERD_SOCKET")"

    [[ -x "$ROUTERD_BIN" ]] \
        || { echo "routerd binary not found at $ROUTERD_BIN" >&2; \
             echo "run: scripts/routerd compile" >&2; exit 1; }
    [[ -x "$ROUTERCTL_BIN" ]] \
        || { echo "routerctl binary not found at $ROUTERCTL_BIN" >&2; exit 1; }

    "$ROUTERD_BIN" --socket "$ROUTERD_SOCKET" --state-dir "$ROUTERD_STATE_DIR" &
    local routerd_pid=$!

    if ! wait_for_socket; then
        echo "routerd socket did not appear at $ROUTERD_SOCKET" >&2
        kill -TERM "$routerd_pid" 2>/dev/null || true
        wait "$routerd_pid" 2>/dev/null || true
        exit 1
    fi

    "$ROUTERCTL_BIN" --socket "$ROUTERD_SOCKET" apply "$ROUTERD_CONFIG"

    trap 'kill -TERM '"$routerd_pid"' 2>/dev/null || true; wait '"$routerd_pid"' 2>/dev/null || true' TERM INT
    wait "$routerd_pid"
}

main "$@"
