#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_TELEMETRY_GATEWAY_SMOKE:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_TELEMETRY_GATEWAY_SMOKE=1 to provision the embedded telemetry gateway"
    exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

podman_arch=$(podman info --format '{{.Host.Arch}}')
case "$podman_arch" in
    aarch64) podman_arch=arm64 ;;
    x86_64) podman_arch=amd64 ;;
esac

deployment=example-full
project="${deployment}-telemetry-gateway"
manifest="$repo_root/manifests/dev/example-full.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
state_root=$(mktemp -d)

cleanup() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" --force >/dev/null 2>&1 || true
    rm -rf "$state_root"
}
trap cleanup EXIT

volume_snapshot() {
    podman volume ls --format '{{.Name}}' |
        grep -E "^${project}[_-](postgres-data|prometheus-data|grafana-data)$" |
        sort
}

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
if [[ ! -f services/telemetry-gateway/go.mod ]]; then
    echo "telemetry-gateway submodule is not initialized; run git submodule update --init --recursive services/telemetry-gateway" >&2
    exit 1
fi

make build
"$vcpe" build --manifest "$manifest" --state-root "$state_root" --platform "linux/$podman_arch"
"$vcpe" up --manifest "$manifest" --state-root "$state_root"

health_output=""
for _ in {1..60}; do
    health_output=$("$vcpe" status --name "$deployment" --state-root "$state_root" 2>&1 || true)
    if [[ "$health_output" == *"health telemetry-gateway/0: healthy"* ]]; then
        break
    fi
    sleep 2
done
if [[ "$health_output" != *"health telemetry-gateway/0: healthy"* ]]; then
    echo "telemetry-gateway did not report common healthy status" >&2
    echo "$health_output" >&2
    exit 1
fi

diagnostic_output=""
for _ in {1..30}; do
    diagnostic_output=$("$vcpe" diagnose --name "$deployment" \
        --from telemetry-gateway --to webhook --state-root "$state_root" --json 2>&1 || true)
    if edge_is_passed "$diagnostic_output" registration-conformant; then
        break
    fi
    sleep 2
done
if ! edge_is_passed "$diagnostic_output" registration-conformant; then
    echo "telemetry-gateway passive webhook diagnosis did not become conformant" >&2
    echo "$diagnostic_output" >&2
    exit 1
fi

before=$(volume_snapshot)
if [[ $(printf '%s\n' "$before" | sed '/^$/d' | wc -l | tr -d ' ') != "3" ]]; then
    echo "expected three telemetry-gateway named volumes, got:" >&2
    echo "$before" >&2
    exit 1
fi

"$vcpe" up --manifest "$manifest" --state-root "$state_root"
after=$(volume_snapshot)
if [[ "$after" != "$before" ]]; then
    echo "telemetry-gateway volume identities changed across reapply" >&2
    printf 'before:\n%s\nafter:\n%s\n' "$before" "$after" >&2
    exit 1
fi

"$vcpe" down --name "$deployment" --state-root "$state_root" --force
after_down=$(volume_snapshot)
if [[ "$after_down" != "$before" ]]; then
    echo "telemetry-gateway volumes were not preserved by vcpe down" >&2
    printf 'before:\n%s\nafter down:\n%s\n' "$before" "$after_down" >&2
    exit 1
fi

"$vcpe" up --manifest "$manifest" --state-root "$state_root"
after_restart=$(volume_snapshot)
if [[ "$after_restart" != "$before" ]]; then
    echo "telemetry-gateway volume identities changed after down and later apply" >&2
    printf 'before:\n%s\nafter restart:\n%s\n' "$before" "$after_restart" >&2
    exit 1
fi

if [[ -n $(git -C services/telemetry-gateway status --porcelain) ]]; then
    echo "telemetry-gateway submodule was dirtied by lifecycle artifacts" >&2
    git -C services/telemetry-gateway status --short >&2
    exit 1
fi

echo "PASS: telemetry gateway health, passive registration, reapply/down volume continuity, and clean submodule verified"