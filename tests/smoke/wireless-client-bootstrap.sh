#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
bootstrap="$repo_root/services/wireless-client/bootstrap.sh"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
mock_dir="$work_dir/bin"
mkdir -p "$mock_dir"

cat >"$mock_dir/ip" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$COMMAND_LOG"
if [[ "$*" == 'link show dev wlan0' ]]; then
    [[ "${MOCK_DEVICE:-1}" == 1 ]]
elif [[ "$*" == 'link show dev wlan1' ]]; then
    true
elif [[ "$*" == '-4 -o addr show dev wlan0' ]]; then
    echo '5: wlan0 inet 192.168.10.20/24 scope global wlan0'
elif [[ "$*" == '-4 -o addr show dev wlan1' ]]; then
    echo '6: wlan1 inet 192.168.20.20/24 scope global wlan1'
fi
EOF
cat >"$mock_dir/iw" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$COMMAND_LOG"
if [[ "${MOCK_ASSOCIATED:-1}" == 1 ]]; then
    bssid="${MOCK_LINK_BSSID:-02:00:00:00:00:10}"
    if [[ -n "${MOCK_LINK_SEQUENCE_FILE:-}" ]]; then
        link_count=0
        [[ -f "$MOCK_LINK_SEQUENCE_FILE" ]] && read -r link_count <"$MOCK_LINK_SEQUENCE_FILE"
        printf '%s\n' "$((link_count + 1))" >"$MOCK_LINK_SEQUENCE_FILE"
        ((link_count > 0)) && bssid='02:00:00:00:00:11'
    fi
    printf 'Connected to %s (on wlan0)\n' "$bssid"
    printf '%s\n' 'SSID: vcpe-lab'
else
    echo 'Not connected.'
fi
EOF
chmod +x "$mock_dir/ip" "$mock_dir/iw"
for command in wpa_supplicant dhclient; do
    cat >"$mock_dir/$command" <<'EOF'
#!/usr/bin/env bash
printf '%s %s\n' "$(basename "$0")" "$*" >>"$COMMAND_LOG"
EOF
    chmod +x "$mock_dir/$command"
done
cat >"$mock_dir/wpa_cli" <<'EOF'
#!/usr/bin/env bash
printf '%s %s\n' "$(basename "$0")" "$*" >>"$COMMAND_LOG"
if [[ "${MOCK_AUTHORIZED:-1}" == 1 && ( "$*" != *'wlan1'* || "${MOCK_SECOND_AUTHORIZED:-1}" == 1 ) ]]; then
    printf '%s\n' 'wpa_state=COMPLETED' 'ssid=vcpe-lab' 'key_mgmt=SAE' 'pairwise_cipher=CCMP'
else
    printf '%s\n' 'wpa_state=SCANNING' 'ssid=vcpe-lab'
fi
EOF
chmod +x "$mock_dir/wpa_cli"

run_client() {
    local runtime_dir=$1
    shift
    env \
        PATH="$mock_dir:$PATH" \
        COMMAND_LOG="$work_dir/commands.log" \
        VCPE_RUNTIME_DIR="$runtime_dir" \
        VCPE_WIRELESS_CLIENT_ONESHOT=1 \
        RADIO_WIFI_DEVICE=wlan0 \
        RADIO_WIFI_MODE=station \
        RADIO_WIFI_SSID=vcpe-lab \
        RADIO_WIFI_ADDRESSING=dhcp \
        RADIO_WIFI_DEFAULT_ROUTE=false \
        "$@" \
        bash "$bootstrap"
}

if run_client "$work_dir/missing" MOCK_DEVICE=0 >"$work_dir/missing.out" 2>&1; then
    echo 'wireless client unexpectedly accepted a missing device' >&2
    exit 1
fi
grep -q 'radio device wlan0 is missing' "$work_dir/missing.out"

if run_client "$work_dir/timeout" MOCK_ASSOCIATED=0 VCPE_ASSOCIATION_TIMEOUT_SECONDS=0 >"$work_dir/timeout.out" 2>&1; then
    echo 'wireless client unexpectedly accepted an association timeout' >&2
    exit 1
fi
grep -q 'association timed out.*vcpe-lab' "$work_dir/timeout.out" || {
    cat "$work_dir/timeout.out" >&2
    exit 1
}

