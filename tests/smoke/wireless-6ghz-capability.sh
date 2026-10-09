#!/usr/bin/env bash
set -euo pipefail

manager=/usr/libexec/vcpe/vcpe-hwsim
image=localhost/vcpe-gateway:tri-band-capability
client_image=ghcr.io/gdcs-dev/wireless-client:dev
namespace=vcpe-cap6-ap
unit=vcpe-cap6-ap.service
container=vcpe-cap6-hostapd
station_namespace=vcpe-cap6-sta
station_unit=vcpe-cap6-sta.service
client_container=vcpe-cap6-client
dhcp_unit=vcpe-cap6-dhcp.service
config=$(mktemp /tmp/vcpe-cap6-hostapd.XXXXXX)
station_config=$(mktemp /tmp/vcpe-cap6-station.XXXXXX)
station_log=$(mktemp /tmp/vcpe-cap6-supplicant.XXXXXX)
negative_config=$(mktemp /tmp/vcpe-cap6-downgrade.XXXXXX)
cleanup_armed=false
chmod 600 "$config"
chmod 600 "$station_config"
chmod 600 "$station_log"
chmod 600 "$negative_config"

cleanup() {
    if [[ "$cleanup_armed" == true ]]; then
        podman rm -f "$client_container" >/dev/null 2>&1 || true
        podman rm -f "$container" >/dev/null 2>&1 || true
        systemctl stop "$dhcp_unit" >/dev/null 2>&1 || true
        "$manager" release --name vcpe-cap6-sta >/dev/null 2>&1 || true
        "$manager" release --name vcpe-cap6-ap >/dev/null 2>&1 || true
        systemctl stop "$station_unit" >/dev/null 2>&1 || true
        systemctl stop "$unit" >/dev/null 2>&1 || true
        ip netns delete "$station_namespace" >/dev/null 2>&1 || true
        ip netns delete "$namespace" >/dev/null 2>&1 || true
    fi
    rm -f "$config" "$station_config" "$station_log" "$negative_config"
}
trap cleanup EXIT

test "$("$manager" list | jq '.data | length')" -eq 0
podman image exists "$image"
podman image exists "$client_image"
! podman container exists "$container"
! podman container exists "$client_container"
! ip netns list | grep -Eq "^($namespace|$station_namespace)( |$)"
! systemctl is-active --quiet "$unit" "$station_unit" "$dhcp_unit"
cleanup_armed=true
iw reg set US
ip netns add "$namespace"
systemd-run --quiet --unit "$unit" --property "NetworkNamespacePath=/run/netns/$namespace" --service-type=exec sleep infinity
pid=$(systemctl show --property MainPID --value "$unit")
"$manager" ensure --name vcpe-cap6-ap --mac 02:76:63:70:65:10 --group-mask 0x4000000000000000 --netns-pid "$pid" --ifname wlan0 --type ap | jq -e '.success'

printf '%s\n' \
    interface=wlan0 driver=nl80211 country_code=US country3=0x49 ieee80211d=1 \
    hw_mode=a channel=5 op_class=131 ieee80211ax=1 he_oper_chwidth=0 \
    he_6ghz_reg_pwr_type=0 > "$config"

for slot in {0..7}; do
    ip -n "$namespace" link add "brvap$slot" type bridge
    ip -n "$namespace" link set "brvap$slot" up
    printf -v bssid '02:76:63:70:65:%02x' "$((16 + slot))"
    if (( slot > 0 )); then
        printf 'bss=wlan0v%d\n' "$slot" >> "$config"
    fi
    printf '%s\n' \
        "bssid=$bssid" "ssid=vcpe-cap6-$slot" "bridge=brvap$slot" \
        wpa=2 wpa_key_mgmt=SAE rsn_pairwise=CCMP ieee80211w=2 \
        sae_pwe=1 wpa_passphrase=capability-only >> "$config"
done

podman run -d --pull=never --name "$container" --privileged \
    --network "ns:/run/netns/$namespace" -v "$config:$config:ro" \
    --entrypoint /usr/sbin/hostapd "$image" -d "$config" >/dev/null

set +o pipefail
if timeout 45 podman logs --follow "$container" 2>&1 | awk '/wlan0: AP-ENABLED/ { enabled=1; exit 0 } END { if (!enabled) exit 1 }'; then
    gate_code=0
else
    gate_code=$?
fi
set -o pipefail
if (( gate_code != 0 )); then
    podman logs "$container" 2>&1 | grep -Ei 'fail|error|invalid|No buffer|AP-ENABLED|BSS count|wlan0v[1-7]:' | tail -80 || true
    exit 1
fi

for slot in {0..7}; do
    device=wlan0
    if (( slot > 0 )); then
        device="wlan0v$slot"
    fi
    ip netns exec "$namespace" iw dev "$device" info | grep -F 'channel 5 (5975 MHz)' >/dev/null
    printf -v bssid '02:76:63:70:65:%02x' "$((16 + slot))"
    ip netns exec "$namespace" iw dev "$device" info | grep -Fi "$bssid" >/dev/null
    ip -n "$namespace" -o link show master "brvap$slot" | grep -F "$device" >/dev/null
done
test "$("$manager" list | jq '.data | length')" -eq 1
regulatory=$(ip netns exec "$namespace" iw reg get)
grep -F 'country US:' <<< "$regulatory" >/dev/null
grep -F '(5925 - 7125 @ 320), (N/A, 12), (N/A), NO-OUTDOOR, PASSIVE-SCAN' <<< "$regulatory" >/dev/null
grep -F '(self-managed)' <<< "$regulatory" >/dev/null
grep -F 'country 99:' <<< "$regulatory" >/dev/null
echo '6 GHz eight-BSS AP startup: ok'

