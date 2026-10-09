#!/usr/bin/env bash
set -euo pipefail
trap 'printf "Gateway smoke failed at line %s\n" "$LINENO" >&2' ERR

manager=/usr/libexec/vcpe/vcpe-hwsim
image=localhost/vcpe-gateway:tri-band-capability
client_image=ghcr.io/gdcs-dev/wireless-client:dev
namespace=vcpe-vap-probe
unit=vcpe-vap-probe.service
container=vcpe-vap-gateway
station_namespace=vcpe-vap-station
station_unit=vcpe-vap-station.service
client_container=vcpe-vap-client
work_dir=$(mktemp -d /tmp/vcpe-vap-probe.XXXXXX)
original_country=$(iw reg get | awk '/^country / {sub(/:$/, "", $2); print $2; exit}')
cleanup_armed=false

cleanup() {
    if [[ "$cleanup_armed" == true ]]; then
        podman rm -f -t 0 "$client_container" >/dev/null 2>&1 || true
        podman rm -f "$container" >/dev/null 2>&1 || true
        for index in 0 1 2 3 4 5 6; do
            systemctl stop "vcpe-vap-dhcp-$index.service" >/dev/null 2>&1 || true
        done
        "$manager" release --name vcpe-vap-station >/dev/null 2>&1 || true
        for radio in 0 1 2; do
            "$manager" release --name "vcpe-vap-probe-$radio" >/dev/null 2>&1 || true
        done
        systemctl stop "$station_unit" >/dev/null 2>&1 || true
        systemctl stop "$unit" >/dev/null 2>&1 || true
        ip netns delete "$station_namespace" >/dev/null 2>&1 || true
        ip netns delete "$namespace" >/dev/null 2>&1 || true
        if [[ -n "$original_country" ]]; then
            iw reg set "$original_country"
        fi
    fi
    rm -rf "$work_dir"
}
trap cleanup EXIT

test "$("$manager" list | jq '.data | length')" -eq 0
podman image exists "$image"
podman image exists "$client_image"
! podman container exists "$container"
! podman container exists "$client_container"
! ip netns list | grep -Eq "^$namespace( |$)"
! ip netns list | grep -Eq "^$station_namespace( |$)"
! systemctl is-active --quiet "$unit" "$station_unit" vcpe-vap-dhcp-{0..4}.service
cleanup_armed=true
iw reg set US
ip netns add "$namespace"
systemd-run --quiet --unit "$unit" --property "NetworkNamespacePath=/run/netns/$namespace" --service-type=exec sleep infinity
pid=$(systemctl show --property MainPID --value "$unit")
for radio in 0 1 2; do
    mac=$(printf '02:76:63:70:%02x:10' "$((radio + 36))")
    "$manager" ensure --name "vcpe-vap-probe-$radio" --mac "$mac" --group-mask 0x4000000000000000 --netns-pid "$pid" --ifname "wlan$radio" --type ap | jq -e '.success' >/dev/null
    case "$radio" in
        0) name=ap24; policy=(hw_mode=g channel=1) ;;
        1) name=ap5; policy=(hw_mode=a channel=36) ;;
        2) name=ap6; policy=(hw_mode=a channel=5 op_class=131 ieee80211ax=1 he_oper_chwidth=0 he_6ghz_reg_pwr_type=0) ;;
    esac
    printf '%s\n' "interface=wlan$radio" driver=nl80211 country_code=US country3=0x49 ieee80211d=1 "${policy[@]}" >"$work_dir/ap$radio.conf"
    for slot in {0..7}; do
        device="wlan${radio}v${slot}"
        if (( slot == 0 )); then
            device="wlan$radio"
        fi
        bssid=$(printf '02:76:63:70:%02x:%02x' "$((radio + 36))" "$((slot + 16))")
        bridge="br${radio}v${slot}"
        ip -n "$namespace" link add "$bridge" type bridge
        ip -n "$namespace" link set "$bridge" up
        if (( slot > 0 )); then
            printf 'bss=%s\n' "$device" >>"$work_dir/ap$radio.conf"
        fi
        ssid="vcpe-$name-$slot"
        if (( slot == 0 )); then
            ssid=vcpe-mirrored
        fi
        printf 'bssid=%s\nssid=%s\nbridge=%s\nauth_algs=1\n' "$bssid" "$ssid" "$bridge" >>"$work_dir/ap$radio.conf"
        security=wpa3-personal
        if (( radio < 2 && slot == 1 )); then
            security=wpa2-personal
        elif (( radio < 2 && slot == 2 )); then
            security=open
        fi
        if [[ "$security" != open ]]; then
            printf '# vcpe-security=%s\n# vcpe-passphrase-file=/etc/vcpe/credentials/home\n' "$security" >>"$work_dir/ap$radio.conf"
        fi
        jq -nc --arg radio "$name" --argjson slot "$slot" --arg interface "$device" --arg bssid "$bssid" --arg bridge "$bridge" \
            '{radio:$radio,slot:$slot,interface:$interface,bssid:$bssid,bridge:$bridge}' >>"$work_dir/expected.jsonl"
    done
