#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_DEPLOYED_WEBHOOK_DIAGNOSTIC:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_DEPLOYED_WEBHOOK_DIAGNOSTIC=1 to provision the webhook diagnostic stack"
    exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

deployment=example-full
manifest="$repo_root/manifests/dev/example-full.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
state_root=$(mktemp -d)
shadow_path=$(mktemp -d)

cleanup() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null 2>&1 || true
    rm -rf "$state_root" "$shadow_path"
}
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

cd "$repo_root"
make build
"$vcpe" up --manifest "$manifest" --state-root "$state_root"

for _ in {1..20}; do
    set +e
    passive_output=$("$vcpe" diagnose \
        --name "$deployment" --from event-sink --to webhook \
        --state-root "$state_root" --json 2>&1)
    set -e
    if edge_is_passed "$passive_output" registration-conformant; then
        break
    fi
    sleep 1
done
if ! edge_is_passed "$passive_output" registration-conformant; then
    echo "event-sink registration did not become fresh and conformant" >&2
    echo "$passive_output" >&2
    exit 1
fi

cat >"$shadow_path/podman" <<'EOF'
#!/bin/sh
echo "diagnose unexpectedly invoked podman" >&2
exit 99
EOF
chmod +x "$shadow_path/podman"

diagnostic_output=$(PATH="$shadow_path:$PATH" "$vcpe" diagnose \
    --name "$deployment" --from event-sink --to webhook \
    --allow-active-callback --event apparmor/diagnostic --device-id mac:001122334455 \
    --state-root "$state_root" --json 2>&1)
diagnostic_status=$?

if [[ $diagnostic_status -ne 0 ]]; then
    echo "expected healthy active webhook diagnostic exit status" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi
if [[ "$diagnostic_output" == *"unexpectedly invoked podman"* ]]; then
    echo "$diagnostic_output" >&2
    exit 1
fi
for edge in \
    registration-present \
    registration-fresh \
    registration-conformant \
    callback-dns \
    callback-transport \
    callback-acceptance \
    caduceus-ingestion \
    caduceus-receipt; do
    if ! edge_is_passed "$diagnostic_output" "$edge"; then
        echo "expected passed webhook diagnostic edge: $edge" >&2
        echo "$diagnostic_output" >&2
        exit 1
    fi
done
if [[ "$diagnostic_output" != *'"key": "correlation-state"'* || "$diagnostic_output" != *'"value": "recorded"'* ]]; then
    echo "expected correlated subscriber receipts" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi

echo "PASS: deployed webhook diagnostic verified Argus registration, direct callback, and Caduceus receipt delivery"