: >"$work_dir/commands.log"
run_client "$work_dir/success"
grep -q 'ssid="vcpe-lab"' "$work_dir/success/wpa-WIFI.conf"
grep -q 'key_mgmt=NONE' "$work_dir/success/wpa-WIFI.conf"
grep -q '^dhclient -v -1 wlan0$' "$work_dir/commands.log"
grep -q '^state=ready$' "$work_dir/success/status"
grep -q '^device=wlan0$' "$work_dir/success/status"
grep -q '^ipv4=192.168.10.20/24$' "$work_dir/success/status"
grep -q '^route del default dev wlan0$' "$work_dir/commands.log"

credential="$work_dir/passphrase"
printf '%s' 'correct horse battery staple' >"$credential"
chmod 600 "$credential"
for security in wpa2-personal wpa3-personal; do
    : >"$work_dir/commands.log"
    run_client "$work_dir/$security" \
        RADIO_WIFI_SECURITY="$security" \
        RADIO_WIFI_PASSPHRASE_FILE="$credential"
    config="$work_dir/$security/wpa-WIFI.conf"
    grep -q '^    proto=RSN$' "$config"
    grep -q '^    pairwise=CCMP$' "$config"
    if [[ "$security" == wpa2-personal ]]; then
        grep -q '^    key_mgmt=WPA-PSK$' "$config"
        ! grep -q 'SAE' "$config"
    else
        grep -q '^    key_mgmt=SAE$' "$config"
        grep -q '^    ieee80211w=2$' "$config"
        ! grep -q 'WPA-PSK' "$config"
    fi
    wpa_line=$(grep -n '^wpa_cli ' "$work_dir/commands.log" | head -1 | cut -d: -f1)
    dhcp_line=$(grep -n '^dhclient ' "$work_dir/commands.log" | head -1 | cut -d: -f1)
    (( wpa_line < dhcp_line ))
done

: >"$work_dir/commands.log"
if run_client "$work_dir/unauthorized" MOCK_AUTHORIZED=0 VCPE_ASSOCIATION_TIMEOUT_SECONDS=0 RADIO_WIFI_SECURITY=wpa3-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" >"$work_dir/unauthorized.out" 2>&1; then
    echo 'wireless client unexpectedly accepted incomplete authentication' >&2
    exit 1
fi
! grep -q '^dhclient ' "$work_dir/commands.log"

: >"$work_dir/commands.log"
second_radio=(RADIO_ZED_MODE=station RADIO_ZED_DEVICE=wlan1 RADIO_ZED_SSID=vcpe-lab
    RADIO_ZED_SECURITY=wpa2-personal RADIO_ZED_PASSPHRASE_FILE="$credential"
    RADIO_ZED_ADDRESSING=dhcp RADIO_ZED_DEFAULT_ROUTE=true)
if run_client "$work_dir/two-radios-failed" "${second_radio[@]}" MOCK_SECOND_AUTHORIZED=0 VCPE_ASSOCIATION_TIMEOUT_SECONDS=0 >"$work_dir/two-radios-failed.out" 2>&1; then
    echo 'wireless client unexpectedly accepted the unauthenticated second radio' >&2
    exit 1
fi
! grep -qx 'state=ready' "$work_dir/two-radios-failed/status"
! grep -q '^dhclient -v -1 wlan1$' "$work_dir/commands.log"

: >"$work_dir/commands.log"
run_client "$work_dir/two-radios" "${second_radio[@]}" >"$work_dir/two-radios.out"
[[ $(grep -c '^state=ready$' "$work_dir/two-radios/status") == 1 ]]
grep -qx 'device=wlan0' "$work_dir/two-radios/status"
grep -qx 'device=wlan1' "$work_dir/two-radios/status"
grep -qx 'ipv4=192.168.20.20/24' "$work_dir/two-radios/status"
grep -q '^dhclient -v -1 wlan1$' "$work_dir/commands.log"
second_auth_line=$(grep -n '^wpa_cli -i wlan1 status$' "$work_dir/commands.log" | head -1 | cut -d: -f1)
second_dhcp_line=$(grep -n '^dhclient -v -1 wlan1$' "$work_dir/commands.log" | head -1 | cut -d: -f1)
(( second_auth_line < second_dhcp_line ))
! grep -q '^route del default dev wlan1$' "$work_dir/commands.log"

if run_client "$work_dir/missing-credential" RADIO_WIFI_SECURITY=wpa2-personal RADIO_WIFI_PASSPHRASE_FILE="$work_dir/missing-passphrase" >"$work_dir/missing-credential.out" 2>&1; then
    echo 'wireless client unexpectedly accepted a missing credential' >&2
    exit 1
fi
grep -q 'protected credential file is missing' "$work_dir/missing-credential.out"