done
ip netns exec "$namespace" sysctl -q -w net.ipv4.conf.all.arp_ignore=1
printf '%s' 'capability-only' >"$work_dir/home"
chmod 600 "$work_dir/home"
printf '%s' 'short' >"$work_dir/invalid"
chmod 600 "$work_dir/invalid"
printf '%s' 'incorrect-station-passphrase' >"$work_dir/wrong-station"
chmod 600 "$work_dir/wrong-station"
jq -s '{radios: (group_by(.radio) | map({name: .[0].radio, mode:"ap", vaps: map({slot, device:.interface, bssid, bridge})}))}' \
        "$work_dir/expected.jsonl" >"$work_dir/contract.json"
touch "$work_dir/ready"

for credential in missing invalid; do
    credential_mount=()
    if [[ "$credential" == invalid ]]; then
        credential_mount=(-v "$work_dir/invalid:/etc/vcpe/credentials/home:ro")
    fi
    if podman run --rm --pull=never --privileged \
        -v "$work_dir/ap0.conf:/etc/vcpe/hostapd/ap.conf:ro" \
        -v "$work_dir/ready:/etc/vcpe/gateway-network-ready:ro" \
        "${credential_mount[@]}" \
        --entrypoint /usr/local/bin/vcpe-hostapd "$image" >"$work_dir/rejected.log" 2>&1; then
        echo "Gateway accepted $credential wireless credential" >&2
        exit 1
    fi
    if ! grep -q 'vcpe-hostapd: protected credential' "$work_dir/rejected.log"; then
        echo "Gateway rejected $credential credential for an unexpected reason" >&2
        head -5 "$work_dir/rejected.log" >&2
        exit 1
    fi
done

podman run -d --pull=never --name "$container" --privileged \
    --network "ns:/run/netns/$namespace" \
    -v "$work_dir/ap0.conf:/etc/vcpe/hostapd/ap0.conf:ro" \
    -v "$work_dir/ap1.conf:/etc/vcpe/hostapd/ap1.conf:ro" \
    -v "$work_dir/ap2.conf:/etc/vcpe/hostapd/ap2.conf:ro" \
    -v "$work_dir/home:/etc/vcpe/credentials/home:ro" \
    -v "$work_dir/contract.json:/run/vcpe/startup-contract.json:ro" \
    -v "$work_dir/ready:/etc/vcpe/gateway-network-ready:ro" \
    --entrypoint /usr/local/bin/vcpe-hostapd "$image" >/dev/null