ip netns add "$station_namespace"
systemd-run --quiet --unit "$station_unit" --property "NetworkNamespacePath=/run/netns/$station_namespace" --service-type=exec sleep infinity
station_pid=$(systemctl show --property MainPID --value "$station_unit")
"$manager" ensure --name vcpe-cap6-sta --mac 02:76:63:70:65:20 --group-mask 0x4000000000000000 --netns-pid "$station_pid" --ifname wlan0 --type station | jq -e '.success'
podman run -d --pull=never --name "$client_container" --privileged \
    --network "ns:/run/netns/$station_namespace" \
    -v "$station_config:$station_config:ro" -v "$negative_config:$negative_config:ro" \
    -v "$station_log:$station_log" \
    --entrypoint sleep "$client_image" infinity >/dev/null
podman exec "$client_container" ip link set wlan0 up
scan=$(podman exec "$client_container" iw dev wlan0 scan freq 5975)
for slot in {0..7}; do
    printf -v bssid '02:76:63:70:65:%02x' "$((16 + slot))"
    grep -Fi "BSS $bssid" <<< "$scan" >/dev/null
    grep -F "SSID: vcpe-cap6-$slot" <<< "$scan" >/dev/null
done
echo '6 GHz eight-BSS beacon scan: ok'

ip -n "$namespace" addr add 10.250.0.1/24 dev brvap0
systemd-run --quiet --unit "$dhcp_unit" --property "NetworkNamespacePath=/run/netns/$namespace" --service-type=exec \
    /usr/sbin/dnsmasq --no-daemon --port=0 --interface=brvap0 --bind-interfaces \
    --dhcp-range=10.250.0.100,10.250.0.120,255.255.255.0,1h
printf '%s\n' \
    ctrl_interface=/run/wpa_supplicant update_config=0 sae_pwe=1 'network={' \
    '    ssid="vcpe-cap6-0"' '    scan_ssid=1' \
    '    bssid=02:76:63:70:65:10' '    freq_list=5975' \
    '    key_mgmt=SAE' '    proto=RSN' '    pairwise=CCMP' \
    '    group=CCMP' '    ieee80211w=2' \
    '    sae_password="capability-only"' '}' > "$station_config"
podman exec "$client_container" wpa_supplicant -B -D nl80211 -i wlan0 -c "$station_config" -f "$station_log" -d >/dev/null

connected=false
deadline=$((SECONDS + 45))
while (( SECONDS < deadline )); do
    station_status=$(podman exec "$client_container" wpa_cli -i wlan0 status 2>/dev/null || true)
    if [[ "$station_status" == *'wpa_state=COMPLETED'* ]]; then
        connected=true
        break
    fi
done
if [[ "$connected" != true ]]; then
    printf 'Station failed to authenticate: %s\n' "$station_status" >&2
    podman exec "$client_container" wpa_cli -i wlan0 scan_results | grep -F 'vcpe-cap6-' | head -8 || true
    podman exec "$client_container" iw dev wlan0 link || true
    grep -Ei 'skip|reject|no suitable|SAE-H2E|PMF|6 GHz|authenticat|associat' "$station_log" | tail -25 || true
    exit 1
fi
grep -F 'bssid=02:76:63:70:65:10' <<< "$station_status"
grep -F 'key_mgmt=SAE' <<< "$station_status"
grep -F 'pairwise_cipher=CCMP' <<< "$station_status"
station_details=$(ip netns exec "$namespace" iw dev wlan0 station get 02:76:63:70:65:20)
grep -Eq 'authorized:[[:space:]]+yes' <<< "$station_details"
grep -Eq 'MFP:[[:space:]]+yes' <<< "$station_details"
podman exec "$client_container" iw dev wlan0 link | grep -F 'freq: 5975'
podman exec "$client_container" dhclient -1 -v wlan0 >/dev/null
podman exec "$client_container" ip -4 addr show wlan0 | grep -F '10.250.0.'
podman exec "$client_container" ping -c 3 -W 2 10.250.0.1 | grep -F '0% packet loss'
test "$("$manager" list | jq '.data | length')" -eq 2
echo '6 GHz SAE association, DHCP, and traffic: ok'

podman exec "$client_container" wpa_cli -i wlan0 terminate >/dev/null
podman exec "$client_container" ip -4 addr flush dev wlan0
printf '%s\n' \
    ctrl_interface=/run/wpa_supplicant update_config=0 'network={' \
    '    ssid="vcpe-cap6-0"' '    scan_ssid=1' \
    '    bssid=02:76:63:70:65:10' '    freq_list=5975' \
    '    key_mgmt=WPA-PSK' '    proto=RSN' '    pairwise=CCMP' \
    '    group=CCMP' '    ieee80211w=2' \
    '    psk="capability-only"' '}' > "$negative_config"
if podman exec "$client_container" wpa_supplicant -B -D nl80211 -i wlan0 -c "$negative_config" -f "$station_log" >/dev/null; then
    deadline=$((SECONDS + 15))
    while (( SECONDS < deadline )); do
        station_status=$(podman exec "$client_container" wpa_cli -i wlan0 status 2>/dev/null || true)
        if [[ "$station_status" == *'wpa_state=COMPLETED'* ]]; then
            echo 'WPA2-only downgrade unexpectedly authenticated' >&2
            exit 1
        fi
    done
fi
test -z "$(podman exec "$client_container" ip -4 -o addr show dev wlan0)"
echo '6 GHz WPA2-only downgrade rejected: ok'