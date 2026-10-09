#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "wireless security image capabilities: $*" >&2
    exit 1
}

if [[ -z "${VCPE_GATEWAY_IMAGE:-}" && -z "${VCPE_WIRELESS_CLIENT_IMAGE:-}" ]]; then
    echo 'wireless security image capabilities: skipped (set VCPE_GATEWAY_IMAGE and/or VCPE_WIRELESS_CLIENT_IMAGE)'
    exit 0
fi

if [[ -n "${VCPE_GATEWAY_IMAGE:-}" ]]; then
    podman run --rm --entrypoint /bin/bash "$VCPE_GATEWAY_IMAGE" -ceu '
        check_hostapd() {
            local config=$1
            local label=$2
            local exit_code=0
            timeout --signal=TERM 2 hostapd "$config" >"/tmp/${label}.log" 2>&1 || exit_code=$?
            if [[ $exit_code -ne 0 && $exit_code -ne 124 ]]; then
                cat "/tmp/${label}.log" >&2
                return 1
            fi
            if grep -Eqi "unknown|unsupported|invalid|failed to set up interface" "/tmp/${label}.log"; then
                cat "/tmp/${label}.log" >&2
                return 1
            fi
        }

        printf "%s\n" \
            "interface=wlan0" \
            "driver=none" \
            "ssid=vcpe-capability" \
            "hw_mode=g" \
            "channel=1" \
            "wpa=2" \
            "wpa_key_mgmt=WPA-PSK" \
            "rsn_pairwise=CCMP" \
            "wpa_passphrase=capability-only" > /tmp/wpa2.conf
        check_hostapd /tmp/wpa2.conf wpa2

        sed "s/WPA-PSK/SAE/; /rsn_pairwise=CCMP/a ieee80211w=2" \
            /tmp/wpa2.conf > /tmp/wpa3.conf
        check_hostapd /tmp/wpa3.conf wpa3

        sed \
            -e "s/hw_mode=g/hw_mode=a/" \
            -e "s/channel=1/channel=5/" \
            -e "/channel=5/a op_class=131\nieee80211ax=1\nhe_oper_chwidth=0\nhe_6ghz_reg_pwr_type=0" \
            /tmp/wpa3.conf > /tmp/wpa3-6ghz.conf
        check_hostapd /tmp/wpa3-6ghz.conf wpa3-6ghz
    ' || fail "Gateway image $VCPE_GATEWAY_IMAGE cannot parse WPA2-PSK/CCMP or SAE/CCMP with required PMF; install an Ubuntu hostapd package built with SAE and IEEE 802.11w support"
fi

if [[ -n "${VCPE_WIRELESS_CLIENT_IMAGE:-}" ]]; then
    podman run --rm --cap-add NET_RAW --entrypoint /bin/bash "$VCPE_WIRELESS_CLIENT_IMAGE" -ceu '
        wpa_supplicant -v
        printf "%s\n" \
            "ctrl_interface=/run/wpa_supplicant" \
            "network={" \
            "  ssid=\"vcpe-capability\"" \
            "  key_mgmt=SAE" \
            "  ieee80211w=2" \
            "  psk=\"capability-only\"" \
            "}" > /tmp/sae.conf
        exit_code=0
        timeout --signal=TERM 2 wpa_supplicant -D none -i capability0 \
            -c /tmp/sae.conf -t > /tmp/wpa-supplicant.log 2>&1 || exit_code=$?
        if grep -Eqi "unknown|unsupported|invalid" /tmp/wpa-supplicant.log; then
            cat /tmp/wpa-supplicant.log >&2
            exit 1
        fi
        grep -q "Successfully initialized wpa_supplicant" /tmp/wpa-supplicant.log
    ' || fail "wireless-client image $VCPE_WIRELESS_CLIENT_IMAGE cannot parse SAE with required PMF; install an Ubuntu wpasupplicant package built with SAE and IEEE 802.11w support"
fi

echo 'wireless security image capabilities: ok'
