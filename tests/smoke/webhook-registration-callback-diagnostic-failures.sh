#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_DEPLOYED_WEBHOOK_DIAGNOSTIC_FAILURES:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_DEPLOYED_WEBHOOK_DIAGNOSTIC_FAILURES=1 to run deployed webhook failure phases"
    exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

deployment=example-full
manifest="$repo_root/manifests/dev/example-full.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
work_root=$(mktemp -d)
shadow_path=$(mktemp -d)
webpa_container="${deployment}-webpa-1"

cleanup() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null 2>&1 || true
    rm -rf "$work_root" "$shadow_path"
}
state_root=""
trap cleanup EXIT

edge_is_passed() {
    local output=$1
    local edge=$2
    printf '%s\n' "$output" | awk -v edge="$edge" '
        index($0, "\"edgeId\": \"" edge "\"") { found = 1; next }
        found && /"state": "passed"/ { passed = 1; exit }
        found && /"edgeId":/ { exit }
        END { exit passed ? 0 : 1 }
    '
}

assert_first_failure() {
    local name=$1
    local expected=$2
    local output=$3
    local status=$4
    if [[ $status -eq 0 ]]; then
        echo "expected failed diagnostic for $name" >&2
        echo "$output" >&2
        exit 1
    fi
    if [[ "$output" == *"diagnose unexpectedly invoked podman"* ]]; then
        echo "$output" >&2
        exit 1
    fi
    if [[ "$output" != *"\"firstFailure\": \"$expected\""* ]]; then
        echo "expected $name first failure: $expected" >&2
        echo "$output" >&2
        exit 1
    fi
}

start_stack() {
    local name=$1
    local stack_manifest=$2
    state_root="$work_root/$name"
    mkdir -p "$state_root"
    "$vcpe" up --manifest "$stack_manifest" --state-root "$state_root"
    for _ in {1..20}; do
        set +e
        passive_output=$("$vcpe" diagnose \
            --name "$deployment" --from event-sink --to webhook \
            --state-root "$state_root" --json 2>&1)
        set -e
        if edge_is_passed "$passive_output" registration-conformant; then
            return
        fi
        sleep 1
    done
    echo "event-sink registration did not become fresh and conformant for $name" >&2
    echo "$passive_output" >&2
    exit 1
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

run_active_diagnostic() {
    local event=$1
    set +e
    diagnostic_output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
        --name "$deployment" --from event-sink --to webhook \
        --allow-active-callback --event "$event" --device-id mac:001122334455 \
        --state-root "$state_root" --json 2>&1)
    diagnostic_status=$?
    set -e
}

cd "$repo_root"
make build
cat >"$shadow_path/podman" <<'EOF'
#!/bin/sh
echo "diagnose unexpectedly invoked podman" >&2
exit 99
EOF
chmod +x "$shadow_path/podman"

start_stack missing-registration "$manifest"
item=$(argus_item)
item_id=$(argus_item_id "$item")
podman exec "$webpa_container" curl -fsS -u user:pass \
    -X DELETE "http://127.0.0.1:6600/api/v1/store/webhooks/$item_id" >/dev/null
run_active_diagnostic apparmor/diagnostic
assert_first_failure missing-registration registration-present "$diagnostic_output" "$diagnostic_status"
stop_stack

start_stack expired-registration "$manifest"
item=$(argus_item)
expired_item=$(printf '%s' "$item" | sed -E 's/"until":"[^"]+"/"until":"2000-01-01T00:00:00Z"/; s/"ttl":[0-9]+/"ttl":3600/')
put_argus_item "$expired_item"
run_active_diagnostic apparmor/diagnostic
assert_first_failure expired-registration registration-fresh "$diagnostic_output" "$diagnostic_status"
stop_stack

start_stack invalid-signature "$manifest"
item=$(argus_item)
invalid_secret_item=$(printf '%s' "$item" | sed -E 's/"secret":"[^"]*"/"secret":"diagnostic-invalid-secret"/')
put_argus_item "$invalid_secret_item"
run_active_diagnostic apparmor/diagnostic
assert_first_failure invalid-signature callback-acceptance "$diagnostic_output" "$diagnostic_status"
stop_stack

unreachable_manifest="$work_root/unreachable-callback.yaml"
awk '
    /WEBHOOK_SECRET:/ {
        print
        print "          WEBHOOK_URL: \"http://does-not-resolve.invalid/webhook\""
        next
    }
    { print }
' "$manifest" >"$unreachable_manifest"
start_stack unreachable-callback "$unreachable_manifest"
run_active_diagnostic apparmor/diagnostic
assert_first_failure unreachable-callback callback-dns "$diagnostic_output" "$diagnostic_status"
stop_stack

start_stack event-filter-mismatch "$manifest"
run_active_diagnostic not-apparmor/diagnostic
assert_first_failure event-filter-mismatch caduceus-ingestion "$diagnostic_output" "$diagnostic_status"
stop_stack

echo "PASS: deployed webhook diagnostic failure phases reported causal first failures"