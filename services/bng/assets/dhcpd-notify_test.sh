#!/bin/bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
subject="$script_dir/dhcpd-notify.sh"

if ! grep -q 'DHCPD_NOTIFY_MAP_FILE' "$subject"; then
	echo "dhcpd-notify.sh does not support sandboxed runtime paths" >&2
	exit 1
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

map_file="$work_dir/dhcp-hosts.map"
state_file="$work_dir/dhcp.state"
hosts_file="$work_dir/dhcp.hosts"
management_file="$work_dir/management.hosts"
log_file="$work_dir/dhcp.log"
lock_dir="$work_dir/dhcp.lock"

cat >"$map_file" <<'EOF'
02:00:00:00:00:01 gateway gateway-1 gateway-1-wan
02:00:00:00:00:02 xb10 xb10-1 xb10-1-wan
EOF
printf '10.10.10.11 webpa-1\n' >"$management_file"

notify() {
	DHCPD_NOTIFY_MAP_FILE="$map_file" \
	DHCPD_NOTIFY_STATE_FILE="$state_file" \
	DHCPD_NOTIFY_HOSTS_FILE="$hosts_file" \
	DHCPD_NOTIFY_LOG_FILE="$log_file" \
	DHCPD_NOTIFY_LOCK_DIR="$lock_dir" \
	DHCPD_NOTIFY_SKIP_RELOAD=1 \
		bash "$subject" "$@"
}

assert_line() {
	local file=$1
	local expected=$2
	grep -Fxq "$expected" "$file" || {
		echo "missing line '$expected' in $file" >&2
		cat "$file" >&2
		exit 1
	}
}

assert_not_contains() {
	local file=$1
	local unexpected=$2
	if [[ -f "$file" ]] && grep -Fq "$unexpected" "$file"; then
		echo "unexpected '$unexpected' in $file" >&2
		cat "$file" >&2
		exit 1
	fi
}

notify commit 10.7.200.100 02:00:00:00:00:01 spoofed-name
assert_line "$hosts_file" "10.7.200.100 gateway gateway-1 gateway-1-wan"
assert_not_contains "$hosts_file" "spoofed-name"

notify commit 10.7.200.101 02:00:00:00:00:01 ignored-again
assert_line "$hosts_file" "10.7.200.101 gateway gateway-1 gateway-1-wan"
assert_not_contains "$hosts_file" "10.7.200.100"

notify release 10.7.200.100 02:00:00:00:00:01 ignored
assert_line "$hosts_file" "10.7.200.101 gateway gateway-1 gateway-1-wan"

before_unknown=$(cat "$hosts_file")
notify commit 10.7.200.150 02:00:00:00:00:ff webpa
[[ $(cat "$hosts_file") == "$before_unknown" ]] || {
	echo "unknown MAC changed DHCP hosts" >&2
	exit 1
}

notify expiry 10.7.200.101 02:00:00:00:00:01 ignored
assert_not_contains "$hosts_file" "gateway"
assert_line "$management_file" "10.10.10.11 webpa-1"

: >"$state_file"
: >"$hosts_file"
notify commit 10.7.200.110 02:00:00:00:00:01 ignored &
first_pid=$!
notify commit 10.7.200.120 02:00:00:00:00:02 ignored &
second_pid=$!
wait "$first_pid"
wait "$second_pid"
assert_line "$hosts_file" "10.7.200.110 gateway gateway-1 gateway-1-wan"
assert_line "$hosts_file" "10.7.200.120 xb10 xb10-1 xb10-1-wan"

echo "dhcpd-notify tests passed"