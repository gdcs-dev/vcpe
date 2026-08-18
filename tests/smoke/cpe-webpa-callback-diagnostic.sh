#!/usr/bin/env bash
set -euo pipefail

CDPATH=''
repo_root=$(cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_DEPLOYED_CPE_CALLBACK_DIAGNOSTIC:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_DEPLOYED_CPE_CALLBACK_DIAGNOSTIC=1 to run Gateway callback diagnostic smoke coverage"
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

deployment=callback-diagnostic-smoke
manifest_source="$repo_root/manifests/dev/example.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
work_root=$(mktemp -d)
shadow_path=$(mktemp -d)
state_root=""
webpa_container="${deployment}-webpa-1"

cleanup() {
    if [[ -n "$state_root" ]]; then
        "$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null 2>&1 || true
    fi
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

assert_result() {
    local name=$1
    local status=$2
    local output=$3
    local edge=$4
    local state=$5
    local reason=${6:-}

    if [[ $status -eq 0 && "$state" != "passed" ]]; then
        echo "expected non-zero diagnostic status for $name" >&2
        echo "$output" >&2
        exit 1
    fi
    if [[ "$output" == *"diagnose unexpectedly invoked podman"* ]]; then
        echo "$output" >&2
        exit 1
    fi
    if ! edge_has_state "$output" "$edge" "$state"; then
        echo "expected $name edge $edge to be $state" >&2
        echo "$output" >&2
        exit 1
    fi
    if [[ -n "$reason" && "$output" != *"\"reasonId\": \"$reason\""* ]]; then
        echo "expected $name reason $reason" >&2
        echo "$output" >&2
        exit 1
    fi
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

start_stack() {
    local name=$1
    local stack_manifest=$2
    state_root="$work_root/$name"
    mkdir -p "$state_root"
    "$vcpe" up --manifest "$stack_manifest" --state-root "$state_root"
    wait_for_registration
}

stop_stack() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null
    state_root=""
}

argus_item() {
    local items item
    items=$(podman exec "$webpa_container" curl -fsS -u user:pass http://127.0.0.1:6600/api/v1/store/webhooks)
    item=${items#\[}
    item=${item%\]}
    if [[ -z "$item" || "$item" == "$items" || "$item" == *,* ]]; then
        echo "expected exactly one Argus webhook item" >&2
        exit 1
    fi
    printf '%s' "$item"
}

argus_item_id() {
    local item=$1
    local id
    id=$(printf '%s' "$item" | sed -nE 's/.*"id":"([^"]+)".*/\1/p')
    if [[ -z "$id" ]]; then
        echo "could not determine Argus webhook item ID" >&2
        exit 1
    fi
    printf '%s' "$id"
}

put_argus_item() {
    local item=$1
    local id
    id=$(argus_item_id "$item")
    printf '%s' "$item" | podman exec -i "$webpa_container" \
        curl -fsS -u user:pass -H 'Content-Type: application/json' --data-binary @- \
        -X PUT "http://127.0.0.1:6600/api/v1/store/webhooks/$id" >/dev/null
}

run_callback() {
    local event=$1
    set +e
    diagnostic_output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
        --name "$deployment" --from gateway --to callback \
        --client-service apparmor-simulator --subscriber event-sink \
        --allow-active-event --event "$event" --device-id mac:001122334455 \
        --state-root "$state_root" --json 2>&1)
    diagnostic_status=$?
    set -e
}

cd "$repo_root"
make build
manifest="$work_root/$deployment.yaml"
sed \
    -e "s/^  name: example$/  name: $deployment/" \
    -e 's/10\.10\.10/10.93.10/g' \
    -e 's/10\.7\.200/10.93.200/g' \
    -e 's/10\.7\.201/10.93.201/g' \
    "$manifest_source" >"$manifest"
cat >"$shadow_path/podman" <<'EOF'
#!/bin/sh
echo "diagnose unexpectedly invoked podman" >&2
exit 99
EOF
chmod +x "$shadow_path/podman"

start_stack healthy "$manifest"
run_callback apparmor/diagnostic
if [[ $diagnostic_status -ne 0 ]]; then
    echo "expected healthy Gateway callback diagnostic exit status" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
for edge in \
    application-parodus talaria-dns talaria-transport talaria-authentication \
    device-registration subscriber-intent argus-reachability argus-authentication \
    registration-present registration-fresh registration-conformant \
    active-event-acceptance routing-observation callback-receipt; do
    assert_result healthy "$diagnostic_status" "$diagnostic_output" "$edge" passed
done
stop_stack

start_stack cpe-connectivity "$manifest"
podman stop "$webpa_container" >/dev/null
run_callback apparmor/diagnostic
if [[ $diagnostic_status -eq 0 ]]; then
    echo "expected CPE connectivity diagnostic failure" >&2
    exit 1
fi
if ! edge_has_state "$diagnostic_output" talaria-dns failed && ! edge_has_state "$diagnostic_output" talaria-transport failed; then
    echo "expected Talaria DNS or transport failure after stopping WebPA" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
stop_stack

start_stack registration-matcher "$manifest"
run_callback not-apparmor/diagnostic
assert_result registration-matcher "$diagnostic_status" "$diagnostic_output" registration-conformant failed registration-mismatch
stop_stack

start_stack routing-observation "$manifest"
podman exec "$webpa_container" sh -c 'pkill -TERM caduceus; while pgrep caduceus >/dev/null; do sleep 0.1; done'
run_callback apparmor/diagnostic
assert_result routing-observation "$diagnostic_status" "$diagnostic_output" routing-observation unknown routing-observation-unavailable
stop_stack

start_stack invalid-callback-signature "$manifest"
item=$(argus_item)
invalid_secret_item=$(printf '%s' "$item" | sed -E 's/"secret":"[^"]*"/"secret":"diagnostic-invalid-secret"/')
put_argus_item "$invalid_secret_item"
run_callback apparmor/diagnostic
assert_result invalid-callback-signature "$diagnostic_status" "$diagnostic_output" callback-receipt unknown caduceus-receipt-missing
if ! edge_has_state "$diagnostic_output" routing-observation passed; then
    echo "expected invalid signature to retain routing evidence" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
stop_stack

missing_receipt_manifest="$work_root/missing-receipt.yaml"
awk '
    /WEBHOOK_SECRET:/ {
        print
        print "          WEBHOOK_URL: \"http://does-not-resolve.invalid/webhook\""
        next
    }
    { print }
' "$manifest" >"$missing_receipt_manifest"
start_stack missing-receipt "$missing_receipt_manifest"
run_callback apparmor/diagnostic
assert_result missing-receipt "$diagnostic_status" "$diagnostic_output" callback-receipt unknown caduceus-receipt-missing
if ! edge_has_state "$diagnostic_output" routing-observation passed; then
    echo "expected absent receipt to retain routing evidence" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
stop_stack

echo "PASS: deployed Gateway callback diagnostic covered delivery and causal failure boundaries"