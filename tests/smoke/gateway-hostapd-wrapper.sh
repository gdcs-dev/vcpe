#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
wrapper="$repo_root/services/gateway/container/hostapd-wrapper.sh"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

mock_dir="$work_dir/bin"
config_dir="$work_dir/hostapd"
runtime_dir="$work_dir/runtime"
ready_file="$work_dir/gateway-network-ready"
probe="$repo_root/services/gateway/container/health-probe.sh"
export VCPE_HOSTAPD_CTRL_DIR="$work_dir/control"
mkdir -p "$mock_dir" "$config_dir"

cat >"$mock_dir/ip" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == 'link show dev wlan0' || "$*" == '-o link show dev wlan0' ]]; then
    if [[ "${AP_UNBRIDGED:-}" == 1 ]]; then
        echo '5: wlan0: <BROADCAST,UP> mtu 1500 state UP'
    else
        echo '5: wlan0: <BROADCAST,UP> mtu 1500 master brlan0 state UP'
    fi
    exit 0
fi
if [[ "$*" == 'link show dev vap7' || "$*" == '-o link show dev vap7' ]]; then
    echo "6: vap7: <BROADCAST,UP> mtu 1500 master ${BAD_VAP_BRIDGE:-brguest} state UP"
    exit 0
fi
exit 1
EOF
cat >"$mock_dir/hostapd" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$HOSTAPD_LOG"
EOF
cat >"$mock_dir/hostapd_cli" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == *'-i wlan0 status' ]]; then
    printf 'state=ENABLED\nbssid[0]=02:00:00:00:00:10\nbssid[1]=02:00:00:00:00:17\n'
elif [[ "$*" == *'-i vap7 status' ]]; then
    printf 'state=%s\nbssid[0]=02:00:00:00:00:10\nbssid[1]=%s\n' "${VAP_STATE:-ENABLED}" "${VAP_BSSID:-02:00:00:00:00:17}"
elif [[ "$*" == *'-i wlan0 get_config' ]]; then
    printf 'bssid=02:00:00:00:00:10\n'
elif [[ "$*" == *'-i vap7 get_config' ]]; then
    printf 'bssid=%s\n' "${VAP_BSSID:-02:00:00:00:00:17}"
else
    exit 1
fi
EOF
cat >"$mock_dir/timeout" <<'EOF'
#!/usr/bin/env bash
shift
exec "$@"
EOF
chmod +x "$mock_dir/ip" "$mock_dir/hostapd" "$mock_dir/hostapd_cli" "$mock_dir/timeout"

printf '%s\n' \
    'interface=wlan0' \
    'bridge=brlan0' \
    'ssid=vcpe-lab' \
    'channel=1' >"$config_dir/ap.conf"

if env PATH="$mock_dir:$PATH" VCPE_NETWORK_READY_FILE="$ready_file" VCPE_HOSTAPD_CONFIG_DIR="$config_dir" bash "$wrapper" >"$work_dir/missing.out" 2>&1; then
    echo 'hostapd wrapper unexpectedly started before Gateway networking was ready' >&2
    exit 1
fi
grep -q 'Gateway networking is not ready' "$work_dir/missing.out"

touch "$ready_file"
hostapd_log="$work_dir/hostapd.log"
if env PATH="$mock_dir:$PATH" AP_UNBRIDGED=1 VCPE_HOSTAPD_BIN="$mock_dir/hostapd" VCPE_NETWORK_READY_FILE="$ready_file" VCPE_HOSTAPD_CONFIG_DIR="$config_dir" VCPE_HOSTAPD_RUNTIME_DIR="$runtime_dir" bash "$wrapper" >"$work_dir/legacy-unbridged.out" 2>&1; then
    echo 'hostapd wrapper unexpectedly accepted an unbridged legacy AP' >&2
    exit 1
fi
grep -q 'is not attached to bridge' "$work_dir/legacy-unbridged.out"
env \
    PATH="$mock_dir:$PATH" \
    HOSTAPD_LOG="$hostapd_log" \
    VCPE_HOSTAPD_BIN="$mock_dir/hostapd" \
    VCPE_NETWORK_READY_FILE="$ready_file" \
    VCPE_HOSTAPD_CONFIG_DIR="$config_dir" \
    VCPE_HOSTAPD_RUNTIME_DIR="$runtime_dir" \
    bash "$wrapper"
