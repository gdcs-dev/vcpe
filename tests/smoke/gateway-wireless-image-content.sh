#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
containerfile="$repo_root/services/gateway/Containerfile"
unit="$repo_root/services/gateway/container/vcpe-hostapd.service"
wrapper="$repo_root/services/gateway/container/hostapd-wrapper.sh"
mesh_unit="$repo_root/services/gateway/container/vcpe-mesh.service"
mesh_wrapper="$repo_root/services/gateway/container/mesh-wrapper.sh"

grep -q 'ARG HOSTAPD_VERSION=2.11' "$containerfile"
grep -q 'ARG HOSTAPD_SHA256=2b3facb632fd4f65e32f4bf82a76b4b72c501f995a4f62e330219fe7aed1747a' "$containerfile"
grep -q 'sha256sum -c -' "$containerfile"
grep -q 'CONFIG_IEEE80211AX=y' "$containerfile"
grep -q 'CONFIG_MESH=y' "$containerfile"
grep -q 'CONFIG_SAE=y' "$containerfile"
grep -q 'ARG WPA_SUPPLICANT_SHA256=912ea06f74e30a8e36fbb68064d6cdff218d8d591db0fc5d75dee6c81ac7fc0a' "$containerfile"
grep -q 'wpa_supplicant.tar.gz" | sha256sum -c -' "$containerfile"
grep -q 'wpa_supplicant/wpa_supplicant /usr/local/sbin/wpa_supplicant' "$containerfile"
grep -Eq 'netplan\.io .*iw hostapd' "$containerfile"
grep -q 'systemctl disable hostapd.service' "$containerfile"
grep -q 'systemctl enable vcpe-hostapd.service' "$containerfile"
grep -q 'container/hostapd-wrapper.sh /usr/local/bin/vcpe-hostapd' "$containerfile"
grep -q 'ExecStart=/usr/local/bin/vcpe-hostapd' "$unit"
grep -q 'ConditionPathExists=/etc/vcpe/gateway-network-ready' "$unit"
grep -q 'ConditionPathExistsGlob=/etc/vcpe/hostapd/\*.conf' "$unit"
test -s "$wrapper"
grep -q 'systemctl enable vcpe-mesh.service' "$containerfile"
grep -q 'container/mesh-wrapper.sh /usr/local/bin/vcpe-mesh' "$containerfile"
grep -q 'LABEL org.gdcs-dev.vcpe.gateway.mesh-sae=1' "$containerfile"
grep -q 'ExecStart=/usr/local/bin/vcpe-mesh' "$mesh_unit"
grep -q 'ConditionPathExistsGlob=/etc/vcpe/mesh/\*.conf' "$mesh_unit"
test -s "$mesh_wrapper"

if [[ -n "${VCPE_GATEWAY_IMAGE:-}" ]]; then
    podman run --rm --entrypoint /bin/bash "$VCPE_GATEWAY_IMAGE" -ceu '
        test -x /usr/sbin/iw
        test -x /usr/sbin/hostapd
        test -x /usr/local/sbin/wpa_supplicant
        test -x /usr/local/bin/wpa_cli
        ! ldd /usr/local/sbin/wpa_supplicant | grep -q "not found"
        hostapd_version=$(hostapd -v 2>&1 || true)
        grep -q "hostapd v2.11" <<<"$hostapd_version"
        test -x /usr/local/bin/vcpe-hostapd
        test -f /etc/systemd/system/vcpe-hostapd.service
        test -L /etc/systemd/system/multi-user.target.wants/vcpe-hostapd.service
        test -x /usr/local/bin/vcpe-mesh
        test -L /etc/systemd/system/multi-user.target.wants/vcpe-mesh.service
        test ! -e /etc/systemd/system/multi-user.target.wants/hostapd.service
    '
fi

echo 'gateway wireless image content: ok'