set +o pipefail
if ! timeout 45 podman logs --follow "$container" 2>&1 | awk '/wlan2: AP-ENABLED/ { enabled=1; exit 0 } END { if (!enabled) exit 1 }'; then
    podman logs "$container" 2>&1 | grep -Ei 'fail|error|invalid|unknown|AP-ENABLED|vcpe-hostapd|ctrl_interface' | tail -35 || true
    exit 1
fi
set -o pipefail
assert_vaps() {
    for attempt in {1..45}; do
        if observations=$(podman exec "$container" gateway-health-probe vaps) &&
            jq -e -s --slurpfile expected "$work_dir/expected.jsonl" '
                length == 24 and all(.[]; .enabled) and
                ([.[] | del(.enabled)] | sort_by(.radio, .slot)) == ($expected | sort_by(.radio, .slot)) and
                ([.[].bssid] | unique | length) == 24
            ' >/dev/null <<<"$observations"; then
            return 0
        fi
        sleep 1
    done
    printf 'Gateway VAP probe did not become ready:\n%s\n' "$observations" >&2
    jq -s --slurpfile expected "$work_dir/expected.jsonl" '
        [.[] | del(.enabled)] as $actual |
        {count: length, expectedCount: ($expected | length), enabled: all(.[]; .enabled),
         matching: ($actual | sort_by(.radio, .slot)) == ($expected | sort_by(.radio, .slot)),
         uniqueBSSIDs: ([.[].bssid] | unique | length),
         missing: ($expected - $actual), unexpected: ($actual - $expected)}
    ' <<<"$observations" >&2
    return 1
}
assert_vaps
for policy in 'wlan0v1 WPA-PSK' 'wlan0v2 NONE' 'wlan1v1 WPA-PSK' 'wlan2v1 SAE'; do
    read -r device key_mgmt <<<"$policy"
    config=$(podman exec "$container" hostapd_cli -p /run/vcpe/hostapd/control -i "$device" get_config)
    if [[ "$key_mgmt" == NONE ]]; then
        valid=$(printf '%s\n' "$config" | grep -Ec '^(key_mgmt|wpa)=' || true)
        [[ "$valid" == 0 ]] || { echo "Gateway $device unexpectedly enabled security" >&2; exit 1; }
    elif ! printf '%s\n' "$config" | grep -Fxq "key_mgmt=$key_mgmt"; then
        echo "Gateway $device did not advertise expected $key_mgmt policy" >&2
        exit 1
    fi
done
ip -n "$namespace" link set wlan2v7 nomaster
if observations=$(podman exec "$container" gateway-health-probe vaps); then
    echo 'Gateway reported readiness despite a detached secondary BSS' >&2
    exit 1
fi
if ! printf '%s\n' "$observations" | jq -e -s '
    length == 24 and
    all(.[]; if .radio == "ap6" and .slot == 7 then .bridge == "-" else .enabled and .bridge != "-" end)
' >/dev/null; then
    printf 'Unexpected failed-BSS observations:\n%s\n' "$observations" >&2
    exit 1
fi
podman restart "$container" >/dev/null
assert_vaps
test "$("$manager" list | jq '.data | length')" -eq 3
manager_before=$("$manager" list | jq -Sc '.data')
printf '%s' 'rotated-development-passphrase' >"$work_dir/home"
podman restart "$container" >/dev/null
assert_vaps
test "$("$manager" list | jq -Sc '.data')" = "$manager_before"
awk '
    /^bss=wlan0v7$/ {selected=1}
    selected && /^bridge=br0v7$/ {sub(/br0v7/, "br0v5"); selected=0}
    {print}
' "$work_dir/ap0.conf" >"$work_dir/ap0.next.conf"
mv "$work_dir/ap0.next.conf" "$work_dir/ap0.conf"
jq -c 'if .radio == "ap24" and .slot == 7 then .bridge = "br0v5" else . end' \
    "$work_dir/expected.jsonl" >"$work_dir/expected.next.jsonl"