grep -qx -- "$runtime_dir/ap.conf" "$hostapd_log"
[[ $(stat -f '%Lp' "$runtime_dir/ap.conf") == 600 ]]
cmp -s "$config_dir/ap.conf" "$runtime_dir/ap.conf"

credential="$work_dir/passphrase"
printf '%s' 'correct horse battery staple' >"$credential"
chmod 600 "$credential"
for security in wpa2-personal wpa3-personal; do
    printf '%s\n' \
        'interface=wlan0' \
        'bridge=brlan0' \
        'ssid=vcpe-lab' \
        'channel=1' \
        "# vcpe-security=$security" \
        "# vcpe-passphrase-file=$credential" >"$config_dir/ap.conf"
    rm -rf "$runtime_dir"
    env \
        PATH="$mock_dir:$PATH" \
        HOSTAPD_LOG="$hostapd_log" \
        VCPE_HOSTAPD_BIN="$mock_dir/hostapd" \
        VCPE_NETWORK_READY_FILE="$ready_file" \
        VCPE_HOSTAPD_CONFIG_DIR="$config_dir" \
        VCPE_HOSTAPD_RUNTIME_DIR="$runtime_dir" \
        bash "$wrapper"
    grep -qx -- "$runtime_dir/ap.conf" "$hostapd_log"
    [[ $(stat -f '%Lp' "$runtime_dir/ap.conf") == 600 ]]
    if [[ "$security" == wpa2-personal ]]; then
        grep -q '^wpa_key_mgmt=WPA-PSK$' "$runtime_dir/ap.conf"
        ! grep -q '^wpa_key_mgmt=SAE$' "$runtime_dir/ap.conf"
    else
        grep -q '^wpa_key_mgmt=SAE$' "$runtime_dir/ap.conf"
        grep -q '^ieee80211w=2$' "$runtime_dir/ap.conf"
        ! grep -q 'WPA-PSK' "$runtime_dir/ap.conf"
    fi
    grep -q '^rsn_pairwise=CCMP$' "$runtime_dir/ap.conf"
done

home_credential="$work_dir/home-passphrase"
iot_credential="$work_dir/iot-passphrase"
printf '%s' 'home-only-passphrase' >"$home_credential"
printf '%s' 'iot-only-passphrase' >"$iot_credential"
chmod 600 "$home_credential" "$iot_credential"
printf '%s\n' \
    'interface=wlan0' 'bssid=02:00:00:00:00:10' 'bridge=brlan0' 'ssid=Home' 'channel=36' \
    '# vcpe-security=wpa3-personal' "# vcpe-passphrase-file=$home_credential" \
    'bss=vap1' 'bridge=briot' 'ssid=IoT' \
    '# vcpe-security=wpa2-personal' "# vcpe-passphrase-file=$iot_credential" \
    'bss=vap2' 'bridge=brguest' 'ssid=Guest' 'wpa=0' >"$config_dir/ap.conf"
rm -rf "$runtime_dir"
env \
    PATH="$mock_dir:$PATH" \
    AP_UNBRIDGED=1 \
    HOSTAPD_LOG="$hostapd_log" \
    VCPE_HOSTAPD_BIN="$mock_dir/hostapd" \
    VCPE_NETWORK_READY_FILE="$ready_file" \
    VCPE_HOSTAPD_CONFIG_DIR="$config_dir" \
    VCPE_HOSTAPD_RUNTIME_DIR="$runtime_dir" \
    bash "$wrapper"
[[ $(stat -f '%Lp' "$runtime_dir/ap.conf") == 600 ]]
grep -qx -- "$runtime_dir/ap.conf" "$hostapd_log"
[[ $(grep -c "^ctrl_interface=$VCPE_HOSTAPD_CTRL_DIR$" "$runtime_dir/ap.conf") == 3 ]]
awk '/^bss=/{exit} /^wpa_key_mgmt=SAE$/{sae=1} /^sae_password=home-only-passphrase$/{password=1} END{exit !(sae && password)}' "$runtime_dir/ap.conf"
awk '/^bss=vap1$/{active=1; next} /^bss=/{active=0} active && /^wpa_key_mgmt=WPA-PSK$/{psk=1} active && /^wpa_passphrase=iot-only-passphrase$/{password=1} END{exit !(psk && password)}' "$runtime_dir/ap.conf"
awk '/^bss=vap2$/{active=1; next} active && /^wpa=0$/{open=1} active && /^(wpa_passphrase|sae_password)=/{secret=1} END{exit !(open && !secret)}' "$runtime_dir/ap.conf"
! grep -q '^# vcpe-' "$runtime_dir/ap.conf"

