#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
source "$repo_root/services/bng/container/management-dns.sh"

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
printf '10.10.10.11 webpa-1\n' > "$work_dir/seed"
: > "$work_dir/live"
: > "$work_dir/reloads"

peer_ready=0
peer_ip=10.10.10.11
lookup_peer_ip() {
    [[ "$peer_ready" == 1 ]] && printf '%s' "$peer_ip"
}
reload_dnsmasq() {
    printf 'reload\n' >> "$work_dir/reloads"
}

refresh_management_hosts "$work_dir/seed" "$work_dir/live"
[[ ! -s "$work_dir/live" ]]
peer_ready=1
refresh_management_hosts "$work_dir/seed" "$work_dir/live"
[[ $(< "$work_dir/live") == '10.10.10.11 webpa-1' ]]
[[ -n $(find "$work_dir/live" -perm -004 -print) ]]
[[ $(wc -l < "$work_dir/reloads") -eq 1 ]]
refresh_management_hosts "$work_dir/seed" "$work_dir/live"
[[ $(wc -l < "$work_dir/reloads") -eq 1 ]]
peer_ip=10.10.10.12
refresh_management_hosts "$work_dir/seed" "$work_dir/live"
[[ $(< "$work_dir/live") == '10.10.10.12 webpa-1' ]]
[[ $(wc -l < "$work_dir/reloads") -eq 2 ]]
peer_ready=0
refresh_management_hosts "$work_dir/seed" "$work_dir/live"
[[ $(< "$work_dir/live") == '10.10.10.12 webpa-1' ]]
[[ $(wc -l < "$work_dir/reloads") -eq 2 ]]

echo 'bng management dns: ok'