mv "$work_dir/expected.next.jsonl" "$work_dir/expected.jsonl"
jq -s '{radios: (group_by(.radio) | map({name: .[0].radio, mode:"ap", vaps: map({slot, device:.interface, bssid, bridge})}))}' \
    "$work_dir/expected.jsonl" >"$work_dir/contract.next.json"
cp "$work_dir/contract.next.json" "$work_dir/contract.json"
podman restart "$container" >/dev/null
assert_vaps
test "$("$manager" list | jq -Sc '.data')" = "$manager_before"
ip netns add "$station_namespace"
systemd-run --quiet --unit "$station_unit" --property "NetworkNamespacePath=/run/netns/$station_namespace" --service-type=exec sleep infinity
station_pid=$(systemctl show --property MainPID --value "$station_unit")
"$manager" ensure --name vcpe-vap-station --mac 02:76:63:70:66:20 --group-mask 0x4000000000000000 --netns-pid "$station_pid" --ifname wlan0 --type station | jq -e '.success' >/dev/null
ip -n "$station_namespace" link set wlan0 up
index=0
for attachment in '0 0 2.4ghz 1 wpa3-personal' '0 1 2.4ghz 1 wpa2-personal' '0 2 2.4ghz 1 open' \
    '1 0 5ghz 36 wpa3-personal' '1 1 5ghz 36 wpa2-personal' '1 2 5ghz 36 open' '2 0 6ghz 5 wpa3-personal'; do
    read -r radio slot band channel security <<<"$attachment"
    name=(ap24 ap5 ap6)
    ssid=vcpe-mirrored
    if (( slot > 0 )); then
        ssid="vcpe-${name[$radio]}-$slot"
    fi
    bssid=$(printf '02:76:63:70:%02x:%02x' "$((radio + 36))" "$((slot + 16))")
    bridge="br${radio}v${slot}"
    ip -n "$namespace" addr add "10.252.$index.1/24" dev "$bridge"
    systemd-run --quiet --unit "vcpe-vap-dhcp-$index.service" --property "NetworkNamespacePath=/run/netns/$namespace" --service-type=exec \
        /usr/sbin/dnsmasq --no-daemon --port=0 --interface="$bridge" --bind-interfaces \
        --dhcp-range="10.252.$index.100,10.252.$index.120,255.255.255.0,1h"
    podman run -d --pull=never --name "$client_container" --privileged \
        --network "ns:/run/netns/$station_namespace" \
        -v "$work_dir/home:/etc/vcpe/credentials/home:ro" \
        -e RADIO_WIFI_MODE=station -e RADIO_WIFI_DEVICE=wlan0 -e "RADIO_WIFI_SSID=$ssid" \
        -e "RADIO_WIFI_MEDIUM=rf$radio" -e "RADIO_WIFI_BAND=$band" -e "RADIO_WIFI_CHANNEL=$channel" \
        -e "RADIO_WIFI_AP_BSSID=$bssid" -e "RADIO_WIFI_SECURITY=$security" \
        -e RADIO_WIFI_PASSPHRASE_FILE=/etc/vcpe/credentials/home \
        --entrypoint /usr/local/bin/wireless-client "$client_image" >/dev/null
    station_ready=false
    for attempt in {1..45}; do
        if podman exec "$client_container" sh -c "grep -qx state=ready /run/vcpe/wireless-client/status && ip -4 -o addr show dev wlan0 | grep -q 10.252.$index."; then
            station_ready=true
            break
        fi
        sleep 1
    done
    if [[ "$station_ready" != true ]]; then
        podman logs "$client_container" | tail -15 >&2
        exit 1
    fi
    podman exec "$client_container" sh -c "grep -qx bssid=$bssid /run/vcpe/wireless-client/status && grep -qx band=$band /run/vcpe/wireless-client/status"
    station_status=$(podman exec "$client_container" wpa_cli -i wlan0 status)
    case "$security" in
        wpa3-personal) expected_key_mgmt=SAE ;;
        wpa2-personal) expected_key_mgmt=WPA2-PSK ;;
        open) expected_key_mgmt=NONE ;;
    esac
    if ! grep -Fxq "key_mgmt=$expected_key_mgmt" <<<"$station_status"; then
        printf 'Unexpected station security on %s slot %s:\n' "$band" "$slot" >&2
        grep -E '^(key_mgmt|pairwise_cipher|wpa_state)=' <<<"$station_status" >&2 || true
        exit 1
    fi
    ip netns exec "$station_namespace" ping -c 1 -W 2 "10.252.$index.1" >/dev/null
    if (( index > 0 )); then
        ip -n "$station_namespace" route replace 10.252.0.1/32 dev wlan0 scope link
        if ip netns exec "$station_namespace" ping -I wlan0 -c 1 -W 2 10.252.0.1 >/dev/null 2>&1; then
            echo "Station crossed from $bridge into the first VAP bridge" >&2
            exit 1
        fi
        ip -n "$station_namespace" route del 10.252.0.1/32 dev wlan0
    fi
    podman rm -f -t 0 "$client_container" >/dev/null
    ip -n "$station_namespace" addr flush dev wlan0
    index=$((index + 1))
