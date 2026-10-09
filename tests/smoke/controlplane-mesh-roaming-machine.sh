#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
offline=${VCPE_MESH_SMOKE_VALIDATE:-0}
if [[ "$offline" == 1 && "${VCPE_RUN_MESH_SMOKE:-0}" == 1 ]]; then
    echo 'choose either offline validation or the live machine proof' >&2
    exit 1
fi
if [[ "$offline" != 1 && "${VCPE_RUN_MESH_SMOKE:-0}" != 1 ]]; then
    echo 'mesh roaming machine smoke: SKIP (set VCPE_RUN_MESH_SMOKE=1 to require the live proof)'
    exit 0
fi

vcpe_bin=${VCPE_BIN:-}
unset VCPE_SKIP_RUNTIME VCPE_SKIP_IMAGE VCPE_SKIP_HOSTNET_PREFLIGHT VCPE_FAIL_PHASE VCPE_DAEMON_SOCKET VCPE_HOSTNET_DELEGATED
for command in ruby; do
    command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done
if [[ "$offline" != 1 ]]; then
    for command in podman jq; do
        command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
    done
    podman machine inspect | jq -e '.[0].State == "running" and .[0].Rootful == true' >/dev/null || {
        echo 'a running rootful Podman machine is required' >&2
        exit 1
    }
    manager=/usr/libexec/vcpe/vcpe-hwsim
    podman machine ssh -- sudo -n "$manager" doctor --probe |
        jq -e '.success and .data.healthy and .data.meshCapability == "supported"' >/dev/null
    [[ $(podman machine ssh -- sudo -n "$manager" list | jq '.data | length') == 0 ]] || {
        echo 'mesh smoke requires an idle wireless manager; leave existing radios untouched' >&2
        exit 1
    }
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/vcpe-mesh-smoke.XXXXXX")
state_root=$work_dir/state
primary=mesh-roaming
neighbor=mesh-roaming-alt
primary_manifest=$work_dir/primary.yaml
neighbor_manifest=$work_dir/neighbor.yaml
started_primary=false
started_neighbor=false
mesh_disabled=false
cleanup() {
    exit_code=$?
    trap - EXIT
    if (( exit_code != 0 )) && [[ "$started_primary" == true ]]; then
        podman machine ssh -- sudo -n "$manager" doctor --probe >"$work_dir/manager-doctor.json" 2>"$work_dir/manager-doctor.log" || true
        podman machine ssh -- sudo -n "$manager" medium-health >"$work_dir/manager-medium.json" 2>/dev/null || true
        podman machine ssh -- sudo -n "$manager" list >"$work_dir/manager-radios.json" 2>/dev/null || true
        podman inspect --type container --format '{{json .State}}' "$primary-root-1" >"$work_dir/root-container-state.json" 2>/dev/null || true
        podman logs "$primary-root-1" >"$work_dir/root-container.log" 2>&1 || true
        for node in root remote; do
            podman exec "$primary-$node-1" systemctl --no-pager status vcpe-hostapd vcpe-mesh >"$work_dir/$node-wireless-status.log" 2>&1 || true
            podman exec "$primary-$node-1" journalctl -u vcpe-hostapd -u vcpe-mesh -n 80 --no-pager >"$work_dir/$node-wireless-journal.log" 2>&1 || true
            podman exec "$primary-$node-1" gateway-health-probe vaps >"$work_dir/$node-vaps.log" 2>&1 || true
            podman exec "$primary-$node-1" gateway-health-probe mesh >"$work_dir/$node-mesh.log" 2>&1 || true
            podman exec "$primary-$node-1" iw phy >"$work_dir/$node-phy.log" 2>&1 || true
            podman exec "$primary-$node-1" iw reg get >"$work_dir/$node-reg.log" 2>&1 || true
            podman exec "$primary-$node-1" iw dev mesh0 station dump >"$work_dir/$node-stations.log" 2>&1 || true
            podman exec "$primary-$node-1" iw dev wlan0 station dump >"$work_dir/$node-ap-stations.log" 2>&1 || true
        done
        podman exec "$primary-root-1" pgrep -a dnsmasq >"$work_dir/root-dhcp-process.log" 2>&1 || true
        for node in root remote; do
            podman exec "$primary-$node-1" ip -j -4 addr show dev brlan0 >"$work_dir/$node-bridge-address.log" 2>&1 || true
            podman exec "$primary-$node-1" ip neigh show dev mesh0 >"$work_dir/$node-mesh-neighbors.log" 2>&1 || true
            podman exec "$primary-$node-1" ip neigh show dev brlan0 >"$work_dir/$node-bridge-neighbors.log" 2>&1 || true
            podman exec "$primary-$node-1" bridge link show >"$work_dir/$node-bridge-links.log" 2>&1 || true
            podman exec "$primary-$node-1" bridge fdb show br brlan0 >"$work_dir/$node-bridge-fdb.log" 2>&1 || true
            podman exec "$primary-$node-1" iw dev mesh0 mpath dump >"$work_dir/$node-mesh-paths.log" 2>&1 || true
            podman exec "$primary-$node-1" iw dev mesh0 mpp dump >"$work_dir/$node-mesh-proxies.log" 2>&1 || true
            podman exec "$primary-$node-1" ip -s link show dev mesh0 >"$work_dir/$node-mesh-counters.log" 2>&1 || true
        done
        podman exec "$primary-remote-1" ping -n -c 1 -W 2 -I brlan0 10.0.0.1 >"$work_dir/remote-bridge-ping.log" 2>&1 || true
        for client in station remote-probe; do
            podman logs "$primary-$client-1" >"$work_dir/$client-container.log" 2>&1 || true
            podman exec "$primary-$client-1" cat /run/vcpe/wireless-client/status >"$work_dir/$client-status.log" 2>&1 || true
            podman exec "$primary-$client-1" wpa_cli -i wlan0 status >"$work_dir/$client-wpa.log" 2>&1 || true
            podman exec "$primary-$client-1" ip -j -4 addr show dev wlan0 >"$work_dir/$client-address.log" 2>&1 || true
            podman exec "$primary-$client-1" ping -n -c 1 -W 2 -I wlan0 10.0.0.1 >"$work_dir/$client-root-ping.log" 2>&1 || true
            podman exec "$primary-$client-1" ip neigh show dev wlan0 >"$work_dir/$client-neighbors.log" 2>&1 || true
            podman exec "$primary-$client-1" ip route get 10.0.0.1 >"$work_dir/$client-root-route.log" 2>&1 || true
            podman exec "$primary-$client-1" iw dev wlan0 link >"$work_dir/$client-link.log" 2>&1 || true
        done
    fi
    if [[ "$mesh_disabled" == true ]]; then
        podman exec "$primary-remote-1" ip link set mesh0 up >/dev/null 2>&1 || true
    fi
    if [[ "$started_primary" == true ]] && ! "$vcpe_bin" down --name "$primary" --state-root "$state_root"; then
        exit_code=1
    fi
    if [[ "$started_neighbor" == true ]] && ! "$vcpe_bin" down --name "$neighbor" --state-root "$state_root"; then
        exit_code=1
    fi
    if (( exit_code == 0 )); then
        rm -rf -- "$work_dir"
    else
        echo "mesh smoke failed; inspect retained state and manifests at $work_dir" >&2
    fi
    exit "$exit_code"
}
trap cleanup EXIT
if [[ -z "$vcpe_bin" ]]; then
    vcpe_bin=$work_dir/vcpe
    (cd "$repo_root/controlplane" && go build -o "$vcpe_bin" ./cmd/vcpe)