escaped_credential="$work_dir/escaped-passphrase"
printf '%s' 'pa"ss\\word#123' >"$escaped_credential"
chmod 600 "$escaped_credential"
run_client "$work_dir/escaped" RADIO_WIFI_SECURITY=wpa2-personal RADIO_WIFI_PASSPHRASE_FILE="$escaped_credential" >"$work_dir/escaped.out"
grep -Fq 'psk="pa\"ss\\\\word#123"' "$work_dir/escaped/wpa-WIFI.conf"
! grep -Fq 'pa"ss\\word#123' "$work_dir/escaped.out"

for band_channel_frequency in '2.4ghz 6 2437 open' '5ghz 36 5180 wpa2-personal' '6ghz 5 5975 wpa3-personal'; do
    read -r band channel frequency security <<<"$band_channel_frequency"
    : >"$work_dir/commands.log"
    run_client "$work_dir/media-$band" \
        RADIO_WIFI_MEDIUM="rf-$band" \
        RADIO_WIFI_BAND="$band" \
        RADIO_WIFI_CHANNEL="$channel" \
        RADIO_WIFI_AP_BSSID='02:00:00:00:00:10' \
        RADIO_WIFI_SECURITY="$security" \
        RADIO_WIFI_PASSPHRASE_FILE="$credential"
    config="$work_dir/media-$band/wpa-WIFI.conf"
    grep -qx "    bssid=02:00:00:00:00:10" "$config"
    grep -qx "    freq_list=$frequency" "$config"
    grep -qx "band=$band" "$work_dir/media-$band/status"
    grep -qx 'expected_ap_bssid=02:00:00:00:00:10' "$work_dir/media-$band/status"
    if [[ "$band" == 6ghz ]]; then
        grep -qx 'sae_pwe=1' "$config"
        grep -qx '    key_mgmt=SAE' "$config"
        grep -qx '    ieee80211w=2' "$config"
    fi
done

: >"$work_dir/commands.log"
run_client "$work_dir/roaming" \
    RADIO_WIFI_SECURITY=wpa3-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" \
    RADIO_WIFI_CANDIDATE_COUNT=2 \
    RADIO_WIFI_CANDIDATE_0_BSSID='02:00:00:00:00:10' RADIO_WIFI_CANDIDATE_0_BAND=5ghz RADIO_WIFI_CANDIDATE_0_CHANNEL=36 \
    RADIO_WIFI_CANDIDATE_1_BSSID='02:00:00:00:00:11' RADIO_WIFI_CANDIDATE_1_BAND=5ghz RADIO_WIFI_CANDIDATE_1_CHANNEL=40
config="$work_dir/roaming/wpa-WIFI.conf"
grep -qx '    freq_list=5180 5200' "$config"
grep -qx '    bgscan="simple:5:-65:10"' "$config"
grep -qx '    # eligible_bssid=02:00:00:00:00:10' "$config"
grep -qx '    # eligible_bssid=02:00:00:00:00:11' "$config"
! grep -q '^    bssid=' "$config"
! grep -q '^expected_ap_bssid=' "$work_dir/roaming/status"
grep -qx 'bssid=02:00:00:00:00:10' "$work_dir/roaming/status"

: >"$work_dir/commands.log"
run_client "$work_dir/roaming-second" MOCK_LINK_BSSID='02:00:00:00:00:11' \
    RADIO_WIFI_SECURITY=wpa3-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" \
    RADIO_WIFI_CANDIDATE_COUNT=2 \
    RADIO_WIFI_CANDIDATE_0_BSSID='02:00:00:00:00:10' RADIO_WIFI_CANDIDATE_0_BAND=5ghz RADIO_WIFI_CANDIDATE_0_CHANNEL=36 \
    RADIO_WIFI_CANDIDATE_1_BSSID='02:00:00:00:00:11' RADIO_WIFI_CANDIDATE_1_BAND=5ghz RADIO_WIFI_CANDIDATE_1_CHANNEL=40
grep -qx 'bssid=02:00:00:00:00:11' "$work_dir/roaming-second/status"
grep -qx 'ipv4=192.168.10.20/24' "$work_dir/roaming-second/status"

