#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
entrypoint="$repo_root/services/gateway/container/entrypoint.sh"

mesh_work=$(mktemp -d)
trap 'rm -rf "$mesh_work"' EXIT
mkdir -p "$mesh_work/bin" "$mesh_work/policy" "$mesh_work/runtime"
printf '%s' 'private-mesh-password' > "$mesh_work/credential"
chmod 600 "$mesh_work/credential"
printf 'device=mesh0\nid=mesh-home\nbridge=brlan\nfrequency=5180\ncredential=%s\n' "$mesh_work/credential" > "$mesh_work/policy/backhaul.conf"
cat > "$mesh_work/bin/ip" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MESH_IP_LOG"
if [[ "$*" == '-o link show dev mesh0' ]]; then
    printf '%s\n' '2: mesh0: <BROADCAST,UP> master brlan state UP'
fi
EOF
cat > "$mesh_work/bin/iw" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == *'station dump' ]]; then
    printf 'Station 02:00:00:00:00:02 (on mesh0)\n\tmesh plink: %s\n\tauthorized: yes\n' "${MESH_PEER_STATE:-ESTAB}"
else
    printf '%s\n' 'type mesh point'
fi
EOF
cat > "$mesh_work/bin/timeout" <<'EOF'
#!/usr/bin/env bash
shift
exec "$@"
EOF
cat > "$mesh_work/bin/wpa_cli" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 'wpa_state=COMPLETED'
EOF
cat > "$mesh_work/bin/wpa_supplicant" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$MESH_SUPPLICANT_ARGS"
EOF
chmod +x "$mesh_work/bin/"*
touch "$mesh_work/network-ready"
env PATH="$mesh_work/bin:$PATH" MESH_IP_LOG="$mesh_work/ip.log" MESH_SUPPLICANT_ARGS="$mesh_work/supplicant.args" \
    VCPE_MESH_POLICY_DIR="$mesh_work/policy" VCPE_MESH_RUNTIME_DIR="$mesh_work/runtime" \
    VCPE_NETWORK_READY_FILE="$mesh_work/network-ready" VCPE_MESH_SUPPLICANT_BIN="$mesh_work/bin/wpa_supplicant" \
    bash "$repo_root/services/gateway/container/mesh-wrapper.sh"
grep -qx 'link set mesh0 master brlan' "$mesh_work/ip.log"
grep -q 'frequency=5180' "$mesh_work/runtime/supplicant.conf"
grep -q 'key_mgmt=SAE' "$mesh_work/runtime/supplicant.conf"
grep -q 'psk="private-mesh-password"' "$mesh_work/runtime/supplicant.conf"
if grep -q 'private-mesh-password' "$mesh_work/supplicant.args" "$mesh_work/ip.log"; then
    echo 'mesh credential leaked to process arguments' >&2
    exit 1
fi
printf '%s\n' '{"radios":[{"name":"backhaul","mode":"mesh","device":"mesh0","bridge":"brlan"}]}' > "$mesh_work/contract.json"
env PATH="$mesh_work/bin:$PATH" MESH_IP_LOG="$mesh_work/ip.log" VCPE_STARTUP_CONTRACT="$mesh_work/contract.json" \
    sh "$repo_root/services/gateway/container/health-probe.sh" mesh > "$mesh_work/peer.json"
jq -e '.radio == "backhaul" and .joined == true and .peers == 1 and .bridge == "brlan"' "$mesh_work/peer.json" >/dev/null
if env PATH="$mesh_work/bin:$PATH" MESH_PEER_STATE=LISTEN MESH_IP_LOG="$mesh_work/ip.log" VCPE_STARTUP_CONTRACT="$mesh_work/contract.json" \
    sh "$repo_root/services/gateway/container/health-probe.sh" mesh > "$mesh_work/peer.json"; then
    echo 'mesh health probe accepted an unauthenticated peer' >&2
    exit 1
fi
jq -e '.peers == 0' "$mesh_work/peer.json" >/dev/null
rm "$mesh_work/credential"
if env PATH="$mesh_work/bin:$PATH" VCPE_MESH_POLICY_DIR="$mesh_work/policy" \
    VCPE_MESH_RUNTIME_DIR="$mesh_work/runtime" VCPE_NETWORK_READY_FILE="$mesh_work/network-ready" \
    bash "$repo_root/services/gateway/container/mesh-wrapper.sh" > "$mesh_work/error.log" 2>&1; then
    echo 'mesh startup accepted a missing protected credential' >&2
    exit 1
fi
if grep -q 'private-mesh-password' "$mesh_work/error.log"; then
    echo 'mesh startup error leaked the credential' >&2
    exit 1
fi

if ((BASH_VERSINFO[0] < 4)); then
    grep -q 'RADIO_\*_MAC' "$entrypoint"
    grep -q 'RADIO_\*_BRIDGE' "$entrypoint"
    grep -q 'RADIO_\*_VAP_\*_BRIDGE' "$entrypoint"
    grep -q 'RADIO_\*_MEDIUM' "$entrypoint"
    grep -q 'name.*==.*tmp-${desired_target}' "$entrypoint"
    grep -q '== "$temp_name"' "$entrypoint"
    grep -q 'ip link set "$dev" nomaster' "$entrypoint"
	grep -q '\[\[ "${!mode_var:-}" == mesh \]\] && continue' "$entrypoint"
    grep -q 'gateway-network-ready' "$entrypoint"
    grep -q 'head -1 || true' "$entrypoint"
    echo 'gateway wireless entrypoint: source assertions ok (Bash 4+ required for dynamic fixture)'
    exit 0
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

