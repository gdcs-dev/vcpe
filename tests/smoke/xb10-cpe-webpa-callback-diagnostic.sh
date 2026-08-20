#!/usr/bin/env bash
set -euo pipefail

CDPATH=''
repo_root=$(cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_DEPLOYED_XB10_CPE_CALLBACK_DIAGNOSTIC:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_DEPLOYED_XB10_CPE_CALLBACK_DIAGNOSTIC=1 to run XB10 callback diagnostic smoke coverage"
    exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

if command -v lsof >/dev/null 2>&1; then
    for port in 47000 47001 47002 47003; do
        if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
            echo "SKIP: health port $port is already in use; stop the conflicting deployment before running this smoke"
            exit 0
        fi
    done
fi

deployment=xb10-callback-diagnostic-smoke
manifest_source="$repo_root/manifests/dev/xb10.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
work_root=$(mktemp -d)
shadow_path=$(mktemp -d)
state_root="$work_root/state"
xb10_container="${deployment}-xb10-1"

cleanup() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null 2>&1 || true
    rm -rf "$work_root" "$shadow_path"
}
trap cleanup EXIT

edge_has_state() {
    local output=$1
    local edge=$2
    local state=$3
    printf '%s\n' "$output" | awk -v edge="$edge" -v state="$state" '
        index($0, "\"edgeId\": \"" edge "\"") { found = 1; next }
        found && $0 ~ "\"state\": \"" state "\"" { matched = 1; exit }
        found && /"edgeId":/ { exit }
        END { exit matched ? 0 : 1 }
    '
}

wait_for_registration() {
    local output=""
    for _ in {1..30}; do
        set +e
        output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
            --name "$deployment" --from event-sink --to webhook \
            --state-root "$state_root" --json 2>&1)
        set -e
        if edge_has_state "$output" registration-conformant passed; then
            return
        fi
        sleep 1
    done
    echo "event-sink registration did not become fresh and conformant" >&2
    echo "$output" >&2
    exit 1
}

wait_for_xb10_callback_capability() {
    local capabilities=""
    for _ in {1..30}; do
        capabilities=$(podman exec "$xb10_container" sh -c 'wget -qO- http://127.0.0.1:9878/diagnostics' 2>/dev/null || true)
        if [[ "$capabilities" == *'"cpe-webpa"'* && "$capabilities" == *'"cpe-webpa-callback"'* && "$capabilities" == *'"parodus-clients"'* ]]; then
            return
        fi
        sleep 1
    done
    echo "XB10 did not advertise callback diagnostic capability" >&2
    echo "$capabilities" >&2
    exit 1
}

wait_for_xb10_cpe() {
    local output=""
    for _ in {1..30}; do
        set +e
        output=$("$vcpe" diagnose \
            --name "$deployment" --from xb10 --to webpa \
            --client-service apparmor-simulator \
            --state-root "$state_root" --json 2>&1)
        status=$?
        set -e
        if [[ $status -eq 0 ]]; then
            return
        fi
        sleep 1
    done
    echo "XB10 passive CPE diagnostics did not become healthy" >&2
    echo "$output" >&2
    exit 1
}

cd "$repo_root"
make build
manifest="$work_root/$deployment.yaml"
sed \
    -e "s/^  name: xb10$/  name: $deployment/" \
    -e 's/10\.10\.20/10.94.20/g' \
    -e 's/10\.7\.200/10.94.200/g' \
    -e 's/10\.7\.201/10.94.201/g' \
    "$manifest_source" >"$manifest"
"$vcpe" up --manifest "$manifest" --state-root "$state_root"

wait_for_registration
wait_for_xb10_callback_capability
wait_for_xb10_cpe

device_id=$(podman exec "$xb10_container" sh -c 'tr -d ":\n" </sys/class/net/erouter0/address')
if [[ ! "$device_id" =~ ^[[:xdigit:]]{12}$ ]]; then
    echo "XB10 has invalid erouter0 identity: $device_id" >&2
    exit 1
fi

cat >"$shadow_path/podman" <<'EOF'
#!/bin/sh
echo "diagnose unexpectedly invoked podman" >&2
exit 99
EOF
chmod +x "$shadow_path/podman"

set +e
diagnostic_output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
    --name "$deployment" --from xb10 --to callback \
    --client-service apparmor-simulator --subscriber event-sink \
    --allow-active-event --event devices/diagnostic --device-id "mac:$device_id" \
    --state-root "$state_root" --json 2>&1)
diagnostic_status=$?
set -e

if [[ $diagnostic_status -ne 0 ]]; then
    echo "expected healthy XB10 callback diagnostic exit status" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
if [[ "$diagnostic_output" == *"diagnose unexpectedly invoked podman"* ]]; then
    echo "$diagnostic_output" >&2
    exit 1
fi
for edge in \
    application-parodus talaria-dns talaria-transport talaria-authentication \
    device-registration subscriber-intent argus-reachability argus-authentication \
    registration-present registration-fresh registration-conformant \
    active-event-acceptance routing-observation callback-receipt; do
    if ! edge_has_state "$diagnostic_output" "$edge" passed; then
        echo "expected $edge to pass" >&2
        echo "$diagnostic_output" >&2
        exit 1
    fi
done
if [[ $(grep -c '"key": "correlation-state"' <<<"$diagnostic_output") -ne 1 || "$diagnostic_output" != *'"value": "accepted"'* ]]; then
    echo "expected exactly one accepted marked event" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi

echo "PASS: deployed XB10 callback diagnostic discovered capability and delivered one correlated event"