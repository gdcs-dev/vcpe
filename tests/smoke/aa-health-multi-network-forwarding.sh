#!/usr/bin/env bash
# Design-gate spike for openspec/changes/direct-health-publication: proves that
# Podman forwards a published host-loopback port to a workload attached to one
# Podman-managed network (aa-health) plus multiple ipamDriver:none topology
# networks, without any container inspection. See design.md "Decisions" and
# "Open Questions".
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
name="vcpe-aa-health-spike-$$"
port=47996
net_unmanaged1="vcpe-aa-health-spike-unmanaged1-$$"
net_unmanaged2="vcpe-aa-health-spike-unmanaged2-$$"
net_managed="vcpe-aa-health-spike-managed-$$"
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
    podman rm -f "$name" >/dev/null 2>&1 || true
    podman network rm -f "$net_unmanaged1" "$net_unmanaged2" "$net_managed" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -x "$binary" ]]; then
    echo "SKIP: staged vcpe-healthd is unavailable; run scripts/stage-runtime-init-binaries first"
    exit 0
fi

echo "podman client: $(podman version --format '{{.Client.Version}}')"
echo "podman machine arch: $machine_arch"

podman network create --ipam-driver none "$net_unmanaged1" >/dev/null
podman network create --ipam-driver none "$net_unmanaged2" >/dev/null
podman network create "$net_managed" >/dev/null

# Interface order mirrors compose rendering: topology networks first, then the
# deterministically-last-alphabetically-sorted "aa-health" managed attachment.
podman run -d --name "$name" \
    --platform "$platform" \
    --network "$net_unmanaged1" \
    --network "$net_unmanaged2" \
    --network "$net_managed" \
    -p "127.0.0.1:${port}:9878" \
    -v "$binary:/usr/local/bin/vcpe-healthd:ro" \
    docker.io/library/alpine:3.19 \
    /usr/local/bin/vcpe-healthd --command true >/dev/null

response=""
for _ in {1..20}; do
    response=$(curl --fail --silent --show-error "http://127.0.0.1:${port}/health" 2>/dev/null || true)
    [[ "$response" == *'"status":"healthy"'* ]] && break
    sleep 1
done

if [[ "$response" != *'"status":"healthy"'* ]]; then
    echo "FAIL: direct forwarding through the managed aa-health attachment did not reach the workload (last response: $response)" >&2
    exit 1
fi

echo "PASS: published host-loopback port forwarded through the managed network despite multiple ipamDriver:none topology attachments"
