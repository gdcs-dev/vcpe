#!/bin/bash
set -euo pipefail

ROUTERD_SOCKET_DIR=${ROUTERD_SOCKET_DIR:-/run/routerd}
ROUTERD_STATE_DIR=${ROUTERD_STATE_DIR:-/var/lib/routerd}
ROUTERD_PROFILE=${ROUTERD_PROFILE:-all}
ROUTERD_CONFIG=${ROUTERD_CONFIG:-/etc/routerd/config.json}
ROUTERD_RENDERED_CONFIG=${ROUTERD_RENDERED_CONFIG:-/runtime-config/etc/routerd/config.json}
ROUTERD_BIN_DIR=${ROUTERD_BIN_DIR:-/usr/bin}
ROUTERD_BIN=${ROUTERD_BIN:-$ROUTERD_BIN_DIR/routerd}
ROUTERCTL_BIN=${ROUTERCTL_BIN:-$ROUTERD_BIN_DIR/routerctl}

# Rename every interface identified by the manifest's IFACE_<ROLE>_MAC/_DEVICE
# env vars — no hardcoded target table — per the manifest-driven-interface-names
# contract shared with BNG/Gateway/XB10.
rename_interfaces_by_mac() {
    declare -A current_by_mac=()
    local path name mac

    for path in /sys/class/net/*; do
        name=$(basename "$path")
        [[ "$name" == lo ]] && continue
        mac=$(cat "$path/address")
        current_by_mac["${mac,,}"]=$name
    done

    declare -A temp_by_target=()
    local var role_key dev_var target current temp_name t
    while IFS='=' read -r var mac; do
        [[ "$var" == IFACE_*_MAC ]] || continue
        [[ -n "$mac" ]] || continue
        role_key=${var%_MAC}
        role_key=${role_key#IFACE_}
        dev_var="IFACE_${role_key}_DEVICE"
        target="${!dev_var:-}"
        [[ -n "$target" ]] || continue
        current="${current_by_mac[${mac,,}]:-}"
        [[ -n "$current" ]] || continue
        [[ "$current" == "$target" ]] && continue
        temp_name="tmp-${target}"
        ip link set "$current" down
        ip link set "$current" name "$temp_name"
        temp_by_target[$target]=$temp_name
    done < <(env)

    # A target name may still be held by an interface outside our rename set
    # (e.g. the health-publication sidecar's raw network attachment, which
    # carries no IFACE_*_MAC identity of its own). Displace any such occupant
    # first so the final rename below never collides with it. The scratch
    # name must stay within IFNAMSIZ (15 bytes).
    local displaced=0
    for t in "${!temp_by_target[@]}"; do
        [[ -e "/sys/class/net/$t" ]] || continue
        displaced=$((displaced + 1))
        ip link set "$t" down
        ip link set "$t" name "disp$displaced"
    done

    for t in "${!temp_by_target[@]}"; do
        ip link set "${temp_by_target[$t]}" name "$t"
    done
}

# prepare_config copies the control plane's pre-rendered ConfigDocument
# (rendered from the manifest at plan/apply time) into place. Nothing in this
# container assembles or transforms config JSON at runtime.
prepare_config() {
    [[ -f "$ROUTERD_RENDERED_CONFIG" ]] \
        || { echo "missing rendered routerd config at $ROUTERD_RENDERED_CONFIG" >&2; exit 1; }
    mkdir -p "$(dirname "$ROUTERD_CONFIG")"
    cp "$ROUTERD_RENDERED_CONFIG" "$ROUTERD_CONFIG"
}

wait_for_socket() {
    local i
    for i in $(seq 1 40); do
        "$ROUTERCTL_BIN" --socket-dir "$ROUTERD_SOCKET_DIR" describe >/dev/null 2>&1 && return 0
        sleep 0.25
    done
    return 1
}

main() {
    if [[ -z "${VCPE_HEALTHD_SUPERVISED:-}" ]]; then
        export VCPE_HEALTHD_SUPERVISED=1
        exec /usr/local/bin/vcpe-healthd \
            --command /usr/local/bin/routerd-health-probe \
            --run /usr/local/bin/routerd-legacy-entrypoint.sh
    fi

    rename_interfaces_by_mac
    prepare_config

    mkdir -p "$ROUTERD_STATE_DIR" "$ROUTERD_SOCKET_DIR"
    chmod 0700 "$ROUTERD_SOCKET_DIR"

    [[ -x "$ROUTERD_BIN" ]] \
        || { echo "routerd binary not found at $ROUTERD_BIN" >&2; \
             echo "run: scripts/routerd compile" >&2; exit 1; }
    [[ -x "$ROUTERCTL_BIN" ]] \
        || { echo "routerctl binary not found at $ROUTERCTL_BIN" >&2; exit 1; }

    "$ROUTERD_BIN" --profile "$ROUTERD_PROFILE" --socket-dir "$ROUTERD_SOCKET_DIR" --state-dir "$ROUTERD_STATE_DIR" &
    local routerd_pid=$!

    if ! wait_for_socket; then
        echo "routerd did not become ready on socket-dir $ROUTERD_SOCKET_DIR" >&2
        kill -TERM "$routerd_pid" 2>/dev/null || true
        wait "$routerd_pid" 2>/dev/null || true
        exit 1
    fi

    if ! "$ROUTERCTL_BIN" --socket-dir "$ROUTERD_SOCKET_DIR" apply --file "$ROUTERD_CONFIG"; then
        echo "routerctl apply failed against $ROUTERD_CONFIG" >&2
        kill -TERM "$routerd_pid" 2>/dev/null || true
        wait "$routerd_pid" 2>/dev/null || true
        exit 1
    fi

    trap 'kill -TERM '"$routerd_pid"' 2>/dev/null || true; wait '"$routerd_pid"' 2>/dev/null || true' TERM INT
    wait "$routerd_pid"
}

main "$@"
