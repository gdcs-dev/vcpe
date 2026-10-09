#!/usr/bin/env bash
set -euo pipefail

image=${WEBCONFIG_IMAGE:-localhost/vcpe-webconfig:test}
name="vcpe-webconfig-image-test-$$"

cleanup() {
    podman rm --force "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

podman run --detach --name "$name" "$image" >/dev/null
for _ in $(seq 1 30); do
    if podman exec "$name" /usr/local/bin/vcpe-healthd --check >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
podman exec "$name" /usr/local/bin/vcpe-healthd --check >/dev/null

response=$(mktemp)
trap 'rm -f "$response"; cleanup' EXIT
podman exec "$name" curl --fail --silent --show-error --dump-header - --output "$response" \
    http://127.0.0.1:9000/api/v1/device/02:00:00:00:00:01/config >"$response.headers"
grep --quiet -i '^Content-Type: multipart/mixed' "$response.headers"
grep --quiet 'privatessid' "$response"