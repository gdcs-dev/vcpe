#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
umask 077

ready_file="${VCPE_NETWORK_READY_FILE:-/etc/vcpe/gateway-network-ready}"
config_dir="${VCPE_HOSTAPD_CONFIG_DIR:-/etc/vcpe/hostapd}"
runtime_dir="${VCPE_HOSTAPD_RUNTIME_DIR:-/run/vcpe/hostapd}"
control_dir="${VCPE_HOSTAPD_CTRL_DIR:-/run/vcpe/hostapd/control}"
hostapd_bin="${VCPE_HOSTAPD_BIN:-/usr/sbin/hostapd}"

if [[ ! -e "$ready_file" ]]; then
    echo "vcpe-hostapd: Gateway networking is not ready: missing $ready_file" >&2
    exit 1
fi

shopt -s nullglob
policies=("$config_dir"/*.conf)
if [[ ${#policies[@]} -eq 0 ]]; then
    echo "vcpe-hostapd: no hostapd configurations found in $config_dir" >&2
    exit 1
fi

mkdir -p "$runtime_dir"
chmod 700 "$runtime_dir"
configs=()
for policy in "${policies[@]}"; do
    config="$runtime_dir/$(basename "$policy")"
    temporary=$(mktemp "$runtime_dir/.hostapd.XXXXXX")
    if grep -q '^bssid=' "$policy"; then
        mkdir -p "$control_dir"
        chmod 700 "$control_dir"
        use_control=1
    else
        use_control=0
    fi
    security=''
    while IFS= read -r line || [[ -n "$line" ]]; do
        if [[ "$line" == '# vcpe-security='* ]]; then
            if [[ -n "$security" ]]; then
                rm -f "$temporary"
                echo "vcpe-hostapd: wireless security marker has no credential file" >&2
                exit 1
            fi
            security=${line#*=}
            continue
        fi
        if [[ "$line" == '# vcpe-passphrase-file='* ]]; then
            passphrase_file=${line#*=}
            if [[ "$security" != wpa2-personal && "$security" != wpa3-personal ]]; then
                rm -f "$temporary"
                echo "vcpe-hostapd: unsupported wireless security policy" >&2
                exit 1
            fi
            if [[ -z "$passphrase_file" || ! -f "$passphrase_file" || -L "$passphrase_file" ]]; then
                rm -f "$temporary"
                echo "vcpe-hostapd: protected credential file is missing or invalid" >&2
                exit 1
            fi
            credential_mode=$(stat -c '%a' "$passphrase_file" 2>/dev/null || stat -f '%Lp' "$passphrase_file")
            if [[ "$credential_mode" != 600 ]]; then
                rm -f "$temporary"
                echo "vcpe-hostapd: protected credential file must have mode 0600" >&2
                exit 1
            fi
            passphrase=$(<"$passphrase_file")
            if (( ${#passphrase} < 8 || ${#passphrase} > 63 )) || grep -q '[^ -~]' <<<"$passphrase"; then
                rm -f "$temporary"
                echo "vcpe-hostapd: protected credential is not 8 through 63 printable ASCII characters" >&2
                exit 1
            fi
            if [[ "$security" == wpa2-personal ]]; then
                printf '%s\n' 'wpa=2' 'wpa_key_mgmt=WPA-PSK' 'rsn_pairwise=CCMP' "wpa_passphrase=$passphrase" >>"$temporary"
            else
                printf '%s\n' 'wpa=2' 'wpa_key_mgmt=SAE' 'rsn_pairwise=CCMP' 'ieee80211w=2' "sae_password=$passphrase" >>"$temporary"
            fi
            unset passphrase
            security=''
            continue
        fi
        if [[ -n "$security" && "$line" == bss=* ]]; then
            rm -f "$temporary"
            echo "vcpe-hostapd: wireless security marker has no credential file" >&2
            exit 1
        fi
        printf '%s\n' "$line" >>"$temporary"
        if [[ "$use_control" == 1 && ( "$line" == interface=* || "$line" == bss=* ) ]]; then
            printf 'ctrl_interface=%s\n' "$control_dir" >>"$temporary"
        fi
    done <"$policy"
    if [[ -n "$security" ]]; then
        rm -f "$temporary"
        echo "vcpe-hostapd: wireless security marker has no credential file" >&2
        exit 1
    fi
    chmod 600 "$temporary"
    mv -f "$temporary" "$config"

    device=$(awk -F= '$1 == "interface" { print $2; exit }' "$config")
    bridge=$(awk -F= '$1 == "bridge" { print $2; exit }' "$config")
    bssid=$(awk -F= '$1 == "bssid" { print $2; exit }' "$config")
    if [[ -z "$device" ]]; then
        echo "vcpe-hostapd: $config does not declare an interface" >&2
        exit 1
    fi
    link=$(ip link show dev "$device" 2>/dev/null) || {
        echo "vcpe-hostapd: AP device $device from $config is missing" >&2
        exit 1
    }
    if [[ -n "$bridge" && -z "$bssid" && "$link" != *"master $bridge"* ]]; then
        echo "vcpe-hostapd: AP device $device is not attached to bridge $bridge" >&2
        exit 1
    fi
    configs+=("$config")
done

exec "$hostapd_bin" "${configs[@]}"