done
test "$("$manager" list | jq '.data | length')" -eq 4
for rejection in 'wrong-bssid 6ghz 5 02:76:63:70:26:11 wpa3-personal' \
    'wrong-band 5ghz 36 02:76:63:70:26:10 wpa3-personal' \
    'wrong-credential 6ghz 5 02:76:63:70:26:10 wpa3-personal' \
    'open-6ghz 6ghz 5 02:76:63:70:26:10 open' \
    'downgrade 6ghz 5 02:76:63:70:26:10 wpa2-personal'; do
    read -r scenario band channel bssid security <<<"$rejection"
    credential_file="$work_dir/home"
    if [[ "$scenario" == wrong-credential ]]; then
        credential_file="$work_dir/wrong-station"
    fi
    if podman run --rm --pull=never --privileged \
        --network "ns:/run/netns/$station_namespace" \
        -v "$credential_file:/etc/vcpe/credentials/home:ro" \
        -e VCPE_ASSOCIATION_TIMEOUT_SECONDS=3 \
        -e RADIO_WIFI_MODE=station -e RADIO_WIFI_DEVICE=wlan0 -e RADIO_WIFI_SSID=vcpe-mirrored \
        -e RADIO_WIFI_MEDIUM=rf6 -e "RADIO_WIFI_BAND=$band" -e "RADIO_WIFI_CHANNEL=$channel" \
        -e "RADIO_WIFI_AP_BSSID=$bssid" -e "RADIO_WIFI_SECURITY=$security" \
        -e RADIO_WIFI_PASSPHRASE_FILE=/etc/vcpe/credentials/home \
        --entrypoint /usr/local/bin/wireless-client "$client_image" >"$work_dir/$scenario.log" 2>&1; then
        echo "Wireless client accepted $scenario" >&2
        exit 1
    fi
    expected_failure='association timed out'
    if [[ "$scenario" == downgrade || "$scenario" == open-6ghz ]]; then
        expected_failure='requires 6 GHz PSC and WPA3-Personal'
    fi
    if ! grep -Fq "$expected_failure" "$work_dir/$scenario.log"; then
        echo "Wireless client rejected $scenario for an unexpected reason" >&2
        exit 1
    fi
    if grep -Fq 'incorrect-station-passphrase' "$work_dir/$scenario.log"; then
        echo "Wireless client disclosed a rejected credential" >&2
        exit 1
    fi
    if [[ -n "$(ip -n "$station_namespace" -4 -o addr show dev wlan0)" ]]; then
        echo "Wireless client obtained DHCP before rejecting $scenario" >&2
        exit 1
    fi
done
echo 'Gateway 24 BSSs and tri-band station association, DHCP, and rejection: ok'