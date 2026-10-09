#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)

if [[ "${VCPE_RUN_CROSS_NETWORK_DNS:-}" != "1" ]]; then
	echo "SKIP: set VCPE_RUN_CROSS_NETWORK_DNS=1 to run deployed cross-network DNS coverage"
	exit 0
fi

if ! command -v podman >/dev/null 2>&1; then
	echo "SKIP: podman is not installed"
	exit 0
fi

deployment=example
manifest="$repo_root/manifests/dev/example.yaml"
vcpe="$repo_root/controlplane/bin/vcpe"
state_root=$(mktemp -d)
bng_paused=0

cleanup() {
	if [[ "$bng_paused" == "1" ]]; then
		podman unpause "$deployment-bng-1" >/dev/null 2>&1 || true
	fi
	"$vcpe" down --name "$deployment" --state-root "$state_root" >/dev/null 2>&1 || true
	rm -rf "$state_root"
}
trap cleanup EXIT

cd "$repo_root"
make build
scripts/stage-runtime-init-binaries bng webpa

podman_arch=$(podman info --format '{{.Host.Arch}}')
case "$podman_arch" in
	aarch64) podman_arch=arm64 ;;
	x86_64) podman_arch=amd64 ;;
esac
podman build --platform "linux/$podman_arch" -t ghcr.io/gdcs-dev/bng:dev \
	-f services/bng/Containerfile services/bng >/dev/null
podman build --platform "linux/$podman_arch" -t ghcr.io/gdcs-dev/webpa:dev \
	-f services/webpa/Containerfile services/webpa >/dev/null

"$vcpe" up --manifest "$manifest" --state-root "$state_root"

gateway_wan_ip=""
gateway_cm_ip=""
for _ in $(seq 1 30); do
	gateway_wan_ip=$(podman exec "$deployment-bng-1" awk '$2 == "gateway" { print $1; exit }' /etc/dnsmasq.dhcp.hosts 2>/dev/null || true)
	gateway_cm_ip=$(podman exec "$deployment-bng-1" awk '$2 == "gateway-1-cm" { print $1; exit }' /etc/dnsmasq.dhcp.hosts 2>/dev/null || true)
	[[ -n "$gateway_wan_ip" && -n "$gateway_cm_ip" ]] && break
	sleep 1
done
[[ -n "$gateway_wan_ip" && -n "$gateway_cm_ip" ]] || {
	echo "Gateway WAN and CM DHCP DNS records were not published" >&2
	exit 1
}

dns_config=$(podman inspect "$deployment-webpa-1" --format '{{json .HostConfig.Dns}}')
[[ "$dns_config" == '["10.10.10.10"]' ]] || {
	echo "unexpected WebPA DNS configuration: $dns_config" >&2
	exit 1
}
[[ "$(podman inspect "$deployment-webpa-1" --format '{{.State.Running}}')" == "true" ]]
podman exec "$deployment-webpa-1" grep -Fxq 'nameserver 10.10.10.1' /etc/resolv.conf

for alias in gateway gateway-1 gateway-1-wan; do
	resolved=$(podman exec "$deployment-webpa-1" getent hosts "$alias" | awk 'NR == 1 { print $1 }')
	[[ "$resolved" == "$gateway_wan_ip" ]] || {
		echo "$alias resolved to $resolved, want $gateway_wan_ip" >&2
		exit 1
	}
done
resolved=$(podman exec "$deployment-webpa-1" getent hosts gateway-1-cm | awk 'NR == 1 { print $1 }')
[[ "$resolved" == "$gateway_cm_ip" ]] || {
	echo "gateway-1-cm resolved to $resolved, want $gateway_cm_ip" >&2
	exit 1
}

podman exec "$deployment-webpa-1" curl -fsS --connect-timeout 5 http://gateway:9878/health >/dev/null

webpa_ip=$(podman inspect "$deployment-webpa-1" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
resolved=$(podman exec "$deployment-webpa-1" getent hosts webpa | awk 'NR == 1 { print $1 }')
[[ "$resolved" == "$webpa_ip" ]] || {
	echo "webpa resolved to $resolved, want Aardvark address $webpa_ip" >&2
	exit 1
}
podman exec "$deployment-webpa-1" getent hosts example.com >/dev/null
podman exec "$deployment-bng-1" grep -Fxq 'nameserver 10.10.10.1' /etc/dnsmasq.upstream-resolv.conf

podman pause "$deployment-bng-1" >/dev/null
bng_paused=1
resolved=$(podman exec "$deployment-webpa-1" getent hosts webpa | awk 'NR == 1 { print $1 }')
[[ "$resolved" == "$webpa_ip" ]] || {
	echo "Aardvark-owned webpa alias failed while BNG was unavailable" >&2
	exit 1
}
podman unpause "$deployment-bng-1" >/dev/null
bng_paused=0
bng_ready=""
for _ in $(seq 1 30); do
	if podman exec "$deployment-webpa-1" getent hosts example.com >/dev/null 2>&1; then
		bng_ready=1
		break
	fi
	sleep 1
done
[[ -n "$bng_ready" ]] || {
	echo "BNG DNS forwarding did not recover after restart" >&2
	exit 1
}

gateway_device=$(podman exec "$deployment-gateway-1" printenv IFACE_WAN_DEVICE)
podman exec "$deployment-gateway-1" dhclient -r "$gateway_device"
for _ in $(seq 1 30); do
	if ! podman exec "$deployment-webpa-1" getent hosts gateway >/dev/null 2>&1; then
		break
	fi
	sleep 1
done
if podman exec "$deployment-webpa-1" getent hosts gateway >/dev/null 2>&1; then
	echo "Gateway alias remained after DHCP release" >&2
	exit 1
fi

podman exec "$deployment-gateway-1" dhclient -v "$gateway_device" >/dev/null
renewed_ip=""
for _ in $(seq 1 30); do
	renewed_ip=$(podman exec "$deployment-webpa-1" getent hosts gateway 2>/dev/null | awk 'NR == 1 { print $1 }' || true)
	[[ -n "$renewed_ip" ]] && break
	sleep 1
done
[[ -n "$renewed_ip" ]] || {
	echo "Gateway alias was not republished after DHCP renewal" >&2
	exit 1
}

echo "PASS: management services resolve DHCP-attached device aliases through BNG"