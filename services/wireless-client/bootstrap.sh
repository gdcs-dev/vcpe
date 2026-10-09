#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
umask 077

runtime_dir="${VCPE_RUNTIME_DIR:-/run/vcpe/wireless-client}"
association_timeout="${VCPE_ASSOCIATION_TIMEOUT_SECONDS:-30}"
association_poll="${VCPE_ASSOCIATION_POLL_SECONDS:-1}"
mkdir -p "$runtime_dir"
: >"$runtime_dir/status"

radio_keys=()
roaming_keys=()
declare -A roaming_devices roaming_bssids roaming_ipv4 roaming_security roaming_ssids roaming_last_bssid
while IFS= read -r key; do
    radio_keys+=("$key")
done < <(env | sed -n 's/^RADIO_\([A-Z0-9_]*\)_MODE=station$/\1/p' | sort)

if [[ ${#radio_keys[@]} -eq 0 ]]; then
    echo 'wireless-client: no station radios are declared' >&2
    exit 1
fi

channel_frequency() {
    local band=$1 channel=$2 security=$3 radio=$4
    if [[ ! "$channel" =~ ^[0-9]+$ ]]; then
        echo "wireless-client: invalid channel for radio $radio" >&2
        return 1
    fi
    case "$band" in
        2.4ghz)
            if ((channel < 1 || channel > 14)); then
                echo "wireless-client: unsupported 2.4 GHz channel for radio $radio" >&2
                return 1
            fi
            if ((channel == 14)); then printf '%s\n' 2484; else printf '%s\n' "$((2407 + 5 * channel))"; fi
            ;;
        5ghz) printf '%s\n' "$((5000 + 5 * channel))" ;;
        6ghz)
            if [[ "$security" != wpa3-personal ]] || ((channel < 5 || channel > 229 || (channel - 5) % 16 != 0)); then
                echo "wireless-client: radio $radio requires 6 GHz PSC and WPA3-Personal" >&2
                return 1
            fi
            printf '%s\n' "$((5950 + 5 * channel))"
            ;;
        *) echo "wireless-client: unsupported band for radio $radio" >&2; return 1 ;;
    esac
}