fi
[[ -x "$vcpe_bin" ]] || { echo "vcpe binary is not executable: $vcpe_bin" >&2; exit 1; }

for name in "$primary" "$neighbor"; do
    manifest=$primary_manifest
    [[ "$name" == "$neighbor" ]] && manifest=$neighbor_manifest
    ruby -ryaml -e '
        document = YAML.safe_load(File.read(ARGV.fetch(0)))
        name = ARGV.fetch(2)
        document.fetch("metadata")["name"] = name
        gateways = document.fetch("spec").fetch("services").select { |service| service.fetch("type") == "gateway" }
        abort "two Gateways are required" unless gateways.size == 2
        gateways.each do |service|
            image = service.fetch("image")
            abort "Gateway image must use the local development repository" unless image.fetch("repository") == "ghcr.io/gdcs-dev/gateway"
            service.fetch("radios").select { |radio| radio.fetch("mode") == "mesh" }.each do |radio|
                radio.fetch("mesh")["id"] = "mesh-lab-alt" if name.end_with?("-alt")
            end
        end
        if name.end_with?("-alt")
            document.fetch("spec").fetch("wirelessNetworks").each { |network| network["ssid"] = "mesh-lab-alt" }
            networks = document.fetch("spec").fetch("networks").to_h { |network| [network.fetch("role"), network] }
            networks.fetch("mgmt").fetch("ipv4").merge!("cidr" => "10.10.31.0/24", "gateway" => "10.10.31.1", "pool" => {"start" => "10.10.31.10", "end" => "10.10.31.250"})
            networks.fetch("wan").fetch("ipv4").merge!("cidr" => "10.7.211.0/24", "gateway" => "10.7.211.1", "pool" => {"start" => "10.7.211.10", "end" => "10.7.211.250"})
            bng = document.fetch("spec").fetch("services").find { |service| service.fetch("name") == "bng" }
            bng.fetch("interfaces").find { |interface| interface.fetch("role") == "wan" }["ipv4"] = "10.7.211.1"
            dhcp = bng.fetch("config").fetch("access").find { |access| access.fetch("role") == "wan" }.fetch("dhcp4")
            dhcp.merge!("subnet" => "10.7.211.0/24", "ranges" => [{"start" => "10.7.211.100", "end" => "10.7.211.200"}])
            dhcp.fetch("options")["routers"] = "10.7.211.1"
        end
        File.write(ARGV.fetch(1), YAML.dump(document))
    ' "$repo_root/manifests/dev/mesh-roaming.yaml" "$manifest" "$name"
    "$vcpe_bin" plan --manifest "$manifest" --state-root "$state_root" >/dev/null