printf '%s\n' 'interface=wlan0' 'bridge=brlan0' 'ssid=Home' \
    '# vcpe-security=wpa3-personal' '# vcpe-passphrase-file=/missing/private' >"$config_dir/ap.conf"
if env PATH="$mock_dir:$PATH" VCPE_NETWORK_READY_FILE="$ready_file" VCPE_HOSTAPD_CONFIG_DIR="$config_dir" VCPE_HOSTAPD_RUNTIME_DIR="$runtime_dir" bash "$wrapper" >"$work_dir/missing-credential.out" 2>&1; then
    echo 'hostapd wrapper unexpectedly accepted a missing BSS credential' >&2
    exit 1
fi
! grep -q 'home-only-passphrase\|iot-only-passphrase' "$work_dir/missing-credential.out"

printf '%s\n' 'interface=wlan0' 'bridge=brlan0' 'ssid=Home' \
    '# vcpe-security=wpa3-personal' "# vcpe-passphrase-file=$credential" >"$config_dir/ap.conf"
printf '%s' 'short' >"$credential"
if env PATH="$mock_dir:$PATH" VCPE_NETWORK_READY_FILE="$ready_file" VCPE_HOSTAPD_CONFIG_DIR="$config_dir" VCPE_HOSTAPD_RUNTIME_DIR="$runtime_dir" bash "$wrapper" >"$work_dir/invalid.out" 2>&1; then
    echo 'hostapd wrapper unexpectedly accepted an invalid credential' >&2
    exit 1
fi
! grep -q 'short' "$work_dir/invalid.out"

cat >"$work_dir/startup-contract.json" <<'EOF'
{"radios":[{"name":"ap5","mode":"ap","vaps":[
    {"slot":0,"device":"wlan0","bssid":"02:00:00:00:00:10","bridge":"brlan0"},
    {"slot":7,"device":"vap7","bssid":"02:00:00:00:00:17","bridge":"brguest"}
]}]}
EOF
probe_env=(PATH="$mock_dir:$PATH" VCPE_STARTUP_CONTRACT="$work_dir/startup-contract.json" VCPE_HOSTAPD_CTRL_DIR="$work_dir/control")
env "${probe_env[@]}" sh "$probe" vaps >"$work_dir/vap-status"
jq -e 'select(.radio == "ap5" and .slot == 0 and .interface == "wlan0" and .bssid == "02:00:00:00:00:10" and .enabled == true and .bridge == "brlan0")' "$work_dir/vap-status" >/dev/null
jq -e 'select(.radio == "ap5" and .slot == 7 and .interface == "vap7" and .bssid == "02:00:00:00:00:17" and .enabled == true and .bridge == "brguest")' "$work_dir/vap-status" >/dev/null
if env "${probe_env[@]}" BAD_VAP_BRIDGE=brwrong sh "$probe" vaps >"$work_dir/wrong-bridge"; then
        echo 'hostapd probe accepted a BSS on the wrong bridge' >&2
        exit 1
fi
jq -e 'select(.slot == 7 and .bridge == "brwrong")' "$work_dir/wrong-bridge" >/dev/null
if env "${probe_env[@]}" VAP_STATE=DISABLED sh "$probe" vaps >"$work_dir/disabled"; then
        echo 'hostapd probe accepted a disabled secondary BSS' >&2
        exit 1
fi
jq -e 'select(.slot == 7 and .enabled == false)' "$work_dir/disabled" >/dev/null
if env "${probe_env[@]}" VAP_BSSID=02:00:00:00:00:18 sh "$probe" vaps >"$work_dir/wrong-bssid"; then
    echo 'hostapd probe accepted the wrong BSSID' >&2
    exit 1
fi
jq -e 'select(.slot == 7 and .bssid == "02:00:00:00:00:18")' "$work_dir/wrong-bssid" >/dev/null

echo 'gateway hostapd wrapper: ok'