for key in "${radio_keys[@]}"; do
    device_var="RADIO_${key}_DEVICE"
    ssid_var="RADIO_${key}_SSID"
    addressing_var="RADIO_${key}_ADDRESSING"
    default_route_var="RADIO_${key}_DEFAULT_ROUTE"
    security_var="RADIO_${key}_SECURITY"
    passphrase_file_var="RADIO_${key}_PASSPHRASE_FILE"
    medium_var="RADIO_${key}_MEDIUM"
    band_var="RADIO_${key}_BAND"
    channel_var="RADIO_${key}_CHANNEL"
    ap_bssid_var="RADIO_${key}_AP_BSSID"
    candidate_count_var="RADIO_${key}_CANDIDATE_COUNT"
    device="${!device_var:-}"
    ssid="${!ssid_var:-}"
    addressing="${!addressing_var:-dhcp}"
    default_route="${!default_route_var:-false}"
    security="${!security_var:-open}"
    passphrase_file="${!passphrase_file_var:-}"
    medium="${!medium_var:-}"
    band="${!band_var:-}"
    channel="${!channel_var:-}"
    ap_bssid="${!ap_bssid_var:-}"
    candidate_count="${!candidate_count_var:-}"

    if [[ -z "$device" || -z "$ssid" ]]; then
        echo "wireless-client: radio $key is missing device or SSID" >&2
        exit 1
    fi
    if ! ip link show dev "$device" >/dev/null 2>&1; then
        echo "wireless-client: radio device $device is missing" >&2
        exit 1
    fi

    escaped_ssid=${ssid//\\/\\\\}
    escaped_ssid=${escaped_ssid//\"/\\\"}
    config="$runtime_dir/wpa-${key}.conf"
    frequency=''
    frequencies=()
    eligible_bssids=()
    needs_sae_pwe=false
    if [[ -n "$candidate_count" ]]; then
        if [[ ! "$candidate_count" =~ ^[0-9]+$ ]] || ((candidate_count < 2 || candidate_count > 64)); then
            echo "wireless-client: invalid roaming candidate count for radio $key" >&2
            exit 1
        fi
        for ((candidate_index=0; candidate_index<candidate_count; candidate_index++)); do
            candidate_prefix="RADIO_${key}_CANDIDATE_${candidate_index}_"
            bssid_var="${candidate_prefix}BSSID"
            band_var="${candidate_prefix}BAND"
            channel_var="${candidate_prefix}CHANNEL"
            candidate_bssid="${!bssid_var:-}"
            candidate_band="${!band_var:-}"
            candidate_channel="${!channel_var:-}"
            if [[ ! "$candidate_bssid" =~ ^([[:xdigit:]]{2}:){5}[[:xdigit:]]{2}$ ]]; then
                echo "wireless-client: invalid roaming candidate BSSID for radio $key" >&2
                exit 1
            fi
            candidate_frequency=$(channel_frequency "$candidate_band" "$candidate_channel" "$security" "$key") || exit 1
            frequencies+=("$candidate_frequency")
            eligible_bssids+=("${candidate_bssid,,}")
            [[ "$candidate_band" == 6ghz ]] && needs_sae_pwe=true
        done
    elif [[ -n "$medium" ]]; then
        if [[ ! "$channel" =~ ^[0-9]+$ || ! "$ap_bssid" =~ ^([[:xdigit:]]{2}:){5}[[:xdigit:]]{2}$ ]]; then
            echo "wireless-client: radio $key requires a valid channel and expected AP BSSID" >&2
            exit 1
        fi
        frequency=$(channel_frequency "$band" "$channel" "$security" "$key") || exit 1
        [[ "$band" == 6ghz ]] && needs_sae_pwe=true
    fi
    if [[ "$needs_sae_pwe" == true ]]; then
        printf '%s\n' 'sae_pwe=1' >"$config"
    else
        : >"$config"
    fi
    cat >>"$config" <<EOF
ctrl_interface=/run/wpa_supplicant
update_config=0
network={
    ssid="$escaped_ssid"
    scan_ssid=1
EOF
    if [[ -n "$medium" ]]; then
        printf '    bssid=%s\n    freq_list=%s\n' "$ap_bssid" "$frequency" >>"$config"
    elif [[ -n "$candidate_count" ]]; then
        unique_frequencies=()
        for candidate_frequency in "${frequencies[@]}"; do
            if [[ " ${unique_frequencies[*]} " != *" $candidate_frequency "* ]]; then
                unique_frequencies+=("$candidate_frequency")
            fi
        done
        printf '    freq_list=%s\n    bgscan="simple:5:-65:10"\n' "${unique_frequencies[*]}" >>"$config"
        for candidate_bssid in "${eligible_bssids[@]}"; do
            printf '    # eligible_bssid=%s\n' "$candidate_bssid" >>"$config"
        done
    fi
    case "$security" in
        open)
            echo '    key_mgmt=NONE' >>"$config"
            ;;
        wpa2-personal|wpa3-personal)
            if [[ -z "$passphrase_file" || ! -f "$passphrase_file" || -L "$passphrase_file" ]]; then
                echo "wireless-client: protected credential file is missing or invalid for radio $key" >&2
                exit 1
            fi
            credential_mode=$(stat -c '%a' "$passphrase_file" 2>/dev/null || stat -f '%Lp' "$passphrase_file")
            if [[ "$credential_mode" != 600 ]]; then
                echo "wireless-client: protected credential file must have mode 0600 for radio $key" >&2
                exit 1
            fi
            passphrase=$(<"$passphrase_file")
            if (( ${#passphrase} < 8 || ${#passphrase} > 63 )) || grep -q '[^ -~]' <<<"$passphrase"; then
                echo "wireless-client: protected credential is invalid for radio $key" >&2
                exit 1
            fi
            escaped_passphrase=${passphrase//\\/\\\\}
            escaped_passphrase=${escaped_passphrase//\"/\\\"}
            if [[ "$security" == wpa2-personal ]]; then
                printf '%s\n' \
                    '    key_mgmt=WPA-PSK' \
                    '    proto=RSN' \
                    '    pairwise=CCMP' \
                    '    group=CCMP' \
                    "    psk=\"$escaped_passphrase\"" >>"$config"
            else
                printf '%s\n' \
                    '    key_mgmt=SAE' \
                    '    proto=RSN' \
                    '    pairwise=CCMP' \
                    '    group=CCMP' \
                    '    ieee80211w=2' \
                    "    sae_password=\"$escaped_passphrase\"" >>"$config"
            fi
            unset passphrase escaped_passphrase
            ;;
        *)
            echo "wireless-client: unsupported security policy for radio $key" >&2
            exit 1
            ;;
    esac
    echo '}' >>"$config"
    chmod 600 "$config"
    if ! wpa_supplicant -B -D nl80211 -i "$device" -c "$config" >/dev/null 2>&1; then
        echo "wireless-client: supplicant rejected security policy for radio $key" >&2
        exit 1
    fi
    deadline=$((SECONDS + association_timeout))
    while true; do
        link=$(iw dev "$device" link 2>/dev/null || true)
        matched_ap=true
        link_bssid=$(awk '/^Connected to / { print tolower($3); exit }' <<<"$link")
        if [[ -n "$candidate_count" ]]; then
            matched_ap=false
            for candidate_bssid in "${eligible_bssids[@]}"; do
                [[ "$link_bssid" == "$candidate_bssid" ]] && matched_ap=true
            done
        elif [[ -n "$medium" && "$link_bssid" != "${ap_bssid,,}" ]]; then
            matched_ap=false
        fi
        if [[ "$security" == open ]]; then
            [[ "$matched_ap" == true && "$link" == *"Connected to "* && "$link" == *"SSID: $ssid"* ]] && break
        else
            supplicant_status=$(wpa_cli -i "$device" status 2>/dev/null || true)
            [[ "$matched_ap" == true && "$supplicant_status" == *"wpa_state=COMPLETED"* && "$supplicant_status" == *"ssid=$ssid"* ]] && break
        fi
        if ((SECONDS >= deadline)); then
            echo "wireless-client: association timed out for $device on SSID $ssid" >&2
            exit 1
        fi
        sleep "$association_poll"
    done

    if [[ "$addressing" == "dhcp" ]]; then
        dhclient -v -1 "$device"
    fi
    case "$default_route" in
        true|1) ;;
        *) ip route del default dev "$device" 2>/dev/null || true ;;
    esac

    bssid=$link_bssid
    ipv4=$(ip -4 -o addr show dev "$device" | awk '{ print $4; exit }')
    if [[ -n "$candidate_count" ]]; then
        roaming_keys+=("$key")
        roaming_devices[$key]=$device
        roaming_bssids[$key]="${eligible_bssids[*]}"
        roaming_ipv4[$key]=$ipv4
        roaming_security[$key]=$security
        roaming_ssids[$key]=$ssid
        roaming_last_bssid[$key]=$bssid
    fi
    {
        echo "radio=$key"
        echo "device=$device"
        echo "ssid=$ssid"
        echo "bssid=$bssid"
        if [[ -n "$medium" ]]; then
            echo "medium=$medium"
            echo "band=$band"
            echo "channel=$channel"
            echo "expected_ap_bssid=$ap_bssid"
        elif [[ -n "$candidate_count" ]]; then
            echo "roaming=1"
        fi
        echo "ipv4=$ipv4"
        echo "default_route=$default_route"
    } >>"$runtime_dir/status"
done

echo 'state=ready' >>"$runtime_dir/status"
echo "wireless-client: ready ($(tr '\n' ' ' <"$runtime_dir/status"))"
if [[ "${VCPE_WIRELESS_CLIENT_ONESHOT:-0}" != 1 ]]; then
    if [[ ${#roaming_keys[@]} -gt 0 ]]; then
        while true; do
            sleep "${VCPE_ROAM_POLL_SECONDS:-1}"
            for key in "${roaming_keys[@]}"; do
                device=${roaming_devices[$key]}
                link=$(iw dev "$device" link 2>/dev/null || true)
                current=$(awk '/^Connected to / { print tolower($3); exit }' <<<"$link")
                eligible=false
                for candidate in ${roaming_bssids[$key]}; do
                    [[ "$current" == "$candidate" ]] && eligible=true
                done
                [[ "$eligible" == true ]] || continue
                [[ "$current" == "${roaming_last_bssid[$key]}" ]] && continue
                if [[ "${roaming_security[$key]}" == open ]]; then
                    [[ "$link" == *"SSID: ${roaming_ssids[$key]}"* ]] || continue
                else
                    supplicant_status=$(wpa_cli -i "$device" status 2>/dev/null || true)
                    [[ "$supplicant_status" == *"wpa_state=COMPLETED"* && "$supplicant_status" == *"ssid=${roaming_ssids[$key]}"* ]] || continue
                fi
                ipv4=$(ip -4 -o addr show dev "$device" | awk '{ print $4; exit }')
                if [[ "$ipv4" != "${roaming_ipv4[$key]}" ]]; then
                    echo "wireless-client: IPv4 changed during roam on $device" >&2
                    exit 1
                fi
                awk -v radio="$key" -v bssid="$current" '
                    /^radio=/ { selected = ($0 == "radio=" radio) }
                    selected && /^bssid=/ { $0 = "bssid=" bssid }
                    { print }
                ' "$runtime_dir/status" >"$runtime_dir/status.next"
                mv -f "$runtime_dir/status.next" "$runtime_dir/status"
                roaming_last_bssid[$key]=$current
            done
        done
    fi
    exec tail -f /dev/null
fi