done

ruby -ryaml -e '
    primary, neighbor = ARGV.map { |path| YAML.safe_load(File.read(path)) }
    primary_networks = primary.fetch("spec").fetch("networks").to_h { |network| [network.fetch("role"), network.fetch("ipv4").fetch("cidr")] }
    neighbor_networks = neighbor.fetch("spec").fetch("networks").to_h { |network| [network.fetch("role"), network.fetch("ipv4").fetch("cidr")] }
    abort "mesh deployments share management or WAN addressing" unless %w[mgmt wan].all? { |role| primary_networks.fetch(role) != neighbor_networks.fetch(role) }
' "$primary_manifest" "$neighbor_manifest"

if [[ "$offline" == 1 ]]; then
    echo 'mesh roaming manifests: valid (offline plan only; no machine proof)'
    exit 0
fi

started_primary=true
"$vcpe_bin" up --manifest "$primary_manifest" --state-root "$state_root"
podman machine ssh -- sudo -n "$manager" medium-health |
    jq -e '(.data.checks | map(select(.name == "baseline" or .name == "service" or .name == "socket" or .name == "registration"))) as $checks |
        .success and .data.healthy and (($checks | length) == 4) and ($checks | all(.[]; .ok))' >/dev/null

check_remote_path() {
    local deployment=$1
    local remote="$deployment-remote-1"
    local probe="$deployment-remote-probe-1"
    local client_status client_ipv4 client_mac
    for attempt in {1..45}; do
        client_status=$(podman exec "$probe" cat /run/vcpe/wireless-client/status 2>/dev/null) || client_status=
        client_ipv4=$(printf '%s\n' "$client_status" | sed -n 's@^ipv4=\([^/]*\)/24$@\1@p')
        client_mac=$(podman exec "$probe" cat /sys/class/net/wlan0/address 2>/dev/null) || client_mac=
        if podman exec "$remote" gateway-health-probe mesh 2>/dev/null |
            jq -e '.radio == "backhaul" and .interface == "mesh0" and .bridge == "brlan0" and .joined and .peers > 0' >/dev/null 2>&1 &&
            podman exec "$probe" grep -Fxq 'state=ready' /run/vcpe/wireless-client/status &&
            podman exec "$probe" grep -Eq '^ipv4=10\.0\.0\.(1[0-9][0-9]|200)/24$' /run/vcpe/wireless-client/status &&
            [[ "$client_ipv4" =~ ^10\.0\.0\.(1[0-9][0-9]|200)$ && "$client_mac" =~ ^[[:xdigit:]:]+$ ]] &&
            podman exec "$deployment-root-1" cat /var/lib/misc/dnsmasq.leases 2>/dev/null |
                awk -v mac="$client_mac" -v ip="$client_ipv4" '$2 == mac && $3 == ip { found = 1 } END { exit !found }' &&
            podman exec "$remote" ip -j -4 addr show dev brlan0 2>/dev/null |
                jq -e '[.[].addr_info[]? | select(.family == "inet")] | length == 0' >/dev/null &&
            ! podman exec "$remote" pgrep -x dnsmasq >/dev/null 2>&1 &&
            podman exec "$probe" ping -n -c 1 -W 2 -I wlan0 10.0.0.1 >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    echo "remote mesh DHCP/LAN path did not recover for $deployment" >&2
    return 1
}

check_management() {
    go -C "$repo_root/controlplane" run ./tests/meshwebpa "$1"
}