mock_dir="$work_dir/bin"
net_root="$work_dir/net"
mkdir -p "$mock_dir" "$net_root/eth0" "$net_root/phy7"
printf '%s\n' '02:00:00:00:00:99' >"$net_root/eth0/address"
printf '%s\n' '02:00:00:00:00:10' >"$net_root/phy7/address"

cat >"$mock_dir/ip" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$IP_LOG"
EOF
chmod +x "$mock_dir/ip"

cat >"$mock_dir/sysctl" <<'EOF'
#!/usr/bin/env bash
printf 'sysctl %s\n' "$*" >>"$IP_LOG"
EOF
chmod +x "$mock_dir/sysctl"

rename_log="$work_dir/rename.log"
env \
    PATH="$mock_dir:$PATH" \
    IP_LOG="$rename_log" \
    VCPE_SYS_CLASS_NET_ROOT="$net_root" \
    RADIO_AP_MAC='02:00:00:00:00:10' \
    RADIO_AP_DEVICE='wlan0' \
    ENTRYPOINT="$entrypoint" \
    bash -c 'source "$ENTRYPOINT"; rename_interfaces_by_mac'

if grep -q 'link set eth0 name vcpe-health0' "$rename_log"; then
    echo 'radio-only Gateway renamed Podman-owned eth0' >&2
    exit 1
fi
grep -qx 'link set phy7 down' "$rename_log"
grep -qx 'link set phy7 name tmp-wlan0' "$rename_log"
grep -qx 'link set tmp-wlan0 name wlan0' "$rename_log"
if grep -q 'phy7 name vcpe-health0' "$rename_log"; then
    echo 'declared radio was misclassified as the health interface' >&2
    exit 1
fi
managed_log="$work_dir/managed-eth0.log"
env \
    PATH="$mock_dir:$PATH" \
    IP_LOG="$managed_log" \
    VCPE_SYS_CLASS_NET_ROOT="$net_root" \
    IFACE_WAN_MAC='02:00:00:00:00:11' \
    IFACE_WAN_DEVICE='eth0' \
    RADIO_AP_MAC='02:00:00:00:00:10' \
    RADIO_AP_DEVICE='wlan0' \
    ENTRYPOINT="$entrypoint" \
    bash -c 'source "$ENTRYPOINT"; rename_interfaces_by_mac'
grep -qx 'link set eth0 name vcpe-health0' "$managed_log"

network_log="$work_dir/network.log"
env \
    PATH="$mock_dir:$PATH" \
    IP_LOG="$network_log" \
    VCPE_NETWORK_READY_FILE="$work_dir/network-ready" \
    BRIDGE_BRLAN0_NAME='brlan0' \
    RADIO_AP_DEVICE='wlan0' \
    RADIO_AP_MODE='ap' \
    RADIO_AP_BRIDGE='brlan0' \
    ENTRYPOINT="$entrypoint" \
    bash -c 'source "$ENTRYPOINT"; configure_networking'

grep -qx 'link set wlan0 up' "$network_log"
grep -qx 'link set wlan0 nomaster' "$network_log"
grep -qx 'link set wlan0 master brlan0' "$network_log"
grep -qx 'addr flush dev wlan0' "$network_log"

media_log="$work_dir/media-network.log"
env \
    PATH="$mock_dir:$PATH" \
    IP_LOG="$media_log" \
    VCPE_NETWORK_READY_FILE="$work_dir/media-network-ready" \
    BRIDGE_BRLAN_NAME='brlan' \
    BRIDGE_BRGUEST_NAME='brguest' \
    RADIO_AP_DEVICE='wlan0' \
    RADIO_AP_MODE='ap' \
    RADIO_AP_MEDIUM='rf5' \
    RADIO_AP_VAP_0_BRIDGE='brlan' \
    RADIO_AP_VAP_0_DEVICE='wlan0' \
    RADIO_AP_VAP_1_BRIDGE='brguest' \
    RADIO_AP_VAP_1_DEVICE='vap1' \
    ENTRYPOINT="$entrypoint" \
    bash -c 'source "$ENTRYPOINT"; configure_networking'

grep -qx 'link add brlan type bridge' "$media_log"
grep -qx 'link add brguest type bridge' "$media_log"
grep -qx 'sysctl -q -w net.ipv4.conf.all.arp_ignore=1' "$media_log"
grep -qx 'link set wlan0 up' "$media_log"
grep -qx 'link set wlan0 nomaster' "$media_log"
grep -qx 'addr flush dev wlan0' "$media_log"
if grep -q 'link set .* master brlan\|link set .* master brguest\|link set vap1\|link add vap1' "$media_log"; then
    echo 'Gateway bootstrap pre-created or enslaved a hostapd-owned VAP' >&2
    exit 1
fi
awk '/^link add brlan type bridge$/{created=NR} /^link set wlan0 up$/{started=NR} END{exit !(created > 0 && started > created)}' "$media_log"

mesh_log="$work_dir/mesh-network.log"
env \
    PATH="$mock_dir:$PATH" \
    IP_LOG="$mesh_log" \
    VCPE_NETWORK_READY_FILE="$work_dir/mesh-network-ready" \
    BRIDGE_BRLAN_NAME='brlan' \
    RADIO_BACKHAUL_DEVICE='mesh0' \
    RADIO_BACKHAUL_MODE='mesh' \
    RADIO_BACKHAUL_MEDIUM='rf5' \
    RADIO_BACKHAUL_BRIDGE='brlan' \
    ENTRYPOINT="$entrypoint" \
    bash -c 'source "$ENTRYPOINT"; configure_networking'
if grep -q '^link set mesh0 ' "$mesh_log"; then
    echo 'Gateway bootstrap touched the late-attached mesh interface' >&2
    exit 1
fi

echo 'gateway wireless entrypoint: ok'