: >"$work_dir/commands.log"
run_client "$work_dir/roam-switch" MOCK_LINK_SEQUENCE_FILE="$work_dir/link-count" VCPE_WIRELESS_CLIENT_ONESHOT=0 \
    RADIO_WIFI_DEFAULT_ROUTE=true RADIO_WIFI_SECURITY=wpa3-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" \
    RADIO_WIFI_CANDIDATE_COUNT=2 \
    RADIO_WIFI_CANDIDATE_0_BSSID='02:00:00:00:00:10' RADIO_WIFI_CANDIDATE_0_BAND=5ghz RADIO_WIFI_CANDIDATE_0_CHANNEL=36 \
    RADIO_WIFI_CANDIDATE_1_BSSID='02:00:00:00:00:11' RADIO_WIFI_CANDIDATE_1_BAND=5ghz RADIO_WIFI_CANDIDATE_1_CHANNEL=36 >"$work_dir/roam-switch.out" 2>&1 &
roam_pid=$!
roamed=false
for ((attempt=0; attempt<40; attempt++)); do
    if [[ -f "$work_dir/roam-switch/status" ]] && grep -qx 'bssid=02:00:00:00:00:11' "$work_dir/roam-switch/status"; then
        roamed=true
        break
    fi
    sleep 0.1
done
kill "$roam_pid" 2>/dev/null || true
wait "$roam_pid" 2>/dev/null || true
[[ "$roamed" == true ]] || { echo 'roaming status did not follow an eligible AP' >&2; exit 1; }
grep -qx 'ipv4=192.168.10.20/24' "$work_dir/roam-switch/status"
[[ $(grep -c '^dhclient -v -1 wlan0$' "$work_dir/commands.log") == 1 ]]
! grep -q '^route del default dev wlan0$' "$work_dir/commands.log"

: >"$work_dir/commands.log"
if run_client "$work_dir/unlisted-ap" MOCK_LINK_BSSID='02:00:00:00:00:12' VCPE_ASSOCIATION_TIMEOUT_SECONDS=0 \
    RADIO_WIFI_SECURITY=wpa3-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" \
    RADIO_WIFI_CANDIDATE_COUNT=2 \
    RADIO_WIFI_CANDIDATE_0_BSSID='02:00:00:00:00:10' RADIO_WIFI_CANDIDATE_0_BAND=5ghz RADIO_WIFI_CANDIDATE_0_CHANNEL=36 \
    RADIO_WIFI_CANDIDATE_1_BSSID='02:00:00:00:00:11' RADIO_WIFI_CANDIDATE_1_BAND=5ghz RADIO_WIFI_CANDIDATE_1_CHANNEL=40 >"$work_dir/unlisted-ap.out" 2>&1; then
    echo 'wireless client accepted an unlisted roaming AP' >&2
    exit 1
fi
grep -q 'association timed out' "$work_dir/unlisted-ap.out"
! grep -q '^dhclient ' "$work_dir/commands.log"

if run_client "$work_dir/invalid-candidate" RADIO_WIFI_CANDIDATE_COUNT=2 RADIO_WIFI_CANDIDATE_0_BSSID=invalid RADIO_WIFI_CANDIDATE_0_BAND=5ghz RADIO_WIFI_CANDIDATE_0_CHANNEL=36 >"$work_dir/invalid-candidate.out" 2>&1; then
    echo 'wireless client accepted an invalid roaming candidate' >&2
    exit 1
fi
grep -q 'invalid roaming candidate' "$work_dir/invalid-candidate.out"

if run_client "$work_dir/downgrade" RADIO_WIFI_MEDIUM=rf6 RADIO_WIFI_BAND=6ghz RADIO_WIFI_CHANNEL=5 RADIO_WIFI_AP_BSSID='02:00:00:00:00:10' RADIO_WIFI_SECURITY=wpa2-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" >"$work_dir/downgrade.out" 2>&1; then
    echo 'wireless client unexpectedly accepted WPA2 on 6 GHz' >&2
    exit 1
fi
grep -q 'requires 6 GHz PSC and WPA3-Personal' "$work_dir/downgrade.out"

: >"$work_dir/commands.log"
if run_client "$work_dir/wrong-ap" VCPE_ASSOCIATION_TIMEOUT_SECONDS=0 RADIO_WIFI_MEDIUM=rf6 RADIO_WIFI_BAND=6ghz RADIO_WIFI_CHANNEL=5 RADIO_WIFI_AP_BSSID='02:00:00:00:00:11' RADIO_WIFI_SECURITY=wpa3-personal RADIO_WIFI_PASSPHRASE_FILE="$credential" >"$work_dir/wrong-ap.out" 2>&1; then
    echo 'wireless client unexpectedly accepted a different AP BSSID' >&2
    exit 1
fi
grep -q 'association timed out' "$work_dir/wrong-ap.out"
! grep -q '^dhclient ' "$work_dir/commands.log"

echo 'wireless client bootstrap: ok'