station_ipv4() {
    podman exec "$primary-station-1" ip -j -4 addr show dev wlan0 |
        jq -er '.[0].addr_info[] | select(.family == "inet") | "\(.local)/\(.prefixlen)"'
}

check_station_path() {
    for attempt in {1..15}; do
        if podman exec "$primary-station-1" ping -n -c 1 -W 2 -I wlan0 10.0.0.1 >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    echo "roaming station root LAN path unavailable after $1" >&2
    return 1
}

check_remote_path "$primary"
check_station_path 'primary apply'
initial_ipv4=$(station_ipv4)
manager_before=$(podman machine ssh -- sudo -n "$manager" list | jq -cS '.data | map(.record.name) | sort')
"$vcpe_bin" up --manifest "$primary_manifest" --state-root "$state_root"
[[ $(station_ipv4) == "$initial_ipv4" ]] || { echo 'unchanged reapply changed station IPv4' >&2; exit 1; }
[[ $(podman machine ssh -- sudo -n "$manager" list | jq -cS '.data | map(.record.name) | sort') == "$manager_before" ]] || {
    echo 'unchanged reapply changed radio ownership' >&2
    exit 1
}
check_remote_path "$primary"

started_neighbor=true
"$vcpe_bin" up --manifest "$neighbor_manifest" --state-root "$state_root"
check_remote_path "$neighbor"
check_station_path 'neighbor apply'
check_management "$primary_manifest"
podman machine ssh -- sudo -n "$manager" list |
    jq -e '.data | (length == 12) and ([.[].record.name] | unique | length == 12) and ([.[].record.groupMask] | unique | length == 2)' >/dev/null
manager_all_before=$(podman machine ssh -- sudo -n "$manager" list | jq -cS '.data | map(.record.name) | sort')

remote="$primary-remote-1"
probe="$primary-remote-probe-1"
restart_remote_gateway() {
    podman exec "$remote" ip link set erouter0 name eth1
    podman restart "$remote"
}
echo 'mesh smoke: disable primary mesh path'
podman exec "$remote" ip link set mesh0 down
mesh_disabled=true
if podman exec "$probe" ping -n -c 1 -W 2 -I wlan0 10.0.0.1 >/dev/null 2>&1; then
    echo 'remote probe still reached the root with the only mesh path disabled' >&2
    exit 1
fi
podman exec "$probe" iw dev wlan0 link | grep -q 'Connected to ' || {
    echo 'remote AP association disappeared during mesh path failure' >&2
    exit 1
}
check_remote_path "$neighbor"
check_management "$primary_manifest"
echo 'mesh smoke: restore remote mesh service after path repair'
podman exec "$remote" ip link set mesh0 up
mesh_disabled=false
podman exec "$remote" systemctl restart vcpe-mesh
"$vcpe_bin" up --manifest "$primary_manifest" --state-root "$state_root"
check_remote_path "$primary"
check_station_path 'remote Gateway repair'
[[ $(podman machine ssh -- sudo -n "$manager" list | jq -cS '.data | map(.record.name) | sort') == "$manager_all_before" ]] || {
    echo 'Gateway repair allocated a different radio identity' >&2
    exit 1
}

echo 'mesh smoke: run automatic roaming scenario'
"$vcpe_bin" scenario run --name "$primary" --scenario crossover --state-root "$state_root" |
    grep -Eq 'passed \(controlled lab\): .* -> .* in .*; IPv4 .* unchanged; maximum bounded probe outage'
[[ $(station_ipv4) == "$initial_ipv4" ]] || { echo 'automatic roam changed station IPv4' >&2; exit 1; }
echo 'mesh smoke: recreate remote Gateway'
restart_remote_gateway
"$vcpe_bin" up --manifest "$primary_manifest" --state-root "$state_root"
check_remote_path "$primary"
[[ $(podman machine ssh -- sudo -n "$manager" list | jq -cS '.data | map(.record.name) | sort') == "$manager_all_before" ]] || {
    echo 'Gateway recreation changed another deployment or radio identity' >&2
    exit 1
}

"$vcpe_bin" down --name "$primary" --state-root "$state_root"
started_primary=false
check_remote_path "$neighbor"
"$vcpe_bin" down --name "$neighbor" --state-root "$state_root"
started_neighbor=false
[[ $(podman machine ssh -- sudo -n "$manager" list | jq '.data | length') == 0 ]] || {
    echo 'wireless radios remained after both deployments were removed' >&2
    exit 1
}
echo 'mesh roaming machine smoke: ok'