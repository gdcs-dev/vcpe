#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_WEBCONFIG_SMOKE:-}" != "1" ]]; then
    echo "SKIP: set VCPE_RUN_WEBCONFIG_SMOKE=1 to run the deployed WebConfig Gateway flow"
    exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
    echo "SKIP: podman is not installed"
    exit 0
fi

deployment="webconfig-smoke-$$"
manifest_source="$repo_root/manifests/dev/example.yaml"
manifest=$(mktemp)
vcpe="$repo_root/controlplane/bin/vcpe"
state_root=$(mktemp -d)

export deployment manifest manifest_source
ruby -ryaml -e 'document = YAML.load_file(ENV.fetch("manifest_source")); document.fetch("metadata")["name"] = ENV.fetch("deployment"); File.write(ENV.fetch("manifest"), YAML.dump(document))'

cleanup() {
    "$vcpe" down --name "$deployment" --state-root "$state_root" --force >/dev/null 2>&1 || true
    rm -f "$manifest"
    rm -rf "$state_root"
}
trap cleanup EXIT

podman_arch=$(podman info --format '{{.Host.Arch}}')
case "$podman_arch" in
    aarch64) podman_arch=arm64 ;;
    x86_64) podman_arch=amd64 ;;
esac

cd "$repo_root"
make build
scripts/stage-runtime-init-binaries bng gateway webpa webconfig
for service in bng gateway webpa webconfig; do
    podman build --platform "linux/$podman_arch" -t "ghcr.io/gdcs-dev/$service:dev" \
        -f "services/$service/Containerfile" "services/$service" >/dev/null
done

"$vcpe" up --manifest "$manifest" --state-root "$state_root"

health_output=""
for _ in {1..60}; do
    health_output=$("$vcpe" status --name "$deployment" --state-root "$state_root" 2>&1 || true)
    [[ "$health_output" == *"health webconfig/0: healthy"* ]] && break
    sleep 2
done
if [[ "$health_output" != *"health webconfig/0: healthy"* ]]; then
    echo "WebConfig did not become healthy" >&2
    echo "$health_output" >&2
    exit 1
fi

url=""
for _ in {1..60}; do
    url=$(podman exec "$deployment-gateway-1" rbuscli -g Device.X_RDK_WebConfig.URL 2>/dev/null || true)
    [[ "$url" == *'http://webconfig:9000/api/v1/device/{mac}/config'* ]] && break
    sleep 2
done
if [[ "$url" != *'http://webconfig:9000/api/v1/device/{mac}/config'* ]]; then
    echo "Gateway WebConfig URL is not configured: $url" >&2
    exit 1
fi

podman exec "$deployment-gateway-1" getent hosts webconfig >/dev/null
podman exec "$deployment-gateway-1" getent hosts webconfig.dns.podman >/dev/null

response=$(mktemp)
headers=$(mktemp)
trap 'rm -f "$response" "$headers"; cleanup' EXIT
podman exec "$deployment-gateway-1" curl --fail --silent --show-error \
    --dump-header - --output /tmp/webconfig-response \
    http://webconfig:9000/api/v1/device/02:00:00:00:00:01/config >"$headers"
podman cp "$deployment-gateway-1:/tmp/webconfig-response" "$response"
grep --quiet -i '^Content-Type: multipart/mixed' "$headers"
grep --quiet 'privatessid' "$response"

podman exec "$deployment-gateway-1" rbuscli -s Device.X_RDK_WebConfig.ForceSync string root >/dev/null
ssid=""
for _ in {1..60}; do
    ssid=$(podman exec "$deployment-gateway-1" rbuscli -g Device.WiFi.SSID.1.SSID 2>/dev/null || true)
    [[ "$ssid" == *"vcpe-private"* ]] && break
    sleep 2
done
if [[ "$ssid" != *"vcpe-private"* ]]; then
    echo "Gateway webcfg client did not apply the seeded privatessid fixture: $ssid" >&2
    exit 1
fi

echo "PASS: WebConfig no-auth multipart download, BNG DNS, and Gateway webcfg application verified"