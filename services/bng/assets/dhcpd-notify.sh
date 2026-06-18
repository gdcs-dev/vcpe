#!/bin/bash
set -euo pipefail

event=${1:-}
lease_ip=${2:-}
client_mac=${3:-}
client_name=${4:-}
map_file=/etc/dnsmasq.dhcp-hosts.map
subnet_map_file=/etc/dnsmasq.dhcp-subnets.map
state_file=/var/lib/dhcp/dnsmasq-dynamic.state
hosts_file=/etc/dnsmasq.dynamic.hosts
log_file=/var/log/dhcpd-notify.log

normalize_mac() {
	printf '%s\n' "$1" | tr '[:upper:]' '[:lower:]'
}

sanitize_hostname() {
	printf '%s\n' "$1" |
		tr '[:upper:]' '[:lower:]' |
		sed -E 's/[^a-z0-9.-]+/-/g; s/^-+//; s/-+$//; s/\.-/./g; s/-\././g'
}

lookup_mapped_hostname() {
	local normalized_mac=$1

	[[ -f "$map_file" ]] || return 1
	awk -v mac="$normalized_mac" 'tolower($1) == mac { print $2; exit }' "$map_file"
}

lookup_subnet_hostname() {
	local ip=$1

	[[ -f "$subnet_map_file" ]] || return 1
	python3 - "$subnet_map_file" "$ip" <<'PY'
import ipaddress
import sys

map_path = sys.argv[1]
lease_ip = ipaddress.ip_address(sys.argv[2])

with open(map_path, encoding="utf-8") as handle:
	for raw_line in handle:
		line = raw_line.strip()
		if not line:
			continue
		subnet, hostname = line.split(maxsplit=1)
		if lease_ip in ipaddress.ip_network(subnet, strict=False):
			print(hostname)
			raise SystemExit(0)

raise SystemExit(1)
PY
}

refresh_hosts_file() {
	local temp_hosts

	temp_hosts=$(mktemp)
	if [[ -f "$state_file" ]]; then
		awk 'NF >= 2 { print $2 " " $1 }' "$state_file" >"$temp_hosts"
	fi
	chmod 0644 "$temp_hosts"
	mv "$temp_hosts" "$hosts_file"
	pkill -HUP dnsmasq 2>/dev/null || true
}

remove_state_records() {
	local hostname=$1
	local ip=$2
	local mac=$3
	local temp_state

	temp_state=$(mktemp)
	if [[ -f "$state_file" ]]; then
		awk -v hostname="$hostname" -v ip="$ip" -v mac="$mac" '
			$1 != hostname && $2 != ip && tolower($3) != mac { print }
		' "$state_file" >"$temp_state"
	fi
	mv "$temp_state" "$state_file"
}

mkdir -p /var/lib/dhcp /var/log

normalized_mac=$(normalize_mac "$client_mac")
sanitized_client_name=$(sanitize_hostname "$client_name")
mapped_name=$(lookup_mapped_hostname "$normalized_mac" || true)
subnet_name=$(lookup_subnet_hostname "$lease_ip" || true)
resolved_name=${sanitized_client_name:-${mapped_name:-${subnet_name:-}}}

printf '%s event=%s ip=%s mac=%s host=%s\n' "$(date -Is)" "$event" "$lease_ip" "$normalized_mac" "$resolved_name" >>"$log_file"

case "$event" in
	commit)
		[[ -n "$resolved_name" ]] || exit 0
		remove_state_records "$resolved_name" "$lease_ip" "$normalized_mac"
		printf '%s %s %s\n' "$resolved_name" "$lease_ip" "$normalized_mac" >>"$state_file"
		refresh_hosts_file
		;;
	release|expiry)
		remove_state_records "$resolved_name" "$lease_ip" "$normalized_mac"
		refresh_hosts_file
		;;
	*)
		exit 0
		;;
esac
