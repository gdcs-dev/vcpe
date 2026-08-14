#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
healthy_name="vcpe-health-healthy-$$"
unhealthy_name="vcpe-health-unhealthy-$$"
healthy_port=47998
unhealthy_port=47997
if ! command -v podman >/dev/null; then
    echo "SKIP: podman is not installed"
    exit 0
fi
machine_arch=$(podman machine ssh -- uname -m 2>/dev/null || true)
case "$machine_arch" in
    aarch64|arm64) platform_dir=linux-arm64; platform=linux/arm64 ;;
    x86_64|amd64) platform_dir=linux-amd64; platform=linux/amd64 ;;
    *)
        echo "SKIP: unsupported Podman machine architecture ${machine_arch:-unknown}"
        exit 0
        ;;
esac
binary="$repo_root/services/bng/container/platforms/$platform_dir/vcpe-healthd"

cleanup() {
    podman rm -f "$healthy_name" "$unhealthy_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$binary" ]]; then
    echo "SKIP: staged vcpe-healthd is unavailable; run scripts/stage-runtime-init-binaries first"
    exit 0
fi

run_healthd() {
    local name=$1
    local port=$2
    local command=$3
    podman run -d --name "$name" \
	--platform "$platform" \
        -p "127.0.0.1:${port}:9878" \
        --health-cmd '/usr/local/bin/vcpe-healthd --check' \
        --health-interval 1s \
        --health-retries 2 \
        -v "$binary:/usr/local/bin/vcpe-healthd:ro" \
        docker.io/library/alpine:3.19 \
        /usr/local/bin/vcpe-healthd --command "$command" >/dev/null
}

wait_for_status() {
    local name=$1
    local expected=$2
    local status
    for _ in {1..20}; do
        status=$(podman inspect --format '{{.State.Health.Status}}' "$name" 2>/dev/null || true)
        [[ "$status" == "$expected" ]] && return 0
        sleep 1
    done
    echo "expected OCI health status $expected for $name, got $status" >&2
    return 1
}

wait_for_endpoint() {
    local port=$1
    local expected=$2
    local response
    for _ in {1..20}; do
        response=$(curl --fail --silent --show-error "http://127.0.0.1:${port}/health" 2>/dev/null || true)
        [[ "$response" == *"\"status\":\"${expected}\""* ]] && return 0
        sleep 1
    done
    echo "health endpoint on port $port did not report $expected" >&2
    return 1
}

run_healthd "$healthy_name" "$healthy_port" true
run_healthd "$unhealthy_name" "$unhealthy_port" false

wait_for_endpoint "$healthy_port" healthy
wait_for_endpoint "$unhealthy_port" unhealthy
wait_for_status "$healthy_name" healthy
wait_for_status "$unhealthy_name" unhealthy

echo "PASS: loopback endpoint and OCI healthcheck share health daemon results"