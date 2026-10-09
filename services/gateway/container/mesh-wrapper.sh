#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
umask 077

policy_dir="${VCPE_MESH_POLICY_DIR:-/etc/vcpe/mesh}"
runtime_dir="${VCPE_MESH_RUNTIME_DIR:-/run/vcpe/mesh}"
ready_file="${VCPE_NETWORK_READY_FILE:-/etc/vcpe/gateway-network-ready}"

if [[ ! -e "$ready_file" ]]; then
    echo 'vcpe-mesh: Gateway bridge is not ready' >&2
    exit 1
fi
shopt -s nullglob
policies=("$policy_dir"/*.conf)
if (( ${#policies[@]} != 1 )); then
    echo 'vcpe-mesh: expected exactly one mesh radio policy' >&2
    exit 1
fi

device='' mesh_id='' bridge='' frequency='' credential=''
while IFS='=' read -r key value; do
    case "$key" in
        device) device=$value ;;
        id) mesh_id=$value ;;
        bridge) bridge=$value ;;
        frequency) frequency=$value ;;
        credential) credential=$value ;;
        *) echo 'vcpe-mesh: invalid mesh radio policy' >&2; exit 1 ;;
    esac
done < "${policies[0]}"
if [[ ! "$device" =~ ^[a-zA-Z0-9_.-]{1,15}$ || ! "$bridge" =~ ^[a-zA-Z0-9_.-]{1,15}$ ||
      -z "$mesh_id" || "$mesh_id" == *$'\n'* || "$mesh_id" == *$'\r'* ||
      ! "$frequency" =~ ^[0-9]{4}$ || ! -f "$credential" || -L "$credential" ]]; then
    echo 'vcpe-mesh: invalid mesh radio policy or protected credential file' >&2
    exit 1
fi
credential_mode=$(stat -c '%a' "$credential" 2>/dev/null || stat -f '%Lp' "$credential")
if [[ "$credential_mode" != 600 ]]; then
    echo 'vcpe-mesh: protected credential file must have mode 0600' >&2
    exit 1
fi
passphrase=$(<"$credential")
if (( ${#passphrase} < 8 || ${#passphrase} > 63 )) || printf '%s' "$passphrase" | grep -q '[^ -~]'; then
    echo 'vcpe-mesh: protected credential is not 8 through 63 printable ASCII characters' >&2
    exit 1
fi

for (( attempt=0; attempt<30; attempt++ )); do
    if ip link show dev "$device" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
if ! ip link show dev "$device" >/dev/null 2>&1; then
    echo "vcpe-mesh: mesh radio $device was not attached within 30 seconds" >&2
    exit 1
fi
if ! iw dev "$device" info | grep -q 'type mesh point'; then
    echo "vcpe-mesh: radio $device is not a mesh point" >&2
    exit 1
fi
ip link show dev "$bridge" >/dev/null 2>&1 || { echo "vcpe-mesh: bridge $bridge is missing" >&2; exit 1; }
ip link set "$device" up
ip link set "$device" master "$bridge"
ip addr flush dev "$device"

mkdir -p "$runtime_dir/control"
chmod 700 "$runtime_dir" "$runtime_dir/control"
temporary=$(mktemp "$runtime_dir/.supplicant.XXXXXX")
trap 'rm -f "$temporary"' EXIT
mesh_id=${mesh_id//\\/\\\\}
mesh_id=${mesh_id//\"/\\\"}
passphrase=${passphrase//\\/\\\\}
passphrase=${passphrase//\"/\\\"}
printf 'ctrl_interface=%s/control\nnetwork={\n    ssid="%s"\n    mode=5\n    frequency=%s\n    key_mgmt=SAE\n    psk="%s"\n}\n' \
    "$runtime_dir" "$mesh_id" "$frequency" "$passphrase" > "$temporary"
unset passphrase
chmod 600 "$temporary"
mv -f "$temporary" "$runtime_dir/supplicant.conf"
trap - EXIT
exec "${VCPE_MESH_SUPPLICANT_BIN:-/usr/local/sbin/wpa_supplicant}" -i "$device" -D nl80211 -b "$bridge" -c "$runtime_dir/supplicant.conf"