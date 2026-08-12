#!/usr/bin/env bash
set -euo pipefail

name="vcpe-health-loopback-port-$$"
port="47999"

cleanup() {
    podman rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

podman run --rm -d --name "$name" \
    -p "127.0.0.1:${port}:80" \
    docker.io/library/nginx:alpine >/dev/null

for _ in {1..20}; do
    if curl --fail --silent --show-error --max-time 2 "http://127.0.0.1:${port}/" >/dev/null; then
        echo "PASS: loopback-published Podman port is reachable from the host"
        exit 0
    fi
    sleep 1
done

echo "FAIL: loopback-published Podman port was not reachable from the host" >